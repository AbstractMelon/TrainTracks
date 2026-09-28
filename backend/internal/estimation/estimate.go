// Package estimation implements the estimation engine (SPEC.md §7): turning
// stale live positions into ESTIMATED / STALE / UNKNOWN positions with an
// explicit confidence value. The decision logic is pure so it can be unit
// tested without infrastructure.
package estimation

import (
	"math"
	"time"

	"traintracks/internal/geo"
	"traintracks/internal/models"
)

// Reason codes for debugging / observability.
const (
	ReasonTripUpdate    = "trip_update"
	ReasonDeadReckoning = "dead_reckoning"
	ReasonStale         = "no_context"
	ReasonUnknown       = "unknown"
)

// MaxHeadingSpeedMps caps dead-reckoning projections (~80 mph, plausible top
// speed for light rail / commuter rail).
const MaxHeadingSpeedMps = 36.0

// BracketingStop is one of the two stops bracketing "now" along a trip.
type BracketingStop struct {
	StationID string
	Lat, Lon  float64
	Chainage  float64 // meters along TripTiming.Path, measured from Path[0]
	Departure *time.Time
	Arrival   *time.Time
}

// TripTiming is the bracketing stop pair from GTFS-RT trip-update predictions.
type TripTiming struct {
	Prev BracketingStop
	Next BracketingStop
	Now  time.Time
	// Path is the route geometry running from Prev to Next, resolved from the
	// route's shape. Nil when no shape covers the pair; interpolation then
	// falls back to the straight chord between the stops.
	Path []models.Point
}

// Weight returns the 0..1 interpolated progress between prev departure and next
// arrival. Falls back to 0/1 on missing edges; clamps to [0,1].
func (t TripTiming) Weight() float64 {
	if t.Prev.Departure == nil || t.Next.Arrival == nil {
		return 0
	}
	total := t.Next.Arrival.Sub(*t.Prev.Departure)
	if total <= 0 {
		return 0
	}
	elapsed := t.Now.Sub(*t.Prev.Departure)
	if elapsed <= 0 {
		return 0
	}
	return geo.Clampf(elapsed.Seconds()/total.Seconds(), 0, 1)
}

// TripUpdateConfidence is f_trip_update(t). Decays slow: the schedule is still
// actively confirmed via trip-update predictions.
func TripUpdateConfidence(age, freshWindow, maxWindow time.Duration) float64 {
	if age <= freshWindow {
		return 1
	}
	if age >= maxWindow {
		return 0
	}
	frac := float64(age-freshWindow) / float64(maxWindow-freshWindow)
	return geo.Clampf(1-frac*0.6, 0, 1)
}

// DeadReckonConfidence is f_dead_reckoning(t). Decays faster: no external
// confirmation of movement beyond the last observation.
func DeadReckonConfidence(age, freshWindow, maxWindow time.Duration) float64 {
	if age <= freshWindow {
		return 1
	}
	if age >= maxWindow {
		return 0
	}
	frac := float64(age-freshWindow) / float64(maxWindow-freshWindow)
	return geo.Clampf(1-frac, 0, 1)
}

// Input drives the decision logic for a single train.
type Input struct {
	State         models.TrainState
	Age           time.Duration
	Trips         *TripTiming // nil when no trip-update predictions exist
	SpeedEstimate float64     // m/s, best available estimate
	FreshWindow   time.Duration
	MaxWindow     time.Duration
	MinConfidence float64
}

// Output is the decided new position for a train.
type Output struct {
	Lat, Lon         float64
	PositionType     models.PositionType
	Confidence       *float64
	CurrentStationID *string
	NextStationID    *string
	DelaySeconds     *int
	Reason           string
}

// Evaluate runs the SPEC.md §7 algorithm for a single train and returns the new
// position. It never fails: missing context degrades the result to STALE or
// UNKNOWN rather than erroring.
func Evaluate(in Input) Output {
	st := in.State
	out := Output{
		Lat:              st.Lat,
		Lon:              st.Lon,
		PositionType:     models.PositionEstimated,
		CurrentStationID: st.CurrentStationID,
		NextStationID:    st.NextStationID,
		DelaySeconds:     st.DelaySeconds,
	}

	// Past the point of all estimation, UNKNOWN regardless of branch.
	if in.Age > in.MaxWindow {
		out.PositionType = models.PositionUnknown
		out.Reason = ReasonUnknown
		out.Confidence = nil
		return out
	}

	switch {
	case in.Trips != nil:
		wt := in.Trips.Weight()
		out.Lat, out.Lon = interpolateBetweenStops(in.Trips, wt)
		out.Confidence = fptr(TripUpdateConfidence(in.Age, in.FreshWindow, in.MaxWindow))
		out.Reason = ReasonTripUpdate
		out.CurrentStationID = strPtr(in.Trips.Prev.StationID)
		out.NextStationID = strPtr(in.Trips.Next.StationID)

	case in.SpeedEstimate > 0:
		lat, lon := deadReckon(st, in.SpeedEstimate, in.Age)
		out.Lat, out.Lon = lat, lon
		out.Confidence = fptr(DeadReckonConfidence(in.Age, in.FreshWindow, in.MaxWindow))
		out.Reason = ReasonDeadReckoning

	default:
		out.PositionType = models.PositionStale
		out.Confidence = nil
		out.Reason = ReasonStale
	}

	if out.Confidence != nil && *out.Confidence < in.MinConfidence {
		out.PositionType = models.PositionUnknown
		out.Confidence = nil
		out.Reason = ReasonUnknown
	}
	return out
}

