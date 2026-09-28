package middleware

import (
	"context"
	"net/http"
	"strconv"

	"traintracks/internal/storage/redis"
)

type ctxKey string

const (
	ctxRateLimit ctxKey = "ratelimit"
)

// RateLimit is a fixed-window IP rate limiter backed by Redis (SPEC.md §11),
// returning the standard X-RateLimit-* headers and 429 + Retry-After on
// overflow.
func RateLimit(rd *redis.Store, perMin int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if perMin <= 0 {
				next.ServeHTTP(w, r)
				return
			}
			ip := clientIP(r)
			remaining, resetSecs, ok, err := rd.AllowFixedWindow(r.Context(), "ip:"+ip, perMin)
			if err != nil {
				// Fail open on limiter errors rather than 500-ing public reads.
				next.ServeHTTP(w, r)
				return
			}
			ctx := context.WithValue(r.Context(), ctxRateLimit, remaining)
			r = r.WithContext(ctx)

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(perMin))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			w.Header().Set("X-RateLimit-Reset", strconv.Itoa(resetSecs))
			if !ok {
				w.Header().Set("Retry-After", strconv.Itoa(resetSecs))
				http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		return fwd
	}
	return r.RemoteAddr
}

// RateLimitRemaining extracts the remaining budget set by the middleware.
func RateLimitRemaining(ctx context.Context) int {
	if v, ok := ctx.Value(ctxRateLimit).(int); ok {
		return v
	}
	return -1
}
