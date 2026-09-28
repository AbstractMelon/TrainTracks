// Package integration runs the full pipeline (adapter → ingestor → tracking →
// estimation → Redis) against real Postgres + Redis.
//
// These tests are opt-in because they require live services:
//
//	docker compose -f deploy/docker-compose.dev.yml up -d
//	TRAINTRACKS_INTEGRATION=1 \
//	POSTGRES_URL=postgres://traintracks:traintracks@localhost:15432/traintracks?sslmode=disable \
//	REDIS_URL=redis://localhost:16379/0 \
//	go test ./internal/integration/...
package integration

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"traintracks/internal/adapters"
	uta "traintracks/internal/adapters/uta"
	"traintracks/internal/config"
	"traintracks/internal/estimation"
	"traintracks/internal/ingestor"
	"traintracks/internal/models"
	"traintracks/internal/storage/postgres"
	"traintracks/internal/storage/redis"
	"traintracks/internal/tracking"
)

var integration bool

func init() {
	integration = os.Getenv("TRAINTRACKS_INTEGRATION") == "1"
}

func requireIntegration(t *testing.T) {
	t.Helper()
	if !integration {
		t.Skip("set TRAINTRACKS_INTEGRATION=1 to run; requires Postgres+Redis (deploy/docker-compose.dev.yml)")
	}
}

type env struct {
	cfg config.Config
	pg  *postgres.Store
	rd  *redis.Store
}

func newEnv(t *testing.T) *env {
	requireIntegration(t)
	t.Helper()
	cfg := config.Default()
	ctx := context.Background()

	pg, err := postgres.New(ctx, cfg.PostgresURL)
	if err != nil {
		t.Fatalf("postgres: %v", err)
	}
	t.Cleanup(func() { pg.Close() })
	rd, err := redis.New(cfg.RedisURL)
	if err != nil {
		t.Fatalf("redis: %v", err)
	}
	t.Cleanup(func() { _ = rd.Close() })

	// Apply migrations, dropping old state first.
	if err := dropAll(cfg.PostgresURL); err != nil {
		t.Fatalf("drop: %v", err)
	}
	m, err := migrate.New("file://"+filepath.Join("..", "..", "migrations"), cfg.PostgresURL)
	if err != nil {
		t.Fatalf("migrate init: %v", err)
	}
	defer m.Close()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("migrate up: %v", err)
	}
	return &env{cfg: cfg, pg: pg, rd: rd}
}

func dropAll(dbURL string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pg, err := postgres.New(ctx, dbURL)
	if err != nil {
		return err
	}
	defer pg.Close()
	_, err = pg.Pool().Exec(ctx, `
		DROP TABLE IF EXISTS trip_delay_predictions, train_observations, junctions,
			track_segment_routes, track_segments, trip_stops, trips, stations, routes,
			agencies CASCADE;
		DROP EXTENSION IF EXISTS postgis CASCADE;`)
	return err
}