// interpolateBetweenStops places the train between the bracketing stops at the
// given timing weight, walking the route geometry when we have it so the
// estimate follows the line instead of cutting the corner.
func interpolateBetweenStops(t *TripTiming, weight float64) (float64, float64) {
	if lat, lon, ok := pointAlongPath(t.Path, weight); ok {
		return lat, lon
	}
	return geo.InterpolatePoint(t.Prev.Lat, t.Prev.Lon, t.Next.Lat, t.Next.Lon, weight)
}

// pointAlongPath returns the point `weight` of the way along a polyline, where
// distance is measured along the line (chainage). The third result is false
// when the path can't be walked, so callers can fall back to a chord.
func pointAlongPath(path []models.Point, weight float64) (float64, float64, bool) {
	total := pathLength(path)
	if total <= 0 {
		return 0, 0, false
	}
	target := geo.Clampf(weight, 0, 1) * total
	traveled := 0.0
	for i := 1; i < len(path); i++ {
		seg := geo.HaversineMeters(path[i-1].Lat, path[i-1].Lon, path[i].Lat, path[i].Lon)
		if traveled+seg >= target {
			frac := 0.0
			if seg > 0 {
				frac = (target - traveled) / seg
			}
			lat, lon := geo.InterpolatePoint(path[i-1].Lat, path[i-1].Lon, path[i].Lat, path[i].Lon, frac)
			return lat, lon, true
		}
		traveled += seg
	}
	last := path[len(path)-1]
	return last.Lat, last.Lon, true
}

// pathLength is the total length of a polyline in meters, 0 when it has no
// usable segments.
func pathLength(path []models.Point) float64 {
	if len(path) < 2 {
		return 0
	}
	total := 0.0
	for i := 1; i < len(path); i++ {
		total += geo.HaversineMeters(path[i-1].Lat, path[i-1].Lon, path[i].Lat, path[i].Lon)
	}
	return total
}

// deadReckon projects the last known position forward by speed over age,
// clamped to a plausible max speed. Uses heading when known.
func deadReckon(st models.TrainState, speed float64, age time.Duration) (float64, float64) {
	elapsed := age.Seconds()
	if speed <= 0 || elapsed <= 0 {
		return st.Lat, st.Lon
	}
	dist := geo.DistanceAtSpeedMeters(speed, elapsed, MaxHeadingSpeedMps*elapsed)
	if dist <= 0 {
		return st.Lat, st.Lon
	}
	if (st.Heading == nil || *st.Heading < 0) && st.NextStationID == nil {
		return st.Lat, st.Lon
	}

	heading := 0.0
	if st.Heading != nil && *st.Heading >= 0 {
		heading = *st.Heading
	}
	// Destination point given bearing + distance (spherical approximation,
	// fine for sub-kilometer projections).
	d := dist / earthRadiusM
	φ1 := st.Lat * degToRad
	λ1 := st.Lon * degToRad
	θ := heading * degToRad
	φ2 := math.Asin(math.Sin(φ1)*math.Cos(d) + math.Cos(φ1)*math.Sin(d)*math.Cos(θ))
	λ2 := λ1 + math.Atan2(math.Sin(θ)*math.Sin(d)*math.Cos(φ1), math.Cos(d)-math.Sin(φ1)*math.Sin(φ2))
	return φ2 * radToDeg, normalizeLon(λ2 * radToDeg)
}

func fptr(f float64) *float64 { return &f }

func strPtr(s string) *string { return &s }

const (
	earthRadiusM = 6371000.0
	degToRad     = math.Pi / 180
	radToDeg     = 180 / math.Pi
)

func normalizeLon(lon float64) float64 {
	for lon > 180 {
		lon -= 360
	}
	for lon < -180 {
		lon += 360
	}
	return lon
}
