package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	mw "traintracks/internal/api/middleware"
	"traintracks/internal/api/sse"
	"traintracks/internal/config"
	"traintracks/internal/storage/postgres"
	"traintracks/internal/storage/redis"
)

// Server wires the REST + SSE surface to Redis and Postgres.
type Server struct {
	cfg config.Config
	log *slog.Logger
	pg  *postgres.Store
	rd  *redis.Store
	hub *sse.Hub

	liveNames *nameCache
	connCap   *connCap
}

const cacheTTL = 60 * time.Second

func NewServer(cfg config.Config, pg *postgres.Store, rd *redis.Store, log *slog.Logger) *Server {
	s := &Server{cfg: cfg, log: log, pg: pg, rd: rd, connCap: newConnCap(cfg.SSEMaxConnPerIP)}
	s.liveNames = newNameCache(pg, cacheTTL, log)

	s.hub = sse.NewHub(rd,
		s.buildLivePayload,
		s.snapshotProvider,
		log, cfg.SSEHeartbeatEvery)
	return s
}

// Router builds the chi router with all middleware.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(mw.Recovery(s.log))
	r.Use(mw.Logger(s.log))
	r.Use(mw.CORS(s.cfg.CORSAllowedOrigin))
	r.Use(mw.RateLimit(s.rd, s.cfg.RateLimitPerMin))

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/agencies", s.handleAgencies)
		r.Get("/agencies/{id}", s.handleAgency)

		r.Get("/routes", s.handleRoutes)
		r.Get("/routes/geo", s.handleRoutesGeo)
		r.Get("/routes/{id}", s.handleRoute)

		r.Get("/stations", s.handleStations)
		r.Get("/stations/{id}", s.handleStation)

		r.Get("/trains", s.handleTrains)
		r.Get("/trains/{id}", s.handleTrain)

		r.Get("/tracks", s.handleTracks)
		r.Get("/search", s.handleSearch)
		r.Get("/status", s.handleStatus)

		r.Get("/live", s.handleLive)
		r.Get("/openapi.json", s.handleOpenAPI)
		r.Get("/docs", s.handleDocs)
	})
	return r
}
