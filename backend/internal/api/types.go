// Package api exposes the public REST + SSE surface (SPEC.md §11).
package api

import (
	"time"

	"traintracks/internal/models"
)

// PositionView is the nested `position` object of a train response.
type PositionView struct {
	Lat          float64   `json:"lat"`
	Lon          float64   `json:"lon"`
	Heading      *float64  `json:"heading,omitempty"`
	SpeedMPH     *float64  `json:"speed_mph,omitempty"`
	Type         string    `json:"type"`
	Source       string    `json:"source"`
	Confidence   *float64  `json:"confidence"`
	LastActualAt time.Time `json:"last_actual_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// StationRef is a short station reference used inside train responses.
type StationRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// RouteRef is a short route reference used inside train responses.
type RouteRef struct {
	ID        string `json:"id"`
	ShortName string `json:"short_name"`
	LongName  string `json:"long_name,omitempty"`
	Mode      string `json:"mode,omitempty"`
	Color     string `json:"color,omitempty"`
}

// TrainView is the public representation of a train. It mirrors SPEC.md §11.1.
type TrainView struct {
	ID             string       `json:"id"`
	Agency         string       `json:"agency"`
	Route          *RouteRef    `json:"route,omitempty"`
	Destination    string       `json:"destination,omitempty"`
	Position       PositionView `json:"position"`
	CurrentStation *StationRef  `json:"current_station"`
	NextStation    *StationRef  `json:"next_station,omitempty"`
	DelaySeconds   *int         `json:"delay_seconds,omitempty"`
}

// toTrainView converts a TrainState into its API representation.
func toTrainView(st models.TrainState, route *RouteRef, stationNames map[string]string) TrainView {
	v := TrainView{
		ID:          st.TrainID,
		Agency:      st.AgencyID,
		Route:       route,
		Destination: st.Destination,
		Position: PositionView{
			Lat:          st.Lat,
			Lon:          st.Lon,
			Heading:      st.Heading,
			SpeedMPH:     mph(st.Speed),
			Type:         string(st.PositionType),
			Source:       st.PositionSource,
			Confidence:   st.Confidence,
			LastActualAt: st.LastActualAt,
			UpdatedAt:    st.LastUpdateAt,
		},
		DelaySeconds: st.DelaySeconds,
	}
	if st.CurrentStationID != nil && *st.CurrentStationID != "" {
		v.CurrentStation = &StationRef{ID: *st.CurrentStationID, Name: stationNames[*st.CurrentStationID]}
	}
	if st.NextStationID != nil && *st.NextStationID != "" {
		v.NextStation = &StationRef{ID: *st.NextStationID, Name: stationNames[*st.NextStationID]}
	}
	return v
}

func mph(speed *float64) *float64 {
	if speed == nil {
		return nil
	}
	v := *speed * 2.23694
	return &v
}
