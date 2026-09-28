package api

import (
	"net/http"
	"sync"
	"time"

	"traintracks/internal/api/sse"
)

var now = func() time.Time { return time.Now() }

// Per-IP concurrent SSE connection cap is intentionally in-memory: cmd/api is a
// single instance in v1 (SPEC.md §11).
type connCap struct {
	mu   sync.Mutex
	open map[string]int
	max  int
}

func newConnCap(max int) *connCap {
	if max <= 0 {
		max = 3
	}
	return &connCap{open: map[string]int{}, max: max}
}

func (c *connCap) acquire(ip string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.open[ip] >= c.max {
		return false
	}
	c.open[ip]++
	return true
}

func (c *connCap) release(ip string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.open[ip] > 0 {
		c.open[ip]--
	}
}

// GET /api/v1/live?bbox=&agency=&route=
func (s *Server) handleLive(w http.ResponseWriter, r *http.Request) {
	ip := clientIPOf(r)
	if !s.connCap.acquire(ip) {
		w.Header().Set("Retry-After", "30")
		s.writeErr(w, http.StatusTooManyRequests, "too many concurrent SSE connections")
		return
	}
	defer s.connCap.release(ip)

	q := r.URL.Query()
	f := sse.Filter{
		Agency: q.Get("agency"),
		Route:  q.Get("route"),
		BBox:   parseBBox(r),
	}
	s.hub.ServeHTTP(w, r, f)
}

func clientIPOf(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		return fwd
	}
	return r.RemoteAddr
}
