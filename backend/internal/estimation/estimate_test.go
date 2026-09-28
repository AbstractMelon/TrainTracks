package estimation

import (
	"testing"
	"time"

	"traintracks/internal/models"
)

const (
	fresh = models.FreshWindow
	maxW  = models.MaxEstimationWindow
)

func TestTripUpdateConfidence(t *testing.T) {
	cases := []struct {
		age  time.Duration
		want float64
	}{
		{0, 1},
		{fresh, 1},
		{fresh + (maxW-fresh)/2, 1 - 0.3},
		{maxW, 0},
		{maxW + 5*time.Second, 0},
	}
	for _, c := range cases {
		got := TripUpdateConfidence(c.age, fresh, maxW)
		if got != c.want {
			t.Errorf("TripUpdateConfidence(%v) = %v, want %v", c.age, got, c.want)
		}
	}
}

func TestDeadReckonConfidence(t *testing.T) {
	cases := []struct {
		age  time.Duration
		want float64
	}{
		{fresh, 1},
		{fresh + (maxW-fresh)/2, 0.5},
		{maxW, 0},
	}
	for _, c := range cases {
		got := DeadReckonConfidence(c.age, fresh, maxW)
		if got != c.want {
			t.Errorf("DeadReckonConfidence(%v) = %v, want %v", c.age, got, c.want)
		}
	}
}

func TestTripTimingWeight(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	dep := now.Add(-5 * time.Minute)
	arr := now.Add(5 * time.Minute)
	tt := TripTiming{
		Prev: BracketingStop{Departure: &dep},
		Next: BracketingStop{Arrival: &arr},
		Now:  now,
	}
	if w := tt.Weight(); w != 0.5 {
		t.Errorf("weight = %v, want 0.5", w)
	}

	// Clamp for early/late.
	tt.Now = now.Add(-10 * time.Minute)
	if w := tt.Weight(); w != 0 {
		t.Errorf("early weight = %v, want 0", w)
	}
	tt.Now = now.Add(10 * time.Minute)
	if w := tt.Weight(); w != 1 {
		t.Errorf("late weight = %v, want 1", w)
	}
}

func TestInterpolateBetweenStopsFollowsPath(t *testing.T) {
	// A dogleg: the straight chord cuts the corner, the route geometry doesn't.
	tt := &TripTiming{
		Prev: BracketingStop{StationID: "uta:a", Lat: 40.0, Lon: -111.0},
		Next: BracketingStop{StationID: "uta:b", Lat: 40.1, Lon: -110.0},
		Path: []models.Point{{Lat: 40.0, Lon: -111.0}, {Lat: 40.1, Lon: -111.0}, {Lat: 40.1, Lon: -110.0}},
	}
	lat, lon := interpolateBetweenStops(tt, 0.5)
	// Halfway by distance is on the north leg, well north of the chord's 40.05.
	if lat < 40.099 {
		t.Errorf("lat = %v, want on the north leg (~40.1), not the chord (40.05)", lat)
	}
	if lon < -111.0 || lon > -110.0 {
		t.Errorf("lon = %v, want within the path bounds", lon)
	}

	lat0, lon0 := interpolateBetweenStops(tt, 0)
	if lat0 != 40.0 || lon0 != -111.0 {
		t.Errorf("start = %v,%v, want 40,-111", lat0, lon0)
	}
	lat1, lon1 := interpolateBetweenStops(tt, 1)
	if lat1 != 40.1 || lon1 != -110.0 {
		t.Errorf("end = %v,%v, want 40.1,-110", lat1, lon1)
	}
}

func TestInterpolateBetweenStopsFallsBackToChord(t *testing.T) {
	tt := &TripTiming{
		Prev: BracketingStop{StationID: "uta:a", Lat: 40.0, Lon: -111.0},
		Next: BracketingStop{StationID: "uta:b", Lat: 40.2, Lon: -111.0},
		Path: []models.Point{{Lat: 40.0, Lon: -111.0}}, // no usable geometry
	}
	lat, lon := interpolateBetweenStops(tt, 0.5)
	if lat != 40.1 || lon != -111.0 {
		t.Errorf("point = %v,%v, want chord midpoint 40.1,-111", lat, lon)
	}
}

