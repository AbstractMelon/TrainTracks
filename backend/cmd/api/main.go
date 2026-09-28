// Command api serves the public REST + SSE API (SPEC.md §11). Stateless except
// for an in-memory SSE connection counter; reads live state from Redis and
// static data from Postgres.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"traintracks/internal/api"
	"traintracks/internal/config"
	"traintracks/internal/storage/postgres"
	"traintracks/internal/storage/redis"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Default()

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

	if err := waitReady(ctx, pg, rd); err != nil {
		logger.Error("dependencies not ready", "err", err)
		os.Exit(1)
	}

	srv := api.NewServer(cfg, pg, rd, logger)
	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("api listening", "addr", cfg.Addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("api server", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("api shutdown", "err", err)
	}
}

func waitReady(ctx context.Context, pg *postgres.Store, rd *redis.Store) error {
	var lastErr error
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if e1, e2 := pg.Ping(ctx), rd.Ping(ctx); e1 == nil && e2 == nil {
			return nil
		} else {
			lastErr = errors.Join(e1, e2)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return lastErr
}
