package estimation

import (
	"testing"
	"time"

	"traintracks/internal/geo"
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
		PositionType: models.PositionActual, PositionSource: "uta-gtfs-rt",
		LastActualLat: 40.0, LastActualLon: -111.0,
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
	// East at 10 m/s for 120s would be 1200m, past the projection cap.
	if d := geo.HaversineMeters(st.LastActualLat, st.LastActualLon, out.Lat, out.Lon); d > MaxDeadReckonMeters+1 {
		t.Errorf("projection = %.0fm from the sighting, want at most the %.0fm cap", d, MaxDeadReckonMeters)
	}
	if out.Confidence == nil || *out.Confidence <= 0 {
		t.Errorf("confidence = %v, want positive", out.Confidence)
	}
}

// Regression: the sweep rewrites Lat/Lon every tick, so a projection measured
// from the previous projection compounds and the train flies off the map.
func TestEvaluateDeadReckoningDoesNotCompound(t *testing.T) {
	heading := 90.0
	st := models.TrainState{
		TrainID: "uta:2", Lat: 40.0, Lon: -111.0,
		PositionType:  models.PositionActual,
		LastActualLat: 40.0, LastActualLon: -111.0,
		Heading:      &heading,
		LastActualAt: time.Now().Add(-2 * time.Minute),
	}
	in := Input{
		State: st, Age: 2 * time.Minute, SpeedEstimate: 20,
		FreshWindow: fresh, MaxWindow: maxW, MinConfidence: models.MinConfidence,
	}
	first := Evaluate(in)

	// The service writes the output back and re-evaluates from the anchor:
	// repeating the sweep must land on the same point, not step past it.
	st.Lat, st.Lon = first.Lat, first.Lon
	st.PositionType = first.PositionType
	in.State = st
	second := Evaluate(in)

	if second.Lat != first.Lat || second.Lon != first.Lon {
		t.Errorf("projection compounded: %v,%v then %v,%v",
			first.Lat, first.Lon, second.Lat, second.Lon)
	}
}

func TestEvaluateHoldsEstimateWithoutAnchor(t *testing.T) {
	// A state written before the anchor existed carries an estimate, not an
	// observation: dead reckoning must hold rather than project from it.
	heading := 90.0
	st := models.TrainState{
		TrainID: "uta:2", Lat: 40.5, Lon: -110.0,
		PositionType: models.PositionEstimated, PositionSource: "estimation-engine",
		Heading:      &heading,
		LastActualAt: time.Now().Add(-2 * time.Minute),
	}
	out := Evaluate(Input{
		State: st, Age: 2 * time.Minute, SpeedEstimate: 20,
		FreshWindow: fresh, MaxWindow: maxW, MinConfidence: models.MinConfidence,
	})
	if out.Lat != st.Lat || out.Lon != st.Lon {
		t.Errorf("position = %v,%v, want held at %v,%v", out.Lat, out.Lon, st.Lat, st.Lon)
	}
	if out.PositionType != models.PositionEstimated {
		t.Errorf("type = %v, want ESTIMATED", out.PositionType)
	}
}

func TestEvaluateDeadReckoningNeedsHeading(t *testing.T) {
	next := "uta:b"
	st := models.TrainState{
		TrainID: "uta:2", Lat: 40.0, Lon: -111.0,
		PositionType:  models.PositionActual,
		LastActualLat: 40.0, LastActualLon: -111.0,
		NextStationID: &next, // no heading: nothing to project along
		LastActualAt:  time.Now().Add(-time.Minute),
	}
	out := Evaluate(Input{
		State: st, Age: time.Minute, SpeedEstimate: 20,
		FreshWindow: fresh, MaxWindow: maxW, MinConfidence: models.MinConfidence,
	})
	if out.Lat != st.LastActualLat || out.Lon != st.LastActualLon {
		t.Errorf("position = %v,%v, want held at the last actual %v,%v",
			out.Lat, out.Lon, st.LastActualLat, st.LastActualLon)
	}
}

// A silent vehicle keeps being swept for minutes: past the cap the estimate
// parks instead of walking off the rails on a straight line.
func TestEvaluateDeadReckoningStopsAtCap(t *testing.T) {
	heading := 90.0
	st := models.TrainState{
		TrainID: "uta:2", Lat: 40.0, Lon: -111.0,
		PositionType:  models.PositionActual,
		LastActualLat: 40.0, LastActualLon: -111.0,
		Heading:      &heading,
		LastActualAt: time.Now().Add(-5 * time.Minute),
	}
	in := Input{
		State: st, Age: 5 * time.Minute, SpeedEstimate: 30, // asks for 9000m of travel
		FreshWindow: fresh, MaxWindow: maxW, MinConfidence: models.MinConfidence,
	}
	first := Evaluate(in)
	if d := geo.HaversineMeters(st.LastActualLat, st.LastActualLon, first.Lat, first.Lon); d > MaxDeadReckonMeters+1 {
		t.Errorf("projection = %.0fm from the sighting, want at most the %.0fm cap", d, MaxDeadReckonMeters)
	}
	in.Age += 30 * time.Second
	second := Evaluate(in)
	if second.Lat != first.Lat || second.Lon != first.Lon {
		t.Errorf("parked estimate kept moving: %v,%v then %v,%v", first.Lat, first.Lon, second.Lat, second.Lon)
	}
}

func TestEvaluateUnknownReturnsLastActual(t *testing.T) {
	st := models.TrainState{
		TrainID: "uta:4", Lat: 36.0, Lon: -111.0, // where a runaway estimate ended up
		PositionType:  models.PositionEstimated,
		LastActualLat: 40.7, LastActualLon: -111.8,
		LastActualAt: time.Now().Add(-maxW - time.Minute),
	}
	out := Evaluate(Input{
		State: st, Age: maxW + time.Minute, SpeedEstimate: 20,
		FreshWindow: fresh, MaxWindow: maxW, MinConfidence: models.MinConfidence,
	})
	if out.PositionType != models.PositionUnknown {
		t.Errorf("type = %v, want UNKNOWN", out.PositionType)
	}
	if out.Lat != 40.7 || out.Lon != -111.8 {
		t.Errorf("position = %v,%v, want last actual 40.7,-111.8", out.Lat, out.Lon)
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
