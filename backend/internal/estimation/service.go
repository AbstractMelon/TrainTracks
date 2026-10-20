package estimation

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"traintracks/internal/config"
	"traintracks/internal/geo"
	"traintracks/internal/models"
	"traintracks/internal/storage/postgres"
	"traintracks/internal/storage/redis"
)

// DefaultSpeedEstimateMps is the historical-average fallback when a train has
// no speed observation (~54 km/h, a typical light-rail cruising speed). Tune
// with real data.
const DefaultSpeedEstimateMps = 15.0

// PredictionMaxAge is how long a trip-update prediction stays usable.
const PredictionMaxAge = 30 * time.Minute

type stationTiming struct {
	station models.Station
	arr     *time.Time
	dep     *time.Time
	delay   *int
}

type pathKey struct{ route, from, to string }

type pathEntry struct {
	pts    []models.Point
	loaded time.Time
}

const (
	// routePathTTL bounds how long resolved stop-to-stop geometry is reused.
	// Shapes are static GTFS data, so this only has to survive re-imports.
	routePathTTL = 10 * time.Minute
	// routePathMaxEntries caps the cache; the active stop pairs are few, so
	// this is only a guard against unbounded growth.
	routePathMaxEntries = 4096
)

// Service sweeps stale trains and pushes ESTIMATED/STALE/UNKNOWN positions to
// Redis. Expected to run on the TrackingSweep ticker inside cmd/ingestor.
type Service struct {
	pg  *postgres.Store
	rd  *redis.Store
	cfg config.Config
	log *slog.Logger

	mu     sync.Mutex // serialize sweeps; cheap and prevents interleaved writes
	pathMu sync.Mutex
	paths  map[pathKey]pathEntry
}

func NewService(pg *postgres.Store, rd *redis.Store, cfg config.Config, log *slog.Logger) *Service {
	return &Service{pg: pg, rd: rd, cfg: cfg, log: log, paths: map[pathKey]pathEntry{}}
}

// Sweep ages every active train once and writes any that cross the freshness
// boundary. Mirrors SPEC.md §6's 5s ticker and §7's algorithm. Predictions are
// read once per agency per sweep, not once per stale train: a per-train read
// made a sweep take minutes and froze "now" at its start.
func (s *Service) Sweep(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	agencies, err := s.pg.ListAgencies(ctx)
	if err != nil {
		return err
	}
	for _, agency := range agencies {
		states, err := s.rd.ListActiveByAgency(ctx, agency.ID)
		if err != nil {
			return err
		}
		stale := make([]models.TrainState, 0, len(states))
		for _, st := range states {
			age := now.Sub(st.LastActualAt)
			if age <= s.cfg.FreshWindow {
				continue
			}
			// UNKNOWN is terminal: Evaluate has no newer answer for that train,
			// and rewriting the state would only refresh StateTTL every tick,
			// pinning a dead train's marker to the map forever. The write that
			// marks it UNKNOWN is the last, so the key lapses StateTTL later and
			// the marker is pruned.
			if st.PositionType == models.PositionUnknown {
				continue
			}
			stale = append(stale, st)
		}
		if len(stale) == 0 {
			continue
		}
		preds, err := s.pg.LatestTripPredictions(ctx, agency.ID, PredictionMaxAge)
		if err != nil {
			s.log.Warn("trip predictions unavailable", "agency", agency.ID, "err", err)
			preds = nil
		}
		byTrip := predictionsByTrip(preds)
		for _, st := range stale {
			if err := s.ageOne(ctx, st, now.Sub(st.LastActualAt), now, byTrip[st.TripID]); err != nil {
				s.log.Error("estimation sweep failed for train", "train", st.TrainID, "err", err)
			}
		}
	}
	return nil
}

// predictionsByTrip indexes a sweep's predictions for per-train lookups.
func predictionsByTrip(preds []models.StopTimePrediction) map[string][]models.StopTimePrediction {
	byTrip := make(map[string][]models.StopTimePrediction, 64)
	for _, p := range preds {
		byTrip[p.TripID] = append(byTrip[p.TripID], p)
	}
	return byTrip
}

func (s *Service) ageOne(ctx context.Context, st models.TrainState, age time.Duration, now time.Time, preds []models.StopTimePrediction) error {
	in := Input{
		State:         st,
		Age:           age,
		FreshWindow:   s.cfg.FreshWindow,
		MaxWindow:     s.cfg.MaxEstimationWindow,
		MinConfidence: s.cfg.MinConfidence,
	}

	if st.TripID != "" {
		tt, speed, delay := s.buildTripTiming(ctx, st.TripID, st.RouteID, now, preds)
		in.Trips = tt
		in.SpeedEstimate = speed
		if tt != nil && delay != nil {
			st.DelaySeconds = delay
		}
	}
	if in.SpeedEstimate == 0 {
		if st.Speed != nil && *st.Speed > 0 {
			in.SpeedEstimate = *st.Speed
		} else {
			in.SpeedEstimate = DefaultSpeedEstimateMps
		}
	}

	out := Evaluate(in)
	st.Lat = out.Lat
	st.Lon = out.Lon
	st.PositionType = out.PositionType
	st.Confidence = out.Confidence
	st.CurrentStationID = out.CurrentStationID
	st.NextStationID = out.NextStationID
	st.PositionSource = "estimation-engine"
	st.LastUpdateAt = now
	if st.DelaySeconds == nil {
		st.DelaySeconds = out.DelaySeconds
	}

	s.log.Debug("train aged", "train", st.TrainID, "type", st.PositionType, "reason", out.Reason, "confidence", out.Confidence)
	return s.rd.SetTrainState(ctx, st)
}

