package tracking

import (
	"context"
	"log/slog"

	"traintracks/internal/models"
	"traintracks/internal/storage/redis"
)

// Engine performs map-matching and state writes on each raw observation
// (SPEC.md §6). It is embeddable inside cmd/ingestor.
type Engine struct {
	rd            *redis.Store
	matchMaxDistM float64
	log           *slog.Logger
}

func New(rd *redis.Store, matchMaxDistMeters float64, log *slog.Logger) *Engine {
	return &Engine{rd: rd, matchMaxDistM: matchMaxDistMeters, log: log}
}

// Observation is a single map-matched raw observation entering the engine.
type Observation struct {
	TrainID string
	models.TrainObservation
	SegmentID *int64
	Chainage  *float64
}

// Process updates live state for a single train. It sets PositionType to
// ACTUAL and writes to Redis.
func (e *Engine) Process(ctx context.Context, obs Observation) error {
	st, err := e.rd.GetTrainState(ctx, obs.TrainID)
	if err != nil {
		return err
	}
	if st == nil {
		st = &models.TrainState{
			TrainID:  obs.TrainID,
			AgencyID: obs.AgencyID,
		}
	}
	st.TrainID = obs.TrainID
	st.AgencyID = obs.AgencyID
	st.RouteID = obs.RouteID
	st.TripID = obs.TripID
	st.Lat = obs.Lat
	st.Lon = obs.Lon
	st.Speed = obs.Speed
	st.Heading = obs.Heading
	st.CurrentTrackSegID = obs.SegmentID
	st.Chainage = obs.Chainage
	st.PositionType = models.PositionActual
	st.PositionSource = obs.Source
	st.LastActualAt = obs.Timestamp
	st.LastUpdateAt = obs.ObservedAt

	e.log.Debug("train actual", "train", st.TrainID, "lat", st.Lat, "lon", st.Lon)
	return e.rd.SetTrainState(ctx, *st)
}
