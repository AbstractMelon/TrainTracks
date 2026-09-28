package uta

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func abs(f float64) float64 { return math.Abs(f) }

func fixtureServer(t *testing.T, paths map[string]string) (*httptest.Server, Config) {
	t.Helper()
	mux := http.NewServeMux()
	for route, file := range paths {
		data, err := os.ReadFile(filepath.Join("testdata", file))
		if err != nil {
			t.Fatalf("read fixture: %v", err)
		}
		mux.Handle(route, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(data)
		}))
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, Config{PollInterval: 5 * time.Second, FetchTimeout: 2 * time.Second}
}

func TestFetchVehiclePositions(t *testing.T) {
	srv, cfg := fixtureServer(t, map[string]string{"/vehicle.pb": "vehicle_positions.pb"})
	cfg.VehiclePositionURL = srv.URL + "/vehicle.pb"
	a := New(cfg)

	obs, err := a.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(obs) != 2 {
		t.Fatalf("observations = %d, want 2", len(obs))
	}
	o := obs[0]
	if o.AgencyID != AgencyID {
		t.Errorf("agency = %q, want %q", o.AgencyID, AgencyID)
	}
	if o.VehicleID != "701" {
		t.Errorf("vehicle = %q, want 701", o.VehicleID)
	}
	if o.RouteID != AgencyID+":Blue" {
		t.Errorf("route = %q, want uta:Blue", o.RouteID)
	}
	if o.TripID != AgencyID+":7842735" {
		t.Errorf("trip = %q, want uta:7842735", o.TripID)
	}
	if abs(o.Lat-40.7233) > 1e-4 || abs(o.Lon-(-111.9083)) > 1e-4 {
		t.Errorf("pos = (%v,%v), want lat 40.7233 lon -111.9083", o.Lat, o.Lon)
	}
	// 15.2 mph → ~6.79 m/s.
	if o.Speed == nil || *o.Speed < 6.7 || *o.Speed > 6.9 {
		t.Errorf("speed = %v, want ~6.79", o.Speed)
	}
	if o.Heading == nil || *o.Heading != 182.5 {
		t.Errorf("heading = %v, want 182.5", o.Heading)
	}
	if !o.Timestamp.Equal(time.Unix(1700000000, 0)) {
		t.Errorf("timestamp = %v", o.Timestamp)
	}
	if o.Source != "uta-gtfs-rt" {
		t.Errorf("source = %q", o.Source)
	}
}

func TestFetchTripUpdates(t *testing.T) {
	srv, cfg := fixtureServer(t, map[string]string{"/trip.pb": "trip_updates.pb"})
	cfg.TripUpdateURL = srv.URL + "/trip.pb"
	a := New(cfg)

	preds, err := a.FetchTripUpdates(context.Background())
	if err != nil {
		t.Fatalf("FetchTripUpdates: %v", err)
	}
	if len(preds) != 3 {
		t.Fatalf("predictions = %d, want 3", len(preds))
	}
	p := preds[0]
	if p.TripID != AgencyID+":7842735" {
		t.Errorf("trip = %q", p.TripID)
	}
	if p.Sequence != 9 {
		t.Errorf("sequence = %d, want 9", p.Sequence)
	}
	if p.StationID != AgencyID+":ME04" {
		t.Errorf("station = %q", p.StationID)
	}
	if p.DelaySeconds == nil || *p.DelaySeconds != 180 {
		t.Errorf("delay = %v, want 180", p.DelaySeconds)
	}
	if p.PredictedArrival == nil || !p.PredictedArrival.Equal(time.Unix(1700000120, 0)) {
		t.Errorf("predicted arrival = %v", p.PredictedArrival)
	}
}

func TestFetchEmptyFeedReturnsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	a := New(Config{VehiclePositionURL: srv.URL, FetchTimeout: 2 * time.Second})
	obs, err := a.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(obs) != 0 {
		t.Errorf("observations = %d, want 0", len(obs))
	}
}

func TestFetchHTTPErrorPropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	a := New(Config{VehiclePositionURL: srv.URL, FetchTimeout: 2 * time.Second})
	if _, err := a.Fetch(context.Background()); err == nil {
		t.Fatal("expected error for 500 response")
	}
}
