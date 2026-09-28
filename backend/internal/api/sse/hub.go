// Package sse implements the /live Server-Sent Events hub (SPEC.md §11.2):
// snapshot-on-connect, streamed diffs, heartbeats, and server-side filtering.
package sse

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"traintracks/internal/geo"
	"traintracks/internal/models"
	"traintracks/internal/storage/redis"
)

const (
	eventTrainUpdate  = "train_update"
	eventTrainRemoved = "train_removed"
	eventHeartbeat    = "heartbeat"
)

// Filter describes the server-side subscription filter.
type Filter struct {
	Agency string
	Route  string
	BBox   *[4]float64 // minLon, minLat, maxLon, maxLat
}

func (f Filter) matches(st models.TrainState) bool {
	if f.Agency != "" && st.AgencyID != f.Agency {
		return false
	}
	if f.Route != "" && st.RouteID != f.Route {
		return false
	}
	if f.BBox != nil && !geo.BboxContains(st.Lat, st.Lon, f.BBox[0], f.BBox[1], f.BBox[2], f.BBox[3]) {
		return false
	}
	return true
}

// PayloadBuilder renders a TrainState into the bytes sent to clients.
type PayloadBuilder func(ctx context.Context, st models.TrainState) ([]byte, error)

// SnapshotProvider lists all active trains (and expired IDs to drop).
type SnapshotProvider func(ctx context.Context) ([]models.TrainState, []string, error)

// Hub brokers the Redis live channel to HTTP SSE connections.
type Hub struct {
	rd        *redis.Store
	build     PayloadBuilder
	snapshot  SnapshotProvider
	log       *slog.Logger
	heartbeat time.Duration
}

func NewHub(rd *redis.Store, build PayloadBuilder, snapshot SnapshotProvider, log *slog.Logger, heartbeat time.Duration) *Hub {
	return &Hub{rd: rd, build: build, snapshot: snapshot, log: log, heartbeat: heartbeat}
}

// ServeHTTP handles one SSE connection. w must not be written to after hijack;
// we stream via the ResponseWriter directly.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request, f Filter) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	updates, unsubscribe, err := h.rd.SubscribeLive(ctx)
	if err != nil {
		h.log.Error("subscribe live", "err", err)
		http.Error(w, `{"error":"stream unavailable"}`, http.StatusInternalServerError)
		return
	}
	defer unsubscribe()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// Snapshot first, so clients never need a separate REST catch-up call.
	h.writeSnapshot(ctx, w, f)

	heartbeat := time.NewTicker(h.heartbeat)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case id := <-updates:
			h.writeTrainUpdate(ctx, w, f, id)
			flusher.Flush()
		case <-heartbeat.C:
			h.writeEvent(w, eventHeartbeat, nil)
			flusher.Flush()
		}
	}
}

func (h *Hub) writeSnapshot(ctx context.Context, w http.ResponseWriter, f Filter) {
	states, removed, err := h.snapshot(ctx)
	if err != nil {
		h.log.Error("snapshot", "err", err)
		h.writeEvent(w, eventHeartbeat, nil)
		return
	}
	for _, st := range states {
		if f.matches(st) {
			h.writeState(ctx, w, st, eventTrainUpdate)
		}
	}
	for _, id := range removed {
		if f.Agency == "" || f.matches(models.TrainState{TrainID: id, AgencyID: f.Agency}) {
			payload, err := json.Marshal(map[string]string{"id": id})
			if err != nil {
				h.log.Warn("marshal removed train id", "id", id, "err", err)
				continue
			}
			h.writeEvent(w, eventTrainRemoved, payload)
		}
	}
}

func (h *Hub) writeTrainUpdate(ctx context.Context, w http.ResponseWriter, f Filter, id string) {
	st, err := h.rd.GetTrainState(ctx, id)
	if err != nil {
		h.log.Warn("get train state", "train", id, "err", err)
		return
	}
	if st == nil {
		return
	}
	if !f.matches(*st) {
		return
	}
	h.writeState(ctx, w, *st, eventTrainUpdate)
}

func (h *Hub) writeState(ctx context.Context, w http.ResponseWriter, st models.TrainState, event string) {
	payload, err := h.build(ctx, st)
	if err != nil {
		h.log.Warn("build payload", "train", st.TrainID, "err", err)
		return
	}
	h.writeEvent(w, event, payload)
}

// writeEvent frames one SSE event. A write error means the client is gone, so
// it is logged rather than propagated into the streaming loop.
func (h *Hub) writeEvent(w http.ResponseWriter, event string, data []byte) {
	frame := "event: " + event + "\n"
	if data != nil {
		frame += "data: " + string(data) + "\n"
	}
	frame += "\n"
	if _, err := w.Write([]byte(frame)); err != nil {
		h.log.Warn("sse write", "err", err)
	}
}

// ErrUnsubscribed stops the select loop when the redis subscription returns.
var ErrUnsubscribed = errors.New("subscription closed")
