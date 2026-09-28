package config

import (
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/joho/godotenv"

	"traintracks/internal/models"
)

// Config bundles all runtime configuration, read from environment variables.
type Config struct {
	// Postgres
	PostgresURL string

	// Redis
	RedisURL string

	// Ingestor
	PollJitter      time.Duration
	FetchTimeout    time.Duration
	RetryMax        int
	RetryBaseDelay  time.Duration
	CircuitBreakN   int // consecutive failures before opening the circuit
	CircuitCooldown time.Duration
	TrackingSweep   time.Duration // how often active traains are aged/estimated

	// API
	Addr              string
	RateLimitPerMin   int
	SSEMaxConnPerIP   int
	SSEHeartbeatEvery time.Duration
	CORSAllowedOrigin string

	// Data
	FreshWindow         time.Duration
	MaxEstimationWindow time.Duration
	MinConfidence       float64
	MapMatchMaxDistM    float64

	// Feed URLs (UTA). These are open public feeds (no API key), so the
	// defaults live here in code; the env vars are only overrides.
	UTAGtfsRtVehicleURL    string
	UTAGtfsRtTripUpdateURL string
	UTAGtfsStaticURL       string
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// loadEnv reads the nearest .env file, searching upward from the working
// directory and stopping at the module root. Existing environment variables
// are never overridden.
func loadEnv() error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	for {
		path := filepath.Join(dir, ".env")
		if _, err := os.Stat(path); err == nil {
			return godotenv.Load(path)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil
		}
		dir = parent
	}
}

func getenvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getenvDur(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

func getenvFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}

// Default returns the configuration; env vars override defaults.
func Default() Config {
	if err := loadEnv(); err != nil {
		slog.Warn("load .env", "err", err)
	}
	return Config{
		PostgresURL: getenv("POSTGRES_URL", "postgres://traintracks:traintracks@localhost:5432/traintracks?sslmode=disable"),
		RedisURL:    getenv("REDIS_URL", "redis://localhost:6379/0"),

		PollJitter:      getenvDur("POLL_JITTER", 1*time.Second),
		FetchTimeout:    getenvDur("FETCH_TIMEOUT", 15*time.Second),
		RetryMax:        getenvInt("RETRY_MAX", 3),
		RetryBaseDelay:  getenvDur("RETRY_BASE_DELAY", 500*time.Millisecond),
		CircuitBreakN:   getenvInt("CIRCUIT_BREAK_N", 5),
		CircuitCooldown: getenvDur("CIRCUIT_COOLDOWN", 5*time.Minute),
		TrackingSweep:   getenvDur("TRACKING_SWEEP", 5*time.Second),

		Addr:              getenv("API_ADDR", ":8080"),
		RateLimitPerMin:   getenvInt("RATE_LIMIT_PER_MIN", 120),
		SSEMaxConnPerIP:   getenvInt("SSE_MAX_CONN_PER_IP", 3),
		SSEHeartbeatEvery: getenvDur("SSE_HEARTBEAT_EVERY", 15*time.Second),
		CORSAllowedOrigin: getenv("CORS_ALLOWED_ORIGIN", "*"),

		FreshWindow:         getenvDur("FRESH_WINDOW", models.FreshWindow),
		MaxEstimationWindow: getenvDur("MAX_ESTIMATION_WINDOW", models.MaxEstimationWindow),
		MinConfidence:       getenvFloat("MIN_CONFIDENCE", models.MinConfidence),
		MapMatchMaxDistM:    getenvFloat("MAP_MATCH_MAX_DIST_M", 75),

		UTAGtfsRtVehicleURL:    getenv("UTA_GTFS_RT_VEHICLE_URL", "https://apps.rideuta.com/tms/gtfs/Vehicle"),
		UTAGtfsRtTripUpdateURL: getenv("UTA_GTFS_RT_TRIPUPDATE_URL", "https://apps.rideuta.com/tms/gtfs/TripUpdate"),
		UTAGtfsStaticURL:       getenv("UTA_GTFS_STATIC_URL", "https://apps.rideuta.com/tms/gtfs/Static"),
	}
}
