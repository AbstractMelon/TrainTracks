// Package ingestor runs the adapter poll loop: timeout, retry w/ backoff,
// circuit breaker, per-adapter isolation, and handoff to the tracking engine
// (SPEC.md §5).
package ingestor

import (
	"context"
	"log/slog"
	"math/rand"
	"sync"
	"time"

	"traintracks/internal/adapters"
	"traintracks/internal/config"
	"traintracks/internal/models"
	"traintracks/internal/storage/postgres"
	"traintracks/internal/storage/redis"
	"traintracks/internal/tracking"
)

// Runner drives all registered adapters.
type Runner struct {
	cfg     config.Config
	log     *slog.Logger
	pg      *postgres.Store
	rd      *redis.Store
	tracker *tracking.Engine

	mu     sync.Mutex
	health map[string]*models.IngestionHealth
	cancel context.CancelFunc
	wg     sync.WaitGroup

	routes tripRoutes
}

func New(cfg config.Config, pg *postgres.Store, rd *redis.Store, tracker *tracking.Engine, log *slog.Logger) *Runner {
	return &Runner{
		cfg:     cfg,
		log:     log,
		pg:      pg,
		rd:      rd,
		tracker: tracker,
		health:  map[string]*models.IngestionHealth{},
	}
}

// Start begins polling every registered adapter. Cancels via Stop.
func (r *Runner) Start(ctx context.Context) {
	ctx, r.cancel = context.WithCancel(ctx)
	for _, a := range adapters.All() {
		r.wg.Add(2)
		go r.pollLoop(ctx, a, time.Duration(0)) // vehicle positions
		go r.pollLoopTripUpdates(ctx, a)        // trip updates
	}
}

func (r *Runner) Stop() {
	if r.cancel != nil {
		r.cancel()
	}
	r.wg.Wait()
}

// pollLoop ticks vehicle-position fetches at the adapter's PollInterval.
func (r *Runner) pollLoop(ctx context.Context, a adapters.Adapter, _ time.Duration) {
	defer r.wg.Done()
	defer func() {
		if p := recover(); p != nil {
			r.log.Error("adapter goroutine panicked", "adapter", a.Name(), "panic", p)
		}
	}()

	interval := a.PollInterval()
	if interval <= 0 {
		interval = 30 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()

	// Initial jitter avoids all adapters stampeding on startup.
	time.Sleep(time.Duration(rand.Int63n(int64(r.cfg.PollJitter))))
	r.fetchWithProtection(ctx, a)

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.fetchWithProtection(ctx, a)
		}
	}
}

// fetchWithProtection wraps a single Fetch in retry/backoff + circuit breaker.
func (r *Runner) fetchWithProtection(ctx context.Context, a adapters.Adapter) {
	h := r.healthOf(a.Name())

	if h.CircuitOpen {
		if time.Since(h.LastFetchAt) < r.cfg.CircuitCooldown {
			r.log.Warn("circuit open, skipping adapter", "adapter", a.Name())
			return
		}
		h.CircuitOpen = false
		h.ConsecutiveFails = 0
	}

	obs, err := r.fetchWithRetry(ctx, a)
	if err != nil {
		h.LastError = err.Error()
		h.ErrorCount++
		h.ConsecutiveFails++
		if h.ConsecutiveFails >= r.cfg.CircuitBreakN {
			h.CircuitOpen = true
			h.State = "open"
		} else {
			h.State = "degraded"
		}
		h.LastFetchAt = time.Now()
		r.persistHealth(a.Name(), h)
		r.log.Error("adapter fetch failed", "adapter", a.Name(), "err", err, "consecutive", h.ConsecutiveFails)
		return
	}

	h.CircuitOpen = false
	h.ConsecutiveFails = 0
	h.LastError = ""
	h.LastSuccessAt = time.Now()
	h.State = "healthy"
	h.Observations += int64(len(obs))
	h.LastFetchAt = time.Now()
	r.persistHealth(a.Name(), h)

	for _, o := range obs {
		if err := r.handleObservation(ctx, a.Name(), o); err != nil {
			r.log.Error("handle observation", "adapter", a.Name(), "err", err, "vehicle", o.VehicleID)
		}
	}
}

