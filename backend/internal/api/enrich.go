package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"traintracks/internal/models"
	"traintracks/internal/storage/postgres"
)

type nameCache struct {
	pg       *postgres.Store
	ttl      time.Duration
	log      *slog.Logger
	mu       sync.Mutex
	stations map[string]string
	routes   map[string]RouteRef
	loaded   time.Time
}

func newNameCache(pg *postgres.Store, ttl time.Duration, log *slog.Logger) *nameCache {
	return &nameCache{pg: pg, ttl: ttl, log: log, stations: map[string]string{}, routes: map[string]RouteRef{}}
}

func (n *nameCache) refreshLocked(ctx context.Context) error {
	if time.Since(n.loaded) < n.ttl {
		return nil
	}
	sts, err := n.pg.ListStations(ctx, "", "")
	if err != nil {
		return err
	}
	stations := make(map[string]string, len(sts))
	for _, st := range sts {
		stations[st.ID] = st.Name
	}
	rs, err := n.pg.ListRoutes(ctx, "")
	if err != nil {
		return err
	}
	routes := make(map[string]RouteRef, len(rs))
	for _, r := range rs {
		routes[r.ID] = RouteRef{ID: r.ID, ShortName: r.ShortName, LongName: r.LongName, Mode: r.Mode, Color: r.Color}
	}
	n.stations = stations
	n.routes = routes
	n.loaded = time.Now()
	return nil
}

func (n *nameCache) stationNames(ctx context.Context) map[string]string {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.refresh(ctx)
	return n.stations
}

func (n *nameCache) routeRefs(ctx context.Context) map[string]RouteRef {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.refresh(ctx)
	return n.routes
}

// refresh must be called with n.mu held. It reloads on TTL expiry; a failure
// keeps the stale cache and is logged, since serving last-known names beats
// failing the request.
func (n *nameCache) refresh(ctx context.Context) {
	if err := n.refreshLocked(ctx); err != nil {
		n.log.Warn("refresh name cache", "err", err)
	}
}

// toTrainViewWithContext enriches a TrainState into its public view using the
// name cache, plus a fresh destination lookup when the trip changed.
func (s *Server) toTrainView(ctx context.Context, st models.TrainState) (TrainView, error) {
	names := s.liveNames.stationNames(ctx)
	routes := s.liveNames.routeRefs(ctx)
	var route *RouteRef
	if rr, ok := routes[st.RouteID]; ok {
		route = &rr
	}
	if st.Destination == "" && st.TripID != "" {
		if h, err := s.pg.HeadsignForTrip(ctx, st.TripID); err == nil && h != "" {
			st.Destination = h
		}
	}
	return toTrainView(st, route, names), nil
}

func (s *Server) buildLivePayload(ctx context.Context, st models.TrainState) ([]byte, error) {
	v, err := s.toTrainView(ctx, st)
	if err != nil {
		return nil, err
	}
	return json.Marshal(v)
}

// snapshotProvider lists every active train across agencies, plus IDs pruned
// because their state expired (so SSE can send train_removed).
func (s *Server) snapshotProvider(ctx context.Context) ([]models.TrainState, []string, error) {
	agencies, err := s.pg.ListAgencies(ctx)
	if err != nil {
		return nil, nil, err
	}
	states := []models.TrainState{}
	removed := []string{}
	for _, a := range agencies {
		active, rm, err := s.rd.ListActiveTrains(ctx, a.ID)
		if err != nil {
			continue
		}
		states = append(states, active...)
		removed = append(removed, rm...)
	}
	return states, removed, nil
}

func parseBBox(r *http.Request) *[4]float64 {
	s := r.URL.Query().Get("bbox")
	if s == "" {
		return nil
	}
	parts := split4(s)
	minLon, e1 := strconv.ParseFloat(parts[0], 64)
	minLat, e2 := strconv.ParseFloat(parts[1], 64)
	maxLon, e3 := strconv.ParseFloat(parts[2], 64)
	maxLat, e4 := strconv.ParseFloat(parts[3], 64)
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
		return nil
	}
	return &[4]float64{minLon, minLat, maxLon, maxLat}
}

func split4(s string) []string {
	out := make([]string, 0, 4)
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return out
}
