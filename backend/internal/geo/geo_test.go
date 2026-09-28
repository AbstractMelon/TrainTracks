package geo

import (
	"math"
	"testing"
)

func TestHaversineMeters(t *testing.T) {
	// Salt Lake City → ~111.32 km per degree of latitude.
	d := HaversineMeters(40.76, -111.89, 41.76, -111.89)
	if math.Abs(d-111320) > 3000 {
		t.Errorf("distance = %v, want ~111320", d)
	}
}

func TestBearingDegrees(t *testing.T) {
	// Due east from one point returns 90.
	b := BearingDegrees(40.0, -111.0, 40.0, -110.99)
	if b < 89 || b > 91 {
		t.Errorf("bearing = %v, want ~90", b)
	}
	// Due north returns 0.
	b = BearingDegrees(40.0, -111.0, 40.01, -111.0)
	if b > 10 && b < 350 {
		t.Errorf("bearing = %v, want ~0", b)
	}
}

func TestBboxContains(t *testing.T) {
	if !BboxContains(40.75, -111.9, -112, 40, -111, 41) {
		t.Error("expected inside")
	}
	if BboxContains(41.5, -111.9, -112, 40, -111, 41) {
		t.Error("expected outside")
	}
}

func TestInterpolatePoint(t *testing.T) {
	lat, lon := InterpolatePoint(0, 0, 10, 10, 0.5)
	if lat != 5 || lon != 5 {
		t.Errorf("got (%v,%v), want (5,5)", lat, lon)
	}
}

func TestClampf(t *testing.T) {
	if v := Clampf(5, 0, 1); v != 1 {
		t.Errorf("clamp = %v, want 1", v)
	}
	if v := Clampf(-3, 0, 1); v != 0 {
		t.Errorf("clamp = %v, want 0", v)
	}
}

func TestDistanceAtSpeedMeters(t *testing.T) {
	if d := DistanceAtSpeedMeters(10, 60, 1000); d != 600 {
		t.Errorf("distance = %v, want 600", d)
	}
	if d := DistanceAtSpeedMeters(10, 60, 300); d != 300 {
		t.Errorf("capped distance = %v, want 300", d)
	}
}
