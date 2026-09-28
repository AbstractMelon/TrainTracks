package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"traintracks/internal/models"
	"traintracks/internal/storage/postgres"
)

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		s.log.Warn("write response", "err", err)
	}
}

func (s *Server) writeErr(w http.ResponseWriter, status int, msg string) {
	s.writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) notFound(w http.ResponseWriter) { s.writeErr(w, http.StatusNotFound, "not found") }

// GET /api/v1/agencies
func (s *Server) handleAgencies(w http.ResponseWriter, r *http.Request) {
	agencies, err := s.pg.ListAgencies(r.Context())
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "load agencies")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"agencies": agencies})
}

// GET /api/v1/agencies/{id}
func (s *Server) handleAgency(w http.ResponseWriter, r *http.Request) {
	a, err := s.pg.GetAgency(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, postgres.ErrNotFound) {
		s.notFound(w)
		return
	}
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "load agency")
		return
	}
	s.writeJSON(w, http.StatusOK, a)
}

// GET /api/v1/routes?agency={id}
func (s *Server) handleRoutes(w http.ResponseWriter, r *http.Request) {
	routes, err := s.pg.ListRoutes(r.Context(), r.URL.Query().Get("agency"))
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "load routes")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"routes": routes})
}

// GET /api/v1/routes/geo?agency={id}&bbox={minLon,minLat,maxLon,maxLat}
func (s *Server) handleRoutesGeo(w http.ResponseWriter, r *http.Request) {
	feats, err := s.pg.RouteShapeFeatures(r.Context(), r.URL.Query().Get("agency"), parseBBox(r))
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "load route geometry")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"type":     "FeatureCollection",
		"features": feats,
	})
}

// GET /api/v1/routes/{id}
func (s *Server) handleRoute(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	route, err := s.pg.GetRoute(ctx, chi.URLParam(r, "id"))
	if errors.Is(err, postgres.ErrNotFound) {
		s.notFound(w)
		return
	}
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "load route")
		return
	}
	stations, err := s.pg.RouteStations(ctx, route.ID)
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "load stations")
		return
	}
	geo, err := s.pg.RouteGeometry(ctx, route.ID)
	if err != nil && !errors.Is(err, postgres.ErrNotFound) {
		s.writeErr(w, http.StatusInternalServerError, "load geometry")
		return
	}
	route.Stations = stations
	if geo != nil {
		route.Geometry = &models.GeoJSON{Type: "LineString", Coordinates: json.RawMessage(geo)}
	}
	s.writeJSON(w, http.StatusOK, route)
}

// GET /api/v1/stations?agency={id}&q={query}
func (s *Server) handleStations(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	stations, err := s.pg.ListStations(r.Context(), q.Get("agency"), q.Get("q"))
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "load stations")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"stations": stations})
}

// GET /api/v1/stations/{id}
func (s *Server) handleStation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st, err := s.pg.GetStation(ctx, chi.URLParam(r, "id"))
	if errors.Is(err, postgres.ErrNotFound) {
		s.notFound(w)
		return
	}
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "load station")
		return
	}
	routes, err := s.pg.StationRoutes(ctx, st.ID)
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "load station routes")
		return
	}
	arrivals, err := s.pg.StationArrivals(ctx, st.ID, now())
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "load arrivals")
		return
	}
	st.Routes = routes
	st.NextArrivals = arrivals
	s.writeJSON(w, http.StatusOK, st)
}

// GET /api/v1/trains?bbox=&agency=&route=&status=
func (s *Server) handleTrains(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	agency := q.Get("agency")
	routes := q.Get("route")
	status := models.TrainStatus(q.Get("status"))
	bbox := parseBBox(r)

	agencies := []string{agency}
	if agency == "" {
		all, err := s.pg.ListAgencies(ctx)
		if err != nil {
			s.writeErr(w, http.StatusInternalServerError, "load agencies")
			return
		}
		agencies = make([]string, 0, len(all))
		for _, a := range all {
			agencies = append(agencies, a.ID)
		}
	}

	states := []models.TrainState{}
	for _, id := range agencies {
		active, err := s.rd.ListActiveByAgency(ctx, id)
		if err != nil {
			continue
		}
		for _, st := range active {
			if routes != "" && st.RouteID != routes {
				continue
			}
			if bbox != nil && !inBBox(st, bbox) {
				continue
			}
			if status != "" && string(st.PositionType) != string(status) {
				continue
			}
			states = append(states, st)
		}
	}

	out := make([]TrainView, 0, len(states))
	for _, st := range states {
		v, err := s.toTrainView(ctx, st)
		if err != nil {
			continue
		}
		out = append(out, v)
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"trains": out})
}

// GET /api/v1/trains/{id}
func (s *Server) handleTrain(w http.ResponseWriter, r *http.Request) {
	st, err := s.rd.GetTrainState(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "load train")
		return
	}
	if st == nil {
		s.notFound(w)
		return
	}
	v, err := s.toTrainView(r.Context(), *st)
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "enrich train")
		return
	}
	s.writeJSON(w, http.StatusOK, v)
}

// GET /api/v1/tracks?bbox=
func (s *Server) handleTracks(w http.ResponseWriter, r *http.Request) {
	bbox := parseBBox(r)
	if bbox == nil {
		s.writeErr(w, http.StatusBadRequest, "bbox parameter required: minLon,minLat,maxLon,maxLat")
		return
	}
	segs, err := s.pg.ListTracks(r.Context(), bbox[0], bbox[1], bbox[2], bbox[3])
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "load tracks")
		return
	}
	feats := make([]map[string]any, 0, len(segs))
	for _, seg := range segs {
		feats = append(feats, map[string]any{
			"type": "Feature",
			"id":   seg.ID,
			"geometry": map[string]any{
				"type":        "LineString",
				"coordinates": seg.Line,
			},
			"properties": map[string]any{
				"osm_way_id":   seg.OSMWayID,
				"railway_type": seg.RailwayType,
				"name":         seg.Name,
			},
		})
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"type":     "FeatureCollection",
		"features": feats,
	})
}

// GET /api/v1/search?q=
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		s.writeErr(w, http.StatusBadRequest, "q parameter required")
		return
	}
	res, err := s.pg.Search(r.Context(), q)
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "search")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"stations": res.Stations,
		"routes":   res.Routes,
	})
}

// GET /api/v1/status
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	health, err := s.rd.ListIngestionHealth(r.Context())
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "load health")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"ingestors": health})
}

func inBBox(st models.TrainState, bbox *[4]float64) bool {
	return st.Lon >= bbox[0] && st.Lon <= bbox[2] && st.Lat >= bbox[1] && st.Lat <= bbox[3]
}
