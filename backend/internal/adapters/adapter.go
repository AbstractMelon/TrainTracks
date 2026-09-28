package adapters

import (
	"context"
	"time"

	"traintracks/internal/models"
)

// Adapter is the interface every agency data-source implements (SPEC.md §4.3).
type Adapter interface {
	Name() string
	PollInterval() time.Duration
	Fetch(ctx context.Context) ([]models.TrainObservation, error)
	TripUpdatePollInterval() time.Duration
	FetchTripUpdates(ctx context.Context) ([]models.StopTimePrediction, error)
}

var registry = map[string]Adapter{}

// Register adds an adapter to the global registry. Call from init() or main.
func Register(a Adapter) {
	registry[a.Name()] = a
}

// Get returns a registered adapter by name, or nil.
func Get(name string) Adapter {
	return registry[name]
}

// All returns every registered adapter.
func All() []Adapter {
	out := make([]Adapter, 0, len(registry))
	for _, a := range registry {
		out = append(out, a)
	}
	return out
}