func (r *Runner) fetchWithRetry(ctx context.Context, a adapters.Adapter) ([]models.TrainObservation, error) {
	var lastErr error
	for attempt := 0; attempt <= r.cfg.RetryMax; attempt++ {
		obs, err := a.Fetch(ctx)
		if err == nil {
			return obs, nil
		}
		lastErr = err
		if attempt < r.cfg.RetryMax {
			backoff := r.cfg.RetryBaseDelay * time.Duration(1<<uint(attempt))
			r.log.Debug("retrying fetch", "adapter", a.Name(), "attempt", attempt+1, "backoff", backoff)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	return nil, lastErr
}

// pollLoopTripUpdates ingests GTFS-RT trip-update predictions into Postgres so
// the estimation engine can interpolate along the schedule (SPEC.md §7).
func (r *Runner) pollLoopTripUpdates(ctx context.Context, a adapters.Adapter) {
	defer r.wg.Done()
	defer func() {
		if p := recover(); p != nil {
			r.log.Error("trip-update goroutine panicked", "adapter", a.Name(), "panic", p)
		}
	}()

	interval := a.TripUpdatePollInterval()
	if interval <= 0 {
		interval = 30 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()

	time.Sleep(time.Duration(rand.Int63n(int64(r.cfg.PollJitter))))
	r.fetchTripUpdates(ctx, a)

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.fetchTripUpdates(ctx, a)
		}
	}
}

func (r *Runner) fetchTripUpdates(ctx context.Context, a adapters.Adapter) {
	preds, err := a.FetchTripUpdates(ctx)
	if err != nil {
		if a.Name() == "uta" {
			r.log.Warn("trip-update fetch failed", "adapter", a.Name(), "err", err)
		}
		return
	}
	for _, p := range preds {
		if err := r.pg.UpsertTripPrediction(ctx, p); err != nil {
			r.log.Error("store trip prediction", "adapter", a.Name(), "err", err)
			return
		}
	}
	if len(preds) > 0 {
		r.log.Debug("trip-update predictions stored", "adapter", a.Name(), "n", len(preds))
	}
}

// tripRouteTTL is how long the trip → route map stays fresh. Refreshing costs
// a single query over every imported trip, so the cache is refreshed on a timer
// instead of per observation.
const tripRouteTTL = 5 * time.Minute

// tripRoutes resolves an observation's route from GTFS static trips. UTA's
// vehicle feed omits trip.route_id, so without this every train would carry an
// empty route: no route name, no vehicle mode, no working route filters.
type tripRoutes struct {
	mu     sync.Mutex
	byTrip map[string]string
	loaded time.Time
}

// routeFor returns the route id for a trip, reloading the map when stale. A
// failed reload still returns the stale value so route info is only lost when
// it never loaded at all.
func (c *tripRoutes) routeFor(ctx context.Context, pg *postgres.Store, tripID string) (string, error) {
	if tripID == "" {
		return "", nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.loaded) >= tripRouteTTL {
		byTrip, err := pg.TripRouteMap(ctx)
		if err != nil {
			if c.byTrip == nil {
				return "", err
			}
			return c.byTrip[tripID], err
		}
		c.byTrip, c.loaded = byTrip, time.Now()
	}
	return c.byTrip[tripID], nil
}

// handleObservation normalizes, logs, map-matches and hands off to the engine.
func (r *Runner) handleObservation(ctx context.Context, source string, o models.TrainObservation) error {
	o.Source = source
	trainID := o.AgencyID + ":" + o.VehicleID

	if o.RouteID == "" {
		routeID, err := r.routes.routeFor(ctx, r.pg, o.TripID)
		if err != nil {
			r.log.Warn("resolve route for trip", "trip", o.TripID, "err", err)
		}
		o.RouteID = routeID
	}

	if err := r.pg.AppendObservation(ctx, o); err != nil {
		return err
	}

	var segID *int64
	var chain *float64
	if matched, err := r.pg.MapMatch(ctx, o.RouteID, o.Lat, o.Lon, r.cfg.MapMatchMaxDistM); err != nil {
		return err
	} else if matched != nil {
		segID = &matched.SegmentID
		chain = &matched.Chainage
	}

	return r.tracker.Process(ctx, tracking.Observation{
		TrainID:          trainID,
		TrainObservation: o,
		SegmentID:        segID,
		Chainage:         chain,
	})
}

func (r *Runner) healthOf(name string) *models.IngestionHealth {
	r.mu.Lock()
	defer r.mu.Unlock()
	h, ok := r.health[name]
	if !ok {
		h = &models.IngestionHealth{Adapter: name, State: "starting"}
		r.health[name] = h
	}
	return h
}

func (r *Runner) persistHealth(name string, h *models.IngestionHealth) {
	if err := r.rd.SetIngestionHealth(context.Background(), *h); err != nil {
		r.log.Warn("persist health", "adapter", name, "err", err)
	}
}

// Health returns a snapshot of per-adapter health for /status.
func (r *Runner) Health() []models.IngestionHealth {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]models.IngestionHealth, 0, len(r.health))
	for _, h := range r.health {
		cp := *h
		out = append(out, cp)
	}
	return out
}
