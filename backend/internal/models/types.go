package models

import (
	"encoding/json"
	"time"
)

// Agency is static metadata about a transit agency, surfaced via /agencies.
type Agency struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Website       string `json:"website,omitempty"`
	GTFSStaticURL string `json:"gtfs_static_url,omitempty"`
	GtfsRtURL     string `json:"gtfs_rt_url,omitempty"`
	Timezone      string `json:"timezone"`
	LicenseNote   string `json:"license_note,omitempty"`
}

// Route is a single transit route (line).
type Route struct {
	ID          string    `json:"id"`
	AgencyID    string    `json:"agency_id"`
	GTFSRouteID string    `json:"gtfs_route_id"`
	ShortName   string    `json:"short_name,omitempty"`
	LongName    string    `json:"long_name,omitempty"`
	Mode        string    `json:"mode"`
	Color       string    `json:"color,omitempty"`
	Stations    []Station `json:"stations,omitempty"`
	Geometry    *GeoJSON  `json:"geometry,omitempty"`
}

// Station is a transit stop/station.
type Station struct {
	ID           string    `json:"id"`
	AgencyID     string    `json:"agency_id"`
	GTFSStopID   string    `json:"gtfs_stop_id"`
	Name         string    `json:"name"`
	Lat          float64   `json:"lat"`
	Lon          float64   `json:"lon"`
	Routes       []Route   `json:"routes,omitempty"`
	NextArrivals []Arrival `json:"next_arrivals,omitempty"`
}

// Point is a bare coordinate pair on a polyline.
type Point struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// Arrival is a scheduled or predicted arrival for a station.
type Arrival struct {
	TrainID     string     `json:"train_id"`
	RouteID     string     `json:"route_id"`
	Headsign    string     `json:"headsign"`
	Direction   *int       `json:"direction_id,omitempty"`
	ScheduledAt time.Time  `json:"scheduled_at"`
	PredictedAt *time.Time `json:"predicted_at,omitempty"`
	DelaySecs   *int       `json:"delay_seconds,omitempty"`
}

// Trip is a scheduled trip.
type Trip struct {
	ID          string `json:"id"`
	RouteID     string `json:"route_id"`
	AgencyID    string `json:"agency_id"`
	ServiceID   string `json:"service_id,omitempty"`
	Headsign    string `json:"headsign,omitempty"`
	DirectionID *int   `json:"direction_id,omitempty"`
}

// Shape is a GTFS shapes.txt polyline (bus or rail corridor geometry).
type Shape struct {
	ID          string      `json:"id"` // '{agency_id}:{gtfs_shape_id}'
	AgencyID    string      `json:"agency_id"`
	GTFSShapeID string      `json:"gtfs_shape_id"`
	Line        [][]float64 `json:"line"` // [lon, lat] pairs
}

// RouteShape links a route to one of the shapes its trips follow.
type RouteShape struct {
	RouteID string
	ShapeID string
}

// RouteShapeFeature is a GeoJSON feature describing one route shape, used by
// GET /api/v1/routes/geo.
type RouteShapeFeature struct {
	Type       string          `json:"type"`
	ID         string          `json:"id"`
	Geometry   GeoJSON         `json:"geometry"`
	Properties RouteShapeProps `json:"properties"`
}

// RouteShapeProps carries route styling info onto a route shape feature.
type RouteShapeProps struct {
	ID        string `json:"id"`
	AgencyID  string `json:"agency_id"`
	ShortName string `json:"short_name,omitempty"`
	LongName  string `json:"long_name,omitempty"`
	Mode      string `json:"mode"`
	Color     string `json:"color,omitempty"`
	ShapeID   string `json:"shape_id"`
}

// TripStop is a scheduled stop time within a trip.
type TripStop struct {
	TripID             string  `json:"trip_id"`
	StationID          string  `json:"station_id"`
	Sequence           int     `json:"sequence"`
	ScheduledArrival   *string `json:"scheduled_arrival,omitempty"`
	ScheduledDeparture *string `json:"scheduled_departure,omitempty"`
}

// TripStopStation is a trip stop joined with the station it refers to.
type TripStopStation struct {
	Sequence int
	Station  Station
}

// TrackSegment is a piece of rail geometry from OSM.
type TrackSegment struct {
	ID          int64       `json:"id"`
	OSMWayID    *int64      `json:"osm_way_id,omitempty"`
	RailwayType string      `json:"railway_type"`
	Name        string      `json:"name,omitempty"`
	Line        [][]float64 `json:"line"`
}

// TrackPoint is a matched position along track geometry.
type TrackPoint struct {
	SegmentID   int64
	Chainage    float64 // meters along the segment
	SegmentFrac float64 // 0..1 fraction along the segment line
	SegmentGeog string  // WKT/geojson line, used by estimation for interpolation
	Distance    float64 // meters from the raw observation to the matched line
}

// GeoJSON is a minimal GeoJSON envelope for serving geometries.
type GeoJSON struct {
	Type        string          `json:"type"`
	Coordinates json.RawMessage `json:"coordinates"`
	Properties  map[string]any  `json:"properties,omitempty"`
}

// TrainStatus drives /trains?status= filtering. Valid values follow PositionType.
type TrainStatus string

// IngestionHealth is per-adapter ingestion health, served via /status.
type IngestionHealth struct {
	Adapter          string    `json:"adapter"`
	LastFetchAt      time.Time `json:"last_fetch_at"`
	LastSuccessAt    time.Time `json:"last_success_at,omitempty"`
	LastError        string    `json:"last_error,omitempty"`
	ErrorCount       int       `json:"error_count"`
	ConsecutiveFails int       `json:"consecutive_failures"`
	CircuitOpen      bool      `json:"circuit_open"`
	Observations     int64     `json:"observations_total"`
	State            string    `json:"state"` // healthy | degraded | open
}

// LiveEvent is what gets pushed over the /live SSE channel.
type LiveEvent struct {
	Type  string     `json:"type"` // train_update | train_removed | heartbeat
	Train TrainState `json:"train,omitempty"`
	ID    string     `json:"id,omitempty"`
}

// StopTimePrediction is one GTFS-RT trip-update stop-time prediction, consumed
// by the estimation engine (SPEC.md §7).
type StopTimePrediction struct {
	TripID             string
	AgencyID           string
	Sequence           int
	StationID          string
	PredictedArrival   *time.Time
	PredictedDeparture *time.Time
	DelaySeconds       *int
	ReportedAt         time.Time
}