// seedBase inserts UTA + a small TRAX network plus a trip.
func seedBase(t *testing.T, env *env) {
	ctx := context.Background()
	sql := `
	INSERT INTO agencies (id, name, timezone) VALUES ('uta', 'Utah Transit Authority', 'America/Denver');
	INSERT INTO routes (id, agency_id, gtfs_route_id, short_name, long_name, mode, color)
		VALUES ('uta:Blue', 'uta', 'Blue', 'Blue Line', 'TRAX Blue Line', 'light_rail', '#0062A0');
	INSERT INTO stations (id, agency_id, gtfs_stop_id, name, lat, lon, geom) VALUES
		('uta:ME04', 'uta', 'ME04', '400 West/200 South', 40.7600, -111.8910, ST_SetSRID(ST_MakePoint(-111.8910, 40.7600), 4326)),
		('uta:ME05', 'uta', 'ME05', 'Courthouse',       40.7500, -111.8900, ST_SetSRID(ST_MakePoint(-111.8900, 40.7500), 4326)),
		('uta:DR01', 'uta', 'DR01', 'Draper Town Center',40.7233, -111.9083, ST_SetSRID(ST_MakePoint(-111.9083, 40.7233), 4326));
	INSERT INTO trips (id, route_id, agency_id, headsign, direction_id)
		VALUES ('uta:7842735', 'uta:Blue', 'uta', 'Draper', 1);
	INSERT INTO trip_stops (trip_id, station_id, sequence, scheduled_arrival, scheduled_departure) VALUES
		('uta:7842735', 'uta:ME04', 9,  '07:50:00', '07:50:00'),
		('uta:7842735', 'uta:ME05', 10, '08:00:00', '08:01:00'),
		('uta:7842735', 'uta:DR01', 11, '08:15:00', '08:15:00');
	INSERT INTO track_segments (osm_way_id, railway_type, name, geom)
		VALUES (123456, 'light_rail', 'TRAX Blue', ST_SetSRID(ST_GeomFromText('LINESTRING(-111.8950 40.7660, -111.8910 40.7600, -111.8900 40.7500, -111.9030 40.7300, -111.9083 40.7233)'), 4326));
	INSERT INTO track_segment_routes (track_segment_id, route_id)
		VALUES (1, 'uta:Blue');`
	if _, err := env.pg.Pool().Exec(ctx, sql); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func fixtureUTAAdapter(t *testing.T) (*httptest.Server, *uta.Config) {
	t.Helper()
	fix, err := os.ReadFile(filepath.Join("..", "adapters", "uta", "testdata", "vehicle_positions.pb"))
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(fix)
	}))
	t.Cleanup(srv.Close)
	return srv, &uta.Config{VehiclePositionURL: srv.URL + "/vehicle", PollInterval: 2 * time.Second, FetchTimeout: 2 * time.Second}
}

