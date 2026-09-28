package api

import (
	_ "embed"
	"net/http"
)

//go:embed openapi.json
var openAPISpec []byte

//go:embed swagger.html
var docsPage []byte

// GET /api/v1/openapi.json
func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	if _, err := w.Write(openAPISpec); err != nil {
		s.log.Warn("write openapi spec", "err", err)
	}
}

// GET /api/v1/docs
func (s *Server) handleDocs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	if _, err := w.Write(docsPage); err != nil {
		s.log.Warn("write docs page", "err", err)
	}
}