// buildTripTiming assembles the bracketing stop pair for a trip from the
// sweep's prediction snapshot, along with the route geometry between those
// stops. Returns nil when there aren't enough predictions. Also returns an
// estimated speed over the bracketing gap and the next stop's predicted delay.
func (s *Service) buildTripTiming(ctx context.Context, tripID, routeID string, now time.Time, preds []models.StopTimePrediction) (*TripTiming, float64, *int) {
	bySeq := map[int]models.StopTimePrediction{}
	for _, p := range preds {
		bySeq[p.Sequence] = p
	}
	if len(bySeq) < 2 {
		return nil, 0, nil
	}
	stopStations, err := s.pg.TripStopsWithStations(ctx, tripID)
	if err != nil || len(stopStations) == 0 {
		return nil, 0, nil
	}

	seqStops := []stationTiming{}
	for _, ts := range stopStations {
		p, ok := bySeq[ts.Sequence]
		if !ok {
			continue
		}
		seqStops = append(seqStops, stationTiming{station: ts.Station, arr: p.PredictedArrival, dep: p.PredictedDeparture, delay: p.DelaySeconds})
	}
	if len(seqStops) < 2 {
		return nil, 0, nil
	}

	var prevIdx, nextIdx = -1, -1
	for i, sp := range seqStops {
		if t := firstOf(sp.dep, sp.arr); t != nil && !t.After(now) {
			prevIdx = i
		}
		if t := firstOf(sp.arr, sp.dep); t != nil && !t.Before(now) && nextIdx == -1 {
			nextIdx = i
		}
	}
	if prevIdx == -1 {
		prevIdx = 0
	}
	if nextIdx == -1 {
		nextIdx = len(seqStops) - 1
	}
	if prevIdx >= nextIdx {
		prevIdx = nextIdx - 1
		if prevIdx < 0 {
			prevIdx = 0
		}
	}

	tt := &TripTiming{
		Prev: toBracketing(seqStops[prevIdx]),
		Next: toBracketing(seqStops[nextIdx]),
		Now:  now,
	}

	// Route geometry for the gap, so the estimate follows the line instead of
	// cutting the corner. Chainage of the pair, and from it the speed estimate,
	// comes from that geometry or from the straight line when there's no shape.
	span := 0.0
	if path := s.routePath(ctx, routeID, tt.Prev.StationID, tt.Next.StationID); len(path) > 0 {
		tt.Path = path
		span = pathLength(path)
	}
	if span <= 0 {
		tt.Path = nil
		span = geo.HaversineMeters(tt.Prev.Lat, tt.Prev.Lon, tt.Next.Lat, tt.Next.Lon)
	}
	tt.Next.Chainage = span

	// Speed estimate: avg travel over the bracketing gap.
	speed := 0.0
	if tt.Prev.Departure != nil && tt.Next.Arrival != nil {
		if dt := tt.Next.Arrival.Sub(*tt.Prev.Departure).Seconds(); dt > 0 {
			speed = span / dt
		}
	}
	var delay *int
	if d := seqStops[nextIdx].delay; d != nil {
		delay = d
	}
	return tt, speed, delay
}

// routePath resolves the route geometry between two stations and memoizes it,
// misses included: routes without a shape would otherwise be re-queried on
// every sweep tick for every stale train.
func (s *Service) routePath(ctx context.Context, routeID, fromStationID, toStationID string) []models.Point {
	key := pathKey{route: routeID, from: fromStationID, to: toStationID}
	now := time.Now()

	s.pathMu.Lock()
	cached, ok := s.paths[key]
	s.pathMu.Unlock()
	if ok && now.Sub(cached.loaded) < routePathTTL {
		return cached.pts
	}

	pts, err := s.pg.RoutePathBetween(ctx, routeID, fromStationID, toStationID)
	if err != nil {
		s.log.Warn("route path unavailable", "route", routeID, "err", err)
		return nil // failures aren't cached; only misses are
	}

	s.pathMu.Lock()
	defer s.pathMu.Unlock()
	if len(s.paths) >= routePathMaxEntries {
		for k, e := range s.paths {
			if now.Sub(e.loaded) >= routePathTTL {
				delete(s.paths, k)
			}
		}
		if len(s.paths) >= routePathMaxEntries {
			s.paths = map[pathKey]pathEntry{}
		}
	}
	s.paths[key] = pathEntry{pts: pts, loaded: now}
	return pts
}

func firstOf(a, b *time.Time) *time.Time {
	if a != nil {
		return a
	}
	return b
}

func toBracketing(st stationTiming) BracketingStop {
	return BracketingStop{
		StationID: st.station.ID,
		Lat:       st.station.Lat,
		Lon:       st.station.Lon,
		Departure: st.dep,
		Arrival:   st.arr,
	}
}