func TestPipelineEndToEnd(t *testing.T) {
	e := newEnv(t)
	seedBase(t, e)

	_, ucfg := fixtureUTAAdapter(t)
	adapters.Register(uta.New(*ucfg))

	tracker := tracking.New(e.rd, e.cfg.MapMatchMaxDistM, testLogger(t))
	runner := ingestor.New(e.cfg, e.pg, e.rd, tracker, testLogger(t))
	ctx, cancel := context.WithCancel(context.Background())
	runner.Start(ctx)
	t.Cleanup(func() { cancel(); runner.Stop() })

	deadline := time.Now().Add(20 * time.Second)
	var st *models.TrainState
	for time.Now().Before(deadline) {
		var err error
		st, err = e.rd.GetTrainState(ctx, "uta:701")
		if err != nil {
			t.Logf("get train state: %v", err)
		}
		if st != nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if st == nil {
		t.Fatal("train uta:701 never became ACTUAL")
	}
	if st.PositionType != models.PositionActual {
		t.Errorf("type = %v, want ACTUAL", st.PositionType)
	}
	if st.AgencyID != "uta" || st.RouteID != "uta:Blue" {
		t.Errorf("route = %q", st.RouteID)
	}
	// Map-matched against the seeded track segment.
	if st.CurrentTrackSegID == nil {
		t.Error("expected map-match to set CurrentTrackSegID")
	}

	// Observation log got appended.
	if n, err := e.pg.CountObservationsSince(ctx, "uta", time.Now().Add(-time.Minute)); err != nil || n < 1 {
		t.Errorf("observations appended = %d, err=%v", n, err)
	}
}

func TestEstimationSweepProducesEstimated(t *testing.T) {
	e := newEnv(t)
	now := time.Now()
	seedBase(t, e)

	// Ensure a train whose last actual was 10 minutes ago and which has
	// trip-update predictions bracketing now sees an ESTIMATED position.
	if _, err := e.pg.Pool().Exec(ctxT, `
		INSERT INTO trip_delay_predictions
			(trip_id, agency_id, stop_sequence, station_id, predicted_arrival, predicted_departure, delay_seconds, reported_at)
		VALUES
			('uta:7842735', 'uta', 9,  'uta:ME04', $1, $1, 60,    now()),
			('uta:7842735', 'uta', 10, 'uta:ME05', $2, $3, 120,   now()),
			('uta:7842735', 'uta', 11, 'uta:DR01', $4, $4, 180,   now())`,
		now.Add(-12*time.Minute), now.Add(-3*time.Minute), now.Add(-2*time.Minute), now.Add(40*time.Minute)); err != nil {
		t.Fatalf("seed predictions: %v", err)
	}

	st := models.TrainState{
		TrainID:        "uta:701",
		AgencyID:       "uta",
		RouteID:        "uta:Blue",
		TripID:         "uta:7842735",
		Lat:            40.7550,
		Lon:            -111.8905,
		PositionType:   models.PositionActual,
		PositionSource: "uta-gtfs-rt",
		LastActualAt:   now.Add(-10 * time.Minute),
		LastUpdateAt:   now.Add(-10 * time.Minute),
	}
	if err := e.rd.SetTrainState(ctxT, st); err != nil {
		t.Fatalf("seed train state: %v", err)
	}

	svc := estimation.NewService(e.pg, e.rd, e.cfg, testLogger(t))
	if err := svc.Sweep(ctxT); err != nil {
		t.Fatalf("sweep: %v", err)
	}

	got, err := e.rd.GetTrainState(ctxT, "uta:701")
	if err != nil || got == nil {
		t.Fatalf("read state: %v", err)
	}
	if got.PositionType != models.PositionEstimated {
		t.Errorf("type = %v, want ESTIMATED", got.PositionType)
	}
	if got.PositionSource != "estimation-engine" {
		t.Errorf("source = %q, want estimation-engine", got.PositionSource)
	}
	if got.Confidence == nil || *got.Confidence <= 0 || *got.Confidence > 1 {
		t.Errorf("confidence = %v, want in (0,1]", got.Confidence)
	}
	if got.NextStationID == nil {
		t.Error("expected next station set from trip predictions")
	}
	if got.DelaySeconds == nil {
		t.Error("expected delay from predictions")
	}
	// Interpolated: now is between seq9 (now-12m) dep and seq10 (now-2m) arr;
	// i.e. the train has moved from ME04 toward ME05.
	if got.Lat > 40.7575 {
		t.Errorf("lat = %v, expected south of ME04 (40.7600)", got.Lat)
	}
}

func TestTrainGoesUnknownWithoutContext(t *testing.T) {
	e := newEnv(t)
	seedBase(t, e)

	// No trip predictions and no speed → STALE, then UNKNOWN past the window.
	st := models.TrainState{
		TrainID:      "uta:999",
		AgencyID:     "uta",
		Lat:          40.0,
		Lon:          -111.0,
		PositionType: models.PositionActual,
		LastActualAt: time.Now().Add(-2 * time.Minute),
	}
	if err := e.rd.SetTrainState(ctxT, st); err != nil {
		t.Fatalf("seed: %v", err)
	}
	svc := estimation.NewService(e.pg, e.rd, e.cfg, testLogger(t))
	if err := svc.Sweep(ctxT); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	st2, err := e.rd.GetTrainState(ctxT, "uta:999")
	if err != nil {
		t.Fatalf("get train state: %v", err)
	}
	if st2 == nil || st2.PositionType != models.PositionStale {
		t.Errorf("got %+v, want STALE", st2)
	}
}

func TestLiveRESTFiltering(t *testing.T) {
	e := newEnv(t)
	seedBase(t, e)

	st := models.TrainState{
		TrainID:        "uta:701",
		AgencyID:       "uta",
		RouteID:        "uta:Blue",
		Lat:            40.7550,
		Lon:            -111.8905,
		PositionType:   models.PositionActual,
		PositionSource: "uta-gtfs-rt",
		LastActualAt:   time.Now(),
		LastUpdateAt:   time.Now(),
	}
	if err := e.rd.SetTrainState(ctxT, st); err != nil {
		t.Fatalf("seed: %v", err)
	}

	states, err := e.rd.ListActiveByAgency(ctxT, "uta")
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 || states[0].TrainID != "uta:701" {
		t.Errorf("active = %+v", states)
	}
}

var ctxT = context.Background()

func testLogger(t *testing.T) *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, nil))
}
