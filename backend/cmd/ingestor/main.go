// Command ingestor runs the data pipeline: adapters → observation log → tracking
// engine (Redis/TrainState) and the estimation-engine freshness sweep.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"traintracks/internal/adapters"
	"traintracks/internal/adapters/uta"
	"traintracks/internal/config"
	"traintracks/internal/estimation"
	"traintracks/internal/ingestor"
	"traintracks/internal/storage/postgres"
	"traintracks/internal/storage/redis"
	"traintracks/internal/tracking"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Default()

	if cfg.UTAGtfsRtVehicleURL != "" {
		adapters.Register(uta.New(uta.NewConfig(cfg)))
		logger.Info("registered adapter", "name", uta.AgencyID, "vehicle_url", cfg.UTAGtfsRtVehicleURL)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pg, err := postgres.New(ctx, cfg.PostgresURL)
	if err != nil {
		logger.Error("connect postgres", "err", err)
		os.Exit(1)
	}
	defer pg.Close()

	rd, err := redis.New(cfg.RedisURL)
	if err != nil {
		logger.Error("connect redis", "err", err)
		os.Exit(1)
	}
	defer rd.Close()

	// Warm-up pings with a short retry to tolerate compose startup ordering.
	if err := waitReady(ctx, cfg, pg, rd); err != nil {
		logger.Error("dependencies not ready", "err", err)
		os.Exit(1)
	}

	tracker := tracking.New(rd, cfg.MapMatchMaxDistM, logger)
	runner := ingestor.New(cfg, pg, rd, tracker, logger)
	runner.Start(ctx)

	est := estimation.NewService(pg, rd, cfg, logger)
	go estimationTicker(ctx, est, cfg.TrackingSweep)

	logger.Info("ingestor started")
	<-ctx.Done()
	logger.Info("shutting down")
	runner.Stop()
}

func estimationTicker(ctx context.Context, est *estimation.Service, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := est.Sweep(ctx); err != nil {
				slog.Info("estimation sweep", "err", err)
			}
		}
	}
}

func waitReady(ctx context.Context, cfg config.Config, pg *postgres.Store, rd *redis.Store) error {
	var lastErr error
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if e1, e2 := pg.Ping(ctx), rd.Ping(ctx); e1 == nil && e2 == nil {
			return nil
		} else {
			lastErr = errors.Join(e1, e2)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return lastErr
}
