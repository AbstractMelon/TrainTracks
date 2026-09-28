// Package postgres implements static/geographic data access and the append-only
// observation log against PostgreSQL + PostGIS.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"traintracks/internal/models"
)

// Store is the Postgres-backed read/write store.
type Store struct {
	pool *pgxpool.Pool
}

var ErrNotFound = errors.New("not found")

func New(ctx context.Context, url string) (*Store, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close()              { s.pool.Close() }
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// ---------------------------------------------------------------------------
// Static data
// ---------------------------------------------------------------------------

const agencyCols = `id, name, website, gtfs_static_url, gtfs_rt_url, timezone, license_note`

func scanAgency(row pgx.Row) (models.Agency, error) {
	var a models.Agency
	var website, static, rt, license *string
	if err := row.Scan(&a.ID, &a.Name, &website, &static, &rt, &a.Timezone, &license); err != nil {
		return a, err
	}
	a.Website = deref(website)
	a.GTFSStaticURL = deref(static)
	a.GtfsRtURL = deref(rt)
	a.LicenseNote = deref(license)
	return a, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (s *Store) ListAgencies(ctx context.Context) ([]models.Agency, error) {
	rows, err := s.pool.Query(ctx, "SELECT "+agencyCols+" FROM agencies ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Agency{}
	for rows.Next() {
		a, err := scanAgency(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) GetAgency(ctx context.Context, id string) (models.Agency, error) {
	row := s.pool.QueryRow(ctx, "SELECT "+agencyCols+" FROM agencies WHERE id = $1", id)
	a, err := scanAgency(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

const routeCols = `id, agency_id, gtfs_route_id, short_name, long_name, mode, color`

func scanRoute(row pgx.Row) (models.Route, error) {
	var r models.Route
	var short, long, color *string
	if err := row.Scan(&r.ID, &r.AgencyID, &r.GTFSRouteID, &short, &long, &r.Mode, &color); err != nil {
		return r, err
	}
	r.ShortName = deref(short)
	r.LongName = deref(long)
	r.Color = deref(color)
	return r, nil
}

func (s *Store) ListRoutes(ctx context.Context, agencyID string) ([]models.Route, error) {
	q := "SELECT " + routeCols + " FROM routes"
	args := []any{}
	if agencyID != "" {
		q += " WHERE agency_id = $1"
		args = append(args, agencyID)
	}
	q += " ORDER BY id"
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Route{}
	for rows.Next() {
		r, err := scanRoute(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) GetRoute(ctx context.Context, id string) (models.Route, error) {
	r, err := scanRoute(s.pool.QueryRow(ctx, "SELECT "+routeCols+" FROM routes WHERE id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

// RouteStations returns the ordered, de-duplicated station list for a route by
// walking scheduled stop sequences across the route's trips.
func (s *Store) RouteStations(ctx context.Context, routeID string) ([]models.Station, error) {
	q := `
		SELECT DISTINCT ON (s.id) s.id, s.agency_id, s.gtfs_stop_id, s.name, s.lat, s.lon
		FROM trip_stops ts
		JOIN stations s ON s.id = ts.station_id
		JOIN trips t ON t.id = ts.trip_id
		WHERE t.route_id = $1
		ORDER BY s.id, ts.sequence`
	rows, err := s.pool.Query(ctx, q, routeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Station{}
	seen := map[string]bool{}
	var first models.Station
	for rows.Next() {
		var st models.Station
		if err := rows.Scan(&st.ID, &st.AgencyID, &st.GTFSStopID, &st.Name, &st.Lat, &st.Lon); err != nil {
			return nil, err
		}
		if seen[st.ID] {
			continue
		}
		seen[st.ID] = true
		if first.ID != "" {
			out = append(out, st)
		} else {
			first = st
			out = append(out, st)
		}
	}
	return out, rows.Err()
}

// RouteGeometry returns the merged line geometry for a route from its mapped
// track segments, as GeoJSON coordinates ordered along the network.
func (s *Store) RouteGeometry(ctx context.Context, routeID string) ([]byte, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, `
		SELECT ST_AsGeoJSON(ST_LineMerge(ST_Collect(
			ORDER BY tsr.track_segment_id
		)))
		FROM (
			SELECT tsr.track_segment_id, ts.geom
			FROM track_segment_routes tsr
			JOIN track_segments ts ON ts.id = tsr.track_segment_id
			WHERE tsr.route_id = $1
		) tsr`, routeID).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return raw, nil
}

const stationCols = `id, agency_id, gtfs_stop_id, name, lat, lon`

func scanStation(row pgx.Row) (models.Station, error) {
	var st models.Station
	if err := row.Scan(&st.ID, &st.AgencyID, &st.GTFSStopID, &st.Name, &st.Lat, &st.Lon); err != nil {
		return st, err
	}
	return st, nil
}

func (s *Store) ListStations(ctx context.Context, agencyID, query string) ([]models.Station, error) {
	q := "SELECT " + stationCols + " FROM stations"
	conds := []string{}
	args := []any{}
	if agencyID != "" {
		args = append(args, agencyID)
		conds = append(conds, fmt.Sprintf("agency_id = $%d", len(args)))
	}
	if query != "" {
		args = append(args, "%"+query+"%")
		conds = append(conds, fmt.Sprintf("name ILIKE $%d", len(args)))
	}
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	q += " ORDER BY name LIMIT 100"
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Station{}
	for rows.Next() {
		st, err := scanStation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

func (s *Store) GetStation(ctx context.Context, id string) (models.Station, error) {
	st, err := scanStation(s.pool.QueryRow(ctx, "SELECT "+stationCols+" FROM stations WHERE id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return st, ErrNotFound
	}
	return st, err
}

// StationRoutes returns the distinct routes serving a station.
func (s *Store) StationRoutes(ctx context.Context, stationID string) ([]models.Route, error) {
	q := `
		SELECT DISTINCT ` + routeAliasCols + `
		FROM trip_stops ts
		JOIN trips t ON t.id = ts.trip_id
		JOIN routes r ON r.id = t.route_id
		WHERE ts.station_id = $1
		ORDER BY r.id`
	rows, err := s.pool.Query(ctx, q, stationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Route{}
	for rows.Next() {
		var r models.Route
		var short, long, color *string
		if err := rows.Scan(&r.ID, &r.AgencyID, &r.GTFSRouteID, &short, &long, &r.Mode, &color); err != nil {
			return nil, err
		}
		r.ShortName = deref(short)
		r.LongName = deref(long)
		r.Color = deref(color)
		out = append(out, r)
	}
	return out, rows.Err()
}

const routeAliasCols = `r.id, r.agency_id, r.gtfs_route_id, r.short_name, r.long_name, r.mode, r.color`

func (s *Store) GetTrip(ctx context.Context, id string) (models.Trip, error) {
	var t models.Trip
	var service *string
	var dir *int
	err := s.pool.QueryRow(ctx, `SELECT id, route_id, agency_id, service_id, headsign, direction_id FROM trips WHERE id = $1`, id).
		Scan(&t.ID, &t.RouteID, &t.AgencyID, &service, &t.Headsign, &dir)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrNotFound
	}
	t.ServiceID = deref(service)
	t.DirectionID = dir
	return t, err
}

// TripStopsWithStations returns a trip's stops in sequence order, each joined
// with its station row so callers don't need a station query per stop.
func (s *Store) TripStopsWithStations(ctx context.Context, tripID string) ([]models.TripStopStation, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT ts.sequence, s.id, s.agency_id, s.gtfs_stop_id, s.name, s.lat, s.lon
		FROM trip_stops ts JOIN stations s ON s.id = ts.station_id
		WHERE ts.trip_id = $1 ORDER BY ts.sequence`, tripID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.TripStopStation{}
	for rows.Next() {
		var stop models.TripStopStation
		if err := rows.Scan(&stop.Sequence, &stop.Station.ID, &stop.Station.AgencyID,
			&stop.Station.GTFSStopID, &stop.Station.Name, &stop.Station.Lat, &stop.Station.Lon); err != nil {
			return nil, err
		}
		out = append(out, stop)
	}
	return out, rows.Err()
}

// HeadsignForTrip returns the headsign (destination) of a trip, if any.
func (s *Store) HeadsignForTrip(ctx context.Context, tripID string) (string, error) {
	var h *string
	err := s.pool.QueryRow(ctx, `SELECT headsign FROM trips WHERE id = $1`, tripID).Scan(&h)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	return deref(h), nil
}

// TripRouteMap returns trip id → route id for every imported trip. Feeds that
// omit trip.route_id (UTA's vehicle feed does) are resolved through this map.
func (s *Store) TripRouteMap(ctx context.Context) (map[string]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, route_id FROM trips`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var tripID, routeID string
		if err := rows.Scan(&tripID, &routeID); err != nil {
			return nil, err
		}
		out[tripID] = routeID
	}
	return out, rows.Err()
}

// StationArrivals returns the upcoming scheduled/predicted arrivals at a
// station (SPEC.md §11.1 stations/{id}).
func (s *Store) StationArrivals(ctx context.Context, stationID string, now time.Time) ([]models.Arrival, error) {
	type row struct {
		tripID       string
		routeID      string
		headsign     string
		direction    *int
		scheduledArr string
		scheduledDep string
	}
	rows, err := s.pool.Query(ctx, `
		SELECT t.id, t.route_id, COALESCE(t.headsign, r.short_name, ''),
		       t.direction_id, to_char(ts.scheduled_arrival, 'HH24:MI:SS'),
		       to_char(ts.scheduled_departure, 'HH24:MI:SS')
		FROM trip_stops ts
		JOIN trips t ON t.id = ts.trip_id
		JOIN routes r ON r.id = t.route_id
		WHERE ts.station_id = $1
		ORDER BY ts.scheduled_arrival`, stationID)
	if err != nil {
		return nil, err
	}
	var rowsOut []row
	trips := map[string]bool{}
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.tripID, &r.routeID, &r.headsign, &r.direction, &r.scheduledArr, &r.scheduledDep); err != nil {
			rows.Close()
			return nil, err
		}
		rowsOut = append(rowsOut, r)
		trips[r.tripID] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Latest predictions for any of the serving trips, restricted to this
	// station, merged below.
	preds, err := s.LatestTripPredictions(ctx, "", 2*time.Hour)
	if err != nil {
		return nil, err
	}
	predByTrip := map[string]*models.StopTimePrediction{}
	for i := range preds {
		p := preds[i]
		if p.StationID == stationID && trips[p.TripID] {
			if cur, ok := predByTrip[p.TripID]; !ok || p.ReportedAt.After(cur.ReportedAt) {
				predByTrip[p.TripID] = &p
			}
		}
	}

	today := dateAtMidnight(now)
	out := []models.Arrival{}
	for _, r := range rowsOut {
		arr, ok := scheduledTime(today, r.scheduledArr)
		if !ok {
			continue
		}
		// Roll forward to next occurrence from now.
		dep := arr
		if d, ok2 := scheduledTime(today, r.scheduledDep); ok2 {
			dep = d
		}
		if !dep.After(now) {
			arr = arr.Add(24 * time.Hour)
			dep = dep.Add(24 * time.Hour)
		}
		a := models.Arrival{
			TrainID:     r.tripID,
			RouteID:     r.routeID,
			Headsign:    r.headsign,
			Direction:   r.direction,
			ScheduledAt: arr,
		}
		if p, ok := predByTrip[r.tripID]; ok {
			if t := p.PredictedArrival; t != nil {
				a.PredictedAt = t
			}
			if p.DelaySeconds != nil {
				a.DelaySecs = p.DelaySeconds
			}
		}
		out = append(out, a)
	}
	// Keep chronological order regardless of schedule/prediction mix.
	sort.Slice(out, func(i, j int) bool {
		t := out[i].ScheduledAt
		if out[i].PredictedAt != nil {
			t = *out[i].PredictedAt
		}
		t2 := out[j].ScheduledAt
		if out[j].PredictedAt != nil {
			t2 = *out[j].PredictedAt
		}
		return t.Before(t2)
	})
	if len(out) > 10 {
		out = out[:10]
	}
	return out, nil
}

func dateAtMidnight(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

func scheduledTime(midnight time.Time, hhmmss string) (time.Time, bool) {
	if hhmmss == "" {
		return time.Time{}, false
	}
	parts := strings.Split(hhmmss, ":")
	if len(parts) != 3 {
		return time.Time{}, false
	}
	hh, err1 := strconv.Atoi(parts[0])
	mm, err2 := strconv.Atoi(parts[1])
	ss, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return time.Time{}, false
	}
	return midnight.Add(time.Duration(hh)*time.Hour + time.Duration(mm)*time.Minute + time.Duration(ss)*time.Second), true
}

// ---------------------------------------------------------------------------
// Route shape geometry. See SPEC.md §8
// ---------------------------------------------------------------------------

const (
	// maxStationDeviationM rejects a stop that does not sit on the route's
	// shape. Measured over live trips: p95 ≈ 17 m, max ≈ 65 m.
	maxStationDeviationM = 100.0
	// minPathChordRatio rejects a stop pair whose shape substring comes out
	// shorter than the straight line between the stops, which happens when both
	// stops project onto the same pass of a looping route.
	minPathChordRatio = 0.9
)

// RoutePathBetween returns the route's shape geometry running between two
// stations, oriented from → to, so estimation can interpolate along the line
// instead of a straight chord (SPEC.md §7). It returns nil when the route has
// no shape or either stop is not on it; callers fall back to the chord.
func (s *Store) RoutePathBetween(ctx context.Context, routeID, fromStationID, toStationID string) ([]models.Point, error) {
	if routeID == "" || fromStationID == "" || toStationID == "" {
		return nil, nil
	}
	const q = `
		WITH frac AS (
			SELECT s.geom,
			       ST_LineLocatePoint(s.geom, a.geom) AS fa,
			       ST_LineLocatePoint(s.geom, b.geom) AS fb,
			       ST_Distance(s.geom::geography, a.geom::geography) AS da,
			       ST_Distance(s.geom::geography, b.geom::geography) AS db,
			       ST_Distance(a.geom::geography, b.geom::geography) AS chord
			FROM route_shapes rs
			JOIN shapes s ON s.id = rs.shape_id
			JOIN stations a ON a.id = $2
			JOIN stations b ON b.id = $3
			WHERE rs.route_id = $1
		),
		cand AS (
			SELECT ST_LineSubstring(geom, LEAST(fa, fb), GREATEST(fa, fb)) AS sub,
			       (fa > fb) AS reversed, da, db, chord
			FROM frac
			-- fa == fb yields a degenerate substring that PostGIS renders as a
			-- Point, not a LineString; the stops are one place, so let the
			-- caller fall back to the chord.
			WHERE fa <> fb
		),
		best AS (
			-- A route carries one shape per direction; keep the one both stops
			-- actually lie on.
			SELECT * FROM cand ORDER BY da + db LIMIT 1
		)
		SELECT ST_AsGeoJSON(sub), reversed, da, db, chord, ST_Length(sub::geography)
		FROM best`
	var geoJSON string
	var reversed bool
	var da, db, chord, pathLen float64
	err := s.pool.QueryRow(ctx, q, routeID, fromStationID, toStationID).
		Scan(&geoJSON, &reversed, &da, &db, &chord, &pathLen)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if da > maxStationDeviationM || db > maxStationDeviationM {
		return nil, nil
	}
	if chord > 0 && pathLen < minPathChordRatio*chord {
		return nil, nil
	}
	pts, err := lineStringPoints(geoJSON)
	if err != nil {
		return nil, err
	}
	if len(pts) < 2 {
		return nil, nil
	}
	if reversed {
		for i, j := 0, len(pts)-1; i < j; i, j = i+1, j-1 {
			pts[i], pts[j] = pts[j], pts[i]
		}
	}
	return pts, nil
}

// lineStringPoints decodes a GeoJSON LineString (lon,lat pairs) into points.
func lineStringPoints(raw string) ([]models.Point, error) {
	var ls struct {
		Coordinates [][]float64 `json:"coordinates"`
	}
	if err := json.Unmarshal([]byte(raw), &ls); err != nil {
		return nil, fmt.Errorf("decode route path: %w", err)
	}
	out := make([]models.Point, 0, len(ls.Coordinates))
	for _, c := range ls.Coordinates {
		if len(c) < 2 {
			return nil, fmt.Errorf("decode route path: short coordinate %v", c)
		}
		out = append(out, models.Point{Lat: c[1], Lon: c[0]})
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Track segments (map matching). See SPEC.md §8
// ---------------------------------------------------------------------------

// MapMatch finds the nearest track segment within maxDist meters of (lat,lon)
// that is mapped to routeID, and returns chainage along it.
func (s *Store) MapMatch(ctx context.Context, routeID string, lat, lon, maxDist float64) (*models.TrackPoint, error) {
	q := `
		SELECT ts.id, ST_LineLocatePoint(ts.geom, ST_SetSRID(ST_MakePoint($3, $2), 4326)) AS frac,
		       ST_Distance(ts.geom, ST_SetSRID(ST_MakePoint($3, $2), 4326)::geography) AS dist,
		       ST_AsGeoJSON(ts.geom)
		FROM track_segments ts
		JOIN track_segment_routes tsr ON tsr.track_segment_id = ts.id
		WHERE tsr.route_id = $1
		  AND ST_DWithin(ts.geom, ST_SetSRID(ST_MakePoint($3, $2), 4326)::geography, $4)
		ORDER BY dist
		LIMIT 1`
	var tp models.TrackPoint
	var frac, dist float64
	var geog string
	err := s.pool.QueryRow(ctx, q, routeID, lat, lon, maxDist).
		Scan(&tp.SegmentID, &frac, &dist, &geog)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	tp.SegmentFrac = frac
	tp.Distance = dist
	tp.SegmentGeog = geog
	chain, err := s.segmentLengthMeters(ctx, tp.SegmentID)
	if err != nil {
		return nil, err
	}
	tp.Chainage = frac * chain
	return &tp, nil
}

func (s *Store) segmentLengthMeters(ctx context.Context, id int64) (float64, error) {
	var m float64
	err := s.pool.QueryRow(ctx, `SELECT ST_Length(geom::geography) FROM track_segments WHERE id = $1`, id).Scan(&m)
	return m, err
}

// SegmentBetween find the linear distance along a segment between two fractions.
func (s *Store) SegmentLengthBetweenMeters(ctx context.Context, id int64, fracA, fracB float64) (float64, error) {
	var m float64
	err := s.pool.QueryRow(ctx, `
		SELECT ABS($2 - $3) * ST_Length(geom::geography)
		FROM track_segments WHERE id = $1`, id, fracA, fracB).Scan(&m)
	return m, err
}

// LineFractionAtDistance returns the fraction along a segment after traveling a
// signed distance (meters) from frac, clamping to [0,1].
func (s *Store) InterpolateFraction(ctx context.Context, id int64, frac, deltaM float64) (float64, error) {
	var length float64
	err := s.pool.QueryRow(ctx, `SELECT ST_Length(geom::geography) FROM track_segments WHERE id = $1`, id).Scan(&length)
	if err != nil {
		return 0, err
	}
	if length <= 0 {
		return frac, nil
	}
	f := frac + deltaM/length
	if f < 0 {
		return 0, nil
	}
	if f > 1 {
		return 1, nil
	}
	return f, nil
}

// GeoJSONPointAtFraction projects a point at fraction frac along a segment, as
// GeoJSON coordinates [lon, lat].
func (s *Store) PointAtFraction(ctx context.Context, id int64, frac float64) ([]float64, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, `
		SELECT ST_AsGeoJSON(ST_LineInterpolatePoint(geom, $2))
		FROM track_segments WHERE id = $1`, id, frac).Scan(&raw)
	if err != nil {
		return nil, err
	}
	var pt []float64
	if err := json.Unmarshal(raw, &pt); err != nil {
		return nil, err
	}
	return pt, nil
}

// ListTracks returns segments intersecting a bbox (minLon, minLat, maxLon, maxLat).
func (s *Store) ListTracks(ctx context.Context, minLon, minLat, maxLon, maxLat float64) ([]models.TrackSegment, error) {
	q := `
		SELECT id, osm_way_id, railway_type, name,
		       ST_AsGeoJSON(ST_Transform(geom, 4326))::jsonb->'coordinates'
		FROM track_segments
		WHERE geom && ST_MakeEnvelope($1, $2, $3, $4, 4326)
		ORDER BY id`
	rows, err := s.pool.Query(ctx, q, minLon, minLat, maxLon, maxLat)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.TrackSegment{}
	for rows.Next() {
		var ts models.TrackSegment
		var line []byte
		var way *int64
		var name *string
		if err := rows.Scan(&ts.ID, &way, &ts.RailwayType, &name, &line); err != nil {
			return nil, err
		}
		if way != nil {
			ts.OSMWayID = way
		}
		ts.Name = deref(name)
		var coords [][]float64
		if err := json.Unmarshal(line, &coords); err != nil {
			return nil, err
		}
		ts.Line = coords
		out = append(out, ts)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Append-only observation log. See SPEC.md §5, §9
// ---------------------------------------------------------------------------

func (s *Store) AppendObservation(ctx context.Context, o models.TrainObservation) error {
	speed, heading := nullableFloat(o.Speed), nullableFloat(o.Heading)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO train_observations (agency_id, train_id, route_id, trip_id, lat, lon, speed, heading,
			observed_at, received_at, source, raw)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		o.AgencyID, o.VehicleID, nullStr(o.RouteID), nullStr(o.TripID),
		o.Lat, o.Lon, speed, heading,
		o.Timestamp, o.ObservedAt, o.Source, o.Raw)
	return err
}

// CountObservationsSince reports raw observations since t. Used by /status and
// tests.
func (s *Store) CountObservationsSince(ctx context.Context, agencyID string, since time.Time) (int64, error) {
	var n int64
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM train_observations
		WHERE agency_id = $1 AND received_at > $2`, agencyID, since).Scan(&n)
	return n, err
}

// ---------------------------------------------------------------------------
// GTFS-RT trip-update predictions. See SPEC.md §7
// ---------------------------------------------------------------------------

// UpsertTripPrediction stores the newest predicted stop time for a trip stop.
// Only the latest report per (trip, sequence) is ever read, so a repeat report
// replaces the row and an out-of-order (older) one is ignored.
func (s *Store) UpsertTripPrediction(ctx context.Context, p models.StopTimePrediction) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO trip_delay_predictions (trip_id, agency_id, stop_sequence, station_id,
			predicted_arrival, predicted_departure, delay_seconds, reported_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (trip_id, stop_sequence) DO UPDATE SET
			predicted_arrival = EXCLUDED.predicted_arrival,
			predicted_departure = EXCLUDED.predicted_departure,
			delay_seconds = EXCLUDED.delay_seconds,
			reported_at = EXCLUDED.reported_at
		WHERE trip_delay_predictions.reported_at <= EXCLUDED.reported_at`,
		p.TripID, p.AgencyID, p.Sequence, nullStr(p.StationID),
		nullTime(p.PredictedArrival), nullTime(p.PredictedDeparture),
		nullInt(p.DelaySeconds), p.ReportedAt)
	return err
}

// LatestTripPredictions returns the most recent prediction per (trip, sequence),
// filtered to the given age window.
func (s *Store) LatestTripPredictions(ctx context.Context, agencyID string, maxAge time.Duration) ([]models.StopTimePrediction, error) {
	q := `
		SELECT DISTINCT ON (trip_id, stop_sequence) trip_id, stop_sequence, station_id,
			predicted_arrival, predicted_departure, delay_seconds, reported_at
		FROM trip_delay_predictions
		WHERE reported_at > now() - $1::interval`
	args := []any{maxAge.String()}
	if agencyID != "" {
		q += " AND agency_id = $2"
		args = append(args, agencyID)
	}
	q += " ORDER BY trip_id, stop_sequence, reported_at DESC"
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.StopTimePrediction{}
	for rows.Next() {
		var p models.StopTimePrediction
		var st *string
		var ar, dp *time.Time
		var delay *int
		if err := rows.Scan(&p.TripID, &p.Sequence, &st, &ar, &dp, &delay, &p.ReportedAt); err != nil {
			return nil, err
		}
		p.AgencyID = agencyID
		p.StationID = deref(st)
		p.PredictedArrival = ar
		p.PredictedDeparture = dp
		p.DelaySeconds = delay
		out = append(out, p)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// GTFS static import + route shapes. See SPEC.md §8, §11.1 /routes/geo
// ---------------------------------------------------------------------------

// StaticDataset is everything cmd/gtfs-import writes for one agency.
type StaticDataset struct {
	Agency      models.Agency
	Routes      []models.Route
	Stations    []models.Station
	Trips       []models.Trip
	TripStops   []models.TripStop
	Shapes      []models.Shape
	RouteShapes []models.RouteShape
}

// LoadStatic upserts one agency's GTFS static dataset in a single transaction.
// Idempotent: re-running refreshes routes/stops/trips/shapes from a fresh feed
// without duplicating rows.
func (s *Store) LoadStatic(ctx context.Context, d StaticDataset) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	a := d.Agency
	if a.ID != "" {
		if _, err := tx.Exec(ctx, `
			INSERT INTO agencies (id, name, website, gtfs_static_url, gtfs_rt_url, timezone, license_note)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (id) DO UPDATE SET
				name = EXCLUDED.name,
				website = EXCLUDED.website,
				timezone = EXCLUDED.timezone`,
			a.ID, a.Name, a.Website, a.GTFSStaticURL, a.GtfsRtURL, a.Timezone, a.LicenseNote); err != nil {
			return err
		}
	}

	for _, r := range d.Routes {
		if _, err := tx.Exec(ctx, `
			INSERT INTO routes (id, agency_id, gtfs_route_id, short_name, long_name, mode, color)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (id) DO UPDATE SET
				short_name = EXCLUDED.short_name,
				long_name = EXCLUDED.long_name,
				mode = EXCLUDED.mode,
				color = EXCLUDED.color`,
			r.ID, r.AgencyID, r.GTFSRouteID, nullStr(r.ShortName), nullStr(r.LongName), r.Mode, nullStr(r.Color)); err != nil {
			return err
		}
	}

	for _, st := range d.Stations {
		if _, err := tx.Exec(ctx, `
			INSERT INTO stations (id, agency_id, gtfs_stop_id, name, lat, lon, geom)
			VALUES ($1, $2, $3, $4, $5, $6, ST_SetSRID(ST_MakePoint($6, $5), 4326))
			ON CONFLICT (id) DO UPDATE SET
				name = EXCLUDED.name,
				lat = EXCLUDED.lat,
				lon = EXCLUDED.lon,
				geom = EXCLUDED.geom`,
			st.ID, st.AgencyID, st.GTFSStopID, st.Name, st.Lat, st.Lon); err != nil {
			return err
		}
	}

	for _, t := range d.Trips {
		if _, err := tx.Exec(ctx, `
			INSERT INTO trips (id, route_id, agency_id, service_id, headsign, direction_id)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (id) DO UPDATE SET
				service_id = EXCLUDED.service_id,
				headsign = EXCLUDED.headsign,
				direction_id = EXCLUDED.direction_id`,
			t.ID, t.RouteID, t.AgencyID, nullStr(t.ServiceID), nullStr(t.Headsign), t.DirectionID); err != nil {
			return err
		}
	}

	for _, sh := range d.Shapes {
		line := shapeLineString(sh.Line)
		if line == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO shapes (id, agency_id, gtfs_shape_id, geom)
			VALUES ($1, $2, $3, ST_GeomFromText($4, 4326))
			ON CONFLICT (id) DO UPDATE SET geom = EXCLUDED.geom`,
			sh.ID, sh.AgencyID, sh.GTFSShapeID, line); err != nil {
			return err
		}
	}

	// Rebuild route→shape links from the current trips (routes without shapes
	// drop out on reload).
	if _, err := tx.Exec(ctx, `
		DELETE FROM route_shapes rs USING routes r
		WHERE rs.route_id = r.id AND r.agency_id = $1`, a.ID); err != nil {
		return err
	}
	for _, rs := range d.RouteShapes {
		if _, err := tx.Exec(ctx, `
			INSERT INTO route_shapes (route_id, shape_id) VALUES ($1, $2)
			ON CONFLICT DO NOTHING`, rs.RouteID, rs.ShapeID); err != nil {
			return err
		}
	}

	if err := s.importTripStops(ctx, tx, d.TripStops); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// importTripStops bulk-loads GTFS stop_times via a temp table, then merges into
// trip_stops. CopyFrom skips FK/uniqueness checks, so rows can arrive in any
// order and duplicates resolve to upserts.
func (s *Store) importTripStops(ctx context.Context, tx pgx.Tx, stops []models.TripStop) error {
	if len(stops) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE trip_stops_import (
			trip_id text, station_id text, sequence int, arrival text, departure text
		) ON COMMIT DROP`); err != nil {
		return err
	}
	rows := make([][]any, 0, len(stops))
	for _, ts := range stops {
		arr, dep := "", ""
		if ts.ScheduledArrival != nil {
			arr = *ts.ScheduledArrival
		}
		if ts.ScheduledDeparture != nil {
			dep = *ts.ScheduledDeparture
		}
		rows = append(rows, []any{ts.TripID, ts.StationID, ts.Sequence, arr, dep})
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"trip_stops_import"},
		[]string{"trip_id", "station_id", "sequence", "arrival", "departure"},
		pgx.CopyFromRows(rows)); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO trip_stops (trip_id, station_id, sequence, scheduled_arrival, scheduled_departure)
		SELECT trip_id, station_id, sequence,
		       NULLIF(arrival, '')::time, NULLIF(departure, '')::time
		FROM trip_stops_import
		ON CONFLICT (trip_id, sequence)
		DO UPDATE SET
			station_id = EXCLUDED.station_id,
			scheduled_arrival = EXCLUDED.scheduled_arrival,
			scheduled_departure = EXCLUDED.scheduled_departure`)
	return err
}

// shapeLineString renders a [lon, lat] polyline as a WKT LINESTRING for
// ST_GeomFromText. Returns "" for degenerate (empty / single-point) lines.
func shapeLineString(line [][]float64) string {
	if len(line) < 2 {
		return ""
	}
	var b strings.Builder
	b.WriteString("LINESTRING(")
	for i, p := range line {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(strconv.FormatFloat(p[0], 'f', 6, 64))
		b.WriteString(" ")
		b.WriteString(strconv.FormatFloat(p[1], 'f', 6, 64))
	}
	b.WriteString(")")
	return b.String()
}

// RouteShapeFeatures returns route shape polylines as GeoJSON features,
// optionally filtered by agency and intersected by a bbox
// (minLon, minLat, maxLon, maxLat).
func (s *Store) RouteShapeFeatures(ctx context.Context, agency string, bbox *[4]float64) ([]models.RouteShapeFeature, error) {
	q := `
		SELECT r.id, r.agency_id, r.gtfs_route_id, r.short_name, r.long_name, r.mode, r.color,
		       sh.id, sh.gtfs_shape_id, ST_AsGeoJSON(sh.geom)::jsonb->'coordinates'
		FROM route_shapes rs
		JOIN routes r ON r.id = rs.route_id
		JOIN shapes sh ON sh.id = rs.shape_id`
	conds := []string{}
	args := []any{}
	if agency != "" {
		args = append(args, agency)
		conds = append(conds, fmt.Sprintf("r.agency_id = $%d", len(args)))
	}
	if bbox != nil {
		args = append(args, bbox[0], bbox[1], bbox[2], bbox[3])
		n := len(args)
		conds = append(conds, fmt.Sprintf("sh.geom && ST_MakeEnvelope($%d, $%d, $%d, $%d, 4326)", n-3, n-2, n-1, n))
	}
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	q += " ORDER BY r.id, sh.id"

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.RouteShapeFeature{}
	for rows.Next() {
		var id, agencyID, gtfsRouteID, shapeID, gtfsShapeID, mode string
		var short, long, color *string
		var geom []byte
		if err := rows.Scan(&id, &agencyID, &gtfsRouteID, &short, &long, &mode, &color, &shapeID, &gtfsShapeID, &geom); err != nil {
			return nil, err
		}
		out = append(out, models.RouteShapeFeature{
			Type: "Feature",
			ID:   id + ":" + gtfsShapeID,
			Geometry: models.GeoJSON{
				Type:        "LineString",
				Coordinates: json.RawMessage(geom),
			},
			Properties: models.RouteShapeProps{
				ID:        id,
				AgencyID:  agencyID,
				ShortName: deref(short),
				LongName:  deref(long),
				Mode:      mode,
				Color:     deref(color),
				ShapeID:   gtfsShapeID,
			},
		})
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Unified search. See SPEC.md §11.1 GET /api/v1/search
// ---------------------------------------------------------------------------

type SearchResult struct {
	Stations []models.Station
	Routes   []models.Route
}

func (s *Store) Search(ctx context.Context, q string) (SearchResult, error) {
	sts, err := s.ListStations(ctx, "", q)
	if err != nil {
		return SearchResult{}, err
	}
	rq := `SELECT ` + routeCols + ` FROM routes WHERE short_name ILIKE $1 OR long_name ILIKE $1 ORDER BY id LIMIT 20`
	rows, err := s.pool.Query(ctx, rq, "%"+q+"%")
	if err != nil {
		return SearchResult{}, err
	}
	defer rows.Close()
	var routes []models.Route
	for rows.Next() {
		r, err := scanRoute(rows)
		if err != nil {
			return SearchResult{}, err
		}
		routes = append(routes, r)
	}
	return SearchResult{Stations: sts, Routes: routes}, rows.Err()
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullableFloat(f *float64) any {
	if f == nil {
		return nil
	}
	return *f
}

func nullTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}

func nullInt(i *int) any {
	if i == nil {
		return nil
	}
	return *i
}
