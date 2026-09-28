// Package geo provides map-matching helpers and geodesic math (SPEC.md §internal/geo).
package geo

import "math"

const earthRadiusM = 6371000.0

func deg2rad(d float64) float64 { return d * math.Pi / 180 }

// HaversineMeters returns the great-circle distance in meters between two
// lat/lon points.
func HaversineMeters(lat1, lon1, lat2, lon2 float64) float64 {
	dLat := deg2rad(lat2 - lat1)
	dLon := deg2rad(lon2 - lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(deg2rad(lat1))*math.Cos(deg2rad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusM * c
}

// BearingDegrees returns the initial bearing in degrees [0,360) from point 1 to point 2.
func BearingDegrees(lat1, lon1, lat2, lon2 float64) float64 {
	dLon := deg2rad(lon2 - lon1)
	y := math.Sin(dLon) * math.Cos(deg2rad(lat2))
	x := math.Cos(deg2rad(lat1))*math.Sin(deg2rad(lat2)) -
		math.Sin(deg2rad(lat1))*math.Cos(deg2rad(lat2))*math.Cos(dLon)
	b := math.Atan2(y, x)
	return math.Mod((b*180/math.Pi)+360, 360)
}

// PolygonContains check if a lat/lon point is inside a bbox (minLon,minLat,maxLon,maxLat).
func BboxContains(lat, lon, minLon, minLat, maxLon, maxLat float64) bool {
	return lon >= minLon && lon <= maxLon && lat >= minLat && lat <= maxLat
}

// Clampf clamps v into [lo, hi].
func Clampf(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// InterpolatePoint linearly interpolates between two lat/lon points by fraction
// t in [0,1].
func InterpolatePoint(lat1, lon1, lat2, lon2, t float64) (float64, float64) {
	t = Clampf(t, 0, 1)
	return lat1 + (lat2-lat1)*t, lon1 + (lon2-lon1)*t
}

// LocationDistanceMeters returns haversine distance at ground speed velocity.
func DistanceAtSpeedMeters(speedMps float64, elapsedS float64, maxPlausibleM float64) float64 {
	d := speedMps * elapsedS
	if d < 0 {
		return 0
	}
	if d > maxPlausibleM {
		return maxPlausibleM
	}
	return d
}
