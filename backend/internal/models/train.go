package models

import (
	"encoding/json"
	"time"
)

// PositionType describes how trustworthy a train's reported position is, per
// SPEC.md §4.1. The transitions between values are implemented in the tracking
// and estimation engines.
type PositionType string

const (
	PositionActual    PositionType = "ACTUAL"
	PositionEstimated PositionType = "ESTIMATED"
	PositionScheduled PositionType = "SCHEDULED"
	PositionStale     PositionType = "STALE"
	PositionUnknown   PositionType = "UNKNOWN"
)

// FreshWindow is the age after which a live observation is no longer considered
// fresh. Tune with real data once ingestion is live.
const FreshWindow = 45 * time.Second

// MaxEstimationWindow is the maximum age after which best-effort estimation is
// abandoned and a train is marked UNKNOWN.
const MaxEstimationWindow = 10 * time.Minute

// MinConfidence is the confidence below which estimation outputs become UNKNOWN.
const MinConfidence = 0.15

// TrainObservation is a single raw position report produced by an adapter.
type TrainObservation struct {
	AgencyID   string
	VehicleID  string
	RouteID    string
	TripID     string
	Lat, Lon   float64
	Speed      *float64  // m/s, nullable. Not every feed provides it
	Heading    *float64  // degrees, nullable
	Timestamp  time.Time // when the source says the position was true
	ObservedAt time.Time // when TrainTracks received it
	Source     string
	Raw        json.RawMessage
}

// TrainState is the live, canonical view of a single train, stored in Redis
// and served by the API.
type TrainState struct {
	TrainID           string       `json:"id"`
	AgencyID          string       `json:"agency"`
	RouteID           string       `json:"route_id"`
	TripID            string       `json:"trip_id"`
	Destination       string       `json:"destination"`
	Lat               float64      `json:"lat"`
	Lon               float64      `json:"lon"`
	Speed             *float64     `json:"speed_mps,omitempty"`
	Heading           *float64     `json:"heading,omitempty"`
	CurrentTrackSegID *int64       `json:"current_track_segment_id,omitempty"`
	Chainage          *float64     `json:"chainage,omitempty"`
	CurrentStationID  *string      `json:"current_station_id,omitempty"`
	NextStationID     *string      `json:"next_station_id,omitempty"`
	DelaySeconds      *int         `json:"delay_seconds,omitempty"`
	PositionType      PositionType `json:"position_type"`
	PositionSource    string       `json:"position_source"`
	Confidence        *float64     `json:"confidence,omitempty"`
	LastActualAt      time.Time    `json:"last_actual_at"`
	LastUpdateAt      time.Time    `json:"last_update_at"`
}