func TestEvaluateTripUpdate(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	dep := now.Add(-2 * time.Minute)
	arr := now.Add(2 * time.Minute)
	tt := &TripTiming{
		Prev: BracketingStop{StationID: "uta:a", Lat: 40.0, Lon: -111.0, Departure: &dep},
		Next: BracketingStop{StationID: "uta:b", Lat: 40.1, Lon: -111.0, Arrival: &arr},
		Now:  now,
	}
	out := Evaluate(Input{
		State: models.TrainState{
			TrainID: "uta:1", Lat: 40.0, Lon: -111.0,
			PositionType: models.PositionActual, PositionSource: "uta-gtfs-rt",
			LastActualAt: now.Add(-3 * time.Minute),
		},
		Age:           3 * time.Minute,
		Trips:         tt,
		SpeedEstimate: 20,
		FreshWindow:   fresh, MaxWindow: maxW, MinConfidence: models.MinConfidence,
	})
	if out.PositionType != models.PositionEstimated {
		t.Errorf("type = %v, want ESTIMATED", out.PositionType)
	}
	if out.Confidence == nil {
		t.Fatal("confidence should be set")
	}
	if out.Lat != 40.05 {
		t.Errorf("lat = %v, want interpolated 40.05", out.Lat)
	}
	if out.CurrentStationID == nil || *out.CurrentStationID != "uta:a" {
		t.Errorf("current station = %v, want uta:a", out.CurrentStationID)
	}
	if out.NextStationID == nil || *out.NextStationID != "uta:b" {
		t.Errorf("next station = %v, want uta:b", out.NextStationID)
	}
}

func TestEvaluateDeadReckoning(t *testing.T) {
	heading := 90.0 // east
	speed := 10.0
	st := models.TrainState{
		TrainID: "uta:2", Lat: 40.0, Lon: -111.0,
		Heading: &heading, Speed: &speed,
		LastActualAt: time.Now().Add(-2 * time.Minute),
	}
	out := Evaluate(Input{
		State:         st,
		Age:           2 * time.Minute,
		SpeedEstimate: speed,
		FreshWindow:   fresh, MaxWindow: maxW, MinConfidence: models.MinConfidence,
	})
	if out.PositionType != models.PositionEstimated {
		t.Errorf("type = %v, want ESTIMATED", out.PositionType)
	}
	// Moving east at ~10 m/s for 120s ≈ 1200m ≈ 0.0108 longitude deg at lat 40.
	if out.Lon <= -111.0 {
		t.Errorf("lon = %v, want > previous lon (east movement)", out.Lon)
	}
	if out.Confidence == nil || *out.Confidence <= 0 {
		t.Errorf("confidence = %v, want positive", out.Confidence)
	}
}

func TestEvaluateStale(t *testing.T) {
	st := models.TrainState{
		TrainID: "uta:3", Lat: 40.0, Lon: -111.0,
		LastActualAt: time.Now().Add(-2 * time.Minute),
	}
	out := Evaluate(Input{
		State:         st,
		Age:           2 * time.Minute,
		SpeedEstimate: 0, // no context to project
		FreshWindow:   fresh, MaxWindow: maxW, MinConfidence: models.MinConfidence,
	})
	if out.PositionType != models.PositionStale {
		t.Errorf("type = %v, want STALE", out.PositionType)
	}
	if out.Confidence != nil {
		t.Errorf("stale confidence should be nil, got %v", *out.Confidence)
	}
}

func TestEvaluateUnknownBeyondWindow(t *testing.T) {
	st := models.TrainState{
		TrainID: "uta:4", Lat: 40.0, Lon: -111.0,
		LastActualAt: time.Now().Add(-maxW - time.Minute),
	}
	out := Evaluate(Input{
		State:         st,
		Age:           maxW + time.Minute,
		SpeedEstimate: 20, // even with context, too old
		FreshWindow:   fresh, MaxWindow: maxW, MinConfidence: models.MinConfidence,
	})
	if out.PositionType != models.PositionUnknown {
		t.Errorf("type = %v, want UNKNOWN", out.PositionType)
	}
	if out.Confidence != nil {
		t.Errorf("unknown confidence should be nil, got %v", *out.Confidence)
	}
}

func TestEvaluateLowConfidenceUnknown(t *testing.T) {
	// Low-confidence dead-reckoning should flip to UNKNOWN at max window
	// boundary where f_trip_update < MinConfidence.
	st := models.TrainState{
		TrainID: "uta:5", Lat: 40.0, Lon: -111.0,
		LastActualAt: time.Now().Add(-maxW),
	}
	age := maxW - 1*time.Second // 0.1 => ~1-0.999*... dead reckoning < 0.15
	out := Evaluate(Input{
		State:         st,
		Age:           age,
		SpeedEstimate: 20,
		FreshWindow:   fresh, MaxWindow: maxW, MinConfidence: models.MinConfidence,
	})
	// DeadReckonConfidence(age≈maxW) ≈ 0 < MinConfidence → UNKNOWN.
	if out.PositionType != models.PositionUnknown {
		t.Errorf("type = %v, want UNKNOWN (confidence below min)", out.PositionType)
	}
}
