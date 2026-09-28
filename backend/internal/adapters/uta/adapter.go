package uta

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	gtfs "github.com/MobilityData/gtfs-realtime-bindings/golang/gtfs"
	"google.golang.org/protobuf/proto"

	"traintracks/internal/config"
	"traintracks/internal/models"
)

const AgencyID = "uta"

type Config struct {
	VehiclePositionURL string
	TripUpdateURL      string
	PollInterval       time.Duration
	FetchTimeout       time.Duration
}

func NewConfig(cfg config.Config) Config {
	return Config{
		VehiclePositionURL: cfg.UTAGtfsRtVehicleURL,
		TripUpdateURL:      cfg.UTAGtfsRtTripUpdateURL,
		PollInterval:       20 * time.Second,
		FetchTimeout:       10 * time.Second,
	}
}

type Adapter struct {
	cfg Config
}

func New(cfg Config) *Adapter {
	return &Adapter{cfg: cfg}
}

func (a *Adapter) Name() string                          { return AgencyID }
func (a *Adapter) PollInterval() time.Duration           { return a.cfg.PollInterval }
func (a *Adapter) TripUpdatePollInterval() time.Duration { return a.cfg.PollInterval }

func (a *Adapter) Fetch(ctx context.Context) ([]models.TrainObservation, error) {
	if a.cfg.VehiclePositionURL == "" {
		return nil, nil
	}
	feed, err := fetchFeed(ctx, a.cfg.VehiclePositionURL, a.cfg.FetchTimeout)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	obs := make([]models.TrainObservation, 0, len(feed.GetEntity()))
	for _, ent := range feed.GetEntity() {
		if ent.GetVehicle() == nil {
			continue
		}
		v := ent.GetVehicle()
		pos := v.GetPosition()
		obs = append(obs, models.TrainObservation{
			AgencyID:   AgencyID,
			VehicleID:  vehicleID(v),
			RouteID:    agencyScopedID(v.GetTrip().GetRouteId()),
			TripID:     agencyScopedID(v.GetTrip().GetTripId()),
			Lat:        float64(pos.GetLatitude()),
			Lon:        float64(pos.GetLongitude()),
			Speed:      floatPtr(float64(pos.GetSpeed()) * 0.44704),
			Heading:    headingPtr(float64(pos.GetBearing())),
			Timestamp:  time.Unix(int64(v.GetTimestamp()), 0),
			ObservedAt: now,
			Source:     "uta-gtfs-rt",
		})
	}
	return obs, nil
}

func (a *Adapter) FetchTripUpdates(ctx context.Context) ([]models.StopTimePrediction, error) {
	if a.cfg.TripUpdateURL == "" {
		return nil, nil
	}
	feed, err := fetchFeed(ctx, a.cfg.TripUpdateURL, a.cfg.FetchTimeout)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	out := []models.StopTimePrediction{}
	for _, ent := range feed.GetEntity() {
		if ent.GetTripUpdate() == nil {
			continue
		}
		tu := ent.GetTripUpdate()
		tripID := AgencyID + ":" + tu.GetTrip().GetTripId()
		for _, stu := range tu.GetStopTimeUpdate() {
			dep := timeFromSeconds(stu.GetDeparture().GetTime())
			arr := timeFromSeconds(stu.GetArrival().GetTime())
			var delay *int
			if d := stu.GetArrival().GetDelay(); d != 0 {
				sec := int(d)
				delay = &sec
			}
			out = append(out, models.StopTimePrediction{
				TripID:             tripID,
				AgencyID:           AgencyID,
				Sequence:           int(stu.GetStopSequence()),
				StationID:          AgencyID + ":" + stu.GetStopId(),
				PredictedArrival:   arr,
				PredictedDeparture: dep,
				DelaySeconds:       delay,
				ReportedAt:         now,
			})
		}
	}
	return out, nil
}

var client = &http.Client{Timeout: 20 * time.Second}

func fetchFeed(ctx context.Context, url string, timeout time.Duration) (*gtfs.FeedMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "TrainTracks/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("feed %s returned status %d", url, resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil // empty 200, treat as no entities
	}
	var feed gtfs.FeedMessage
	if err := proto.Unmarshal(data, &feed); err != nil {
		return nil, err
	}
	return &feed, nil
}

func vehicleID(v *gtfs.VehiclePosition) string {
	return v.GetVehicle().GetId()
}

// agencyScopedID namespaces a feed id as agency:id. The vehicle feed leaves
// trip.route_id empty, so an omitted id stays empty instead of becoming "uta:",
// which lets the ingestor resolve the route from GTFS static trips.
func agencyScopedID(id string) string {
	if id == "" {
		return ""
	}
	return AgencyID + ":" + id
}

func floatPtr(v float64) *float64 { return &v }

func headingPtr(v float64) *float64 {
	if v == 0 {
		return nil
	}
	return &v
}

func timeFromSeconds(sec int64) *time.Time {
	if sec == 0 {
		return nil
	}
	t := time.Unix(sec, 0)
	return &t
}
