package api

import (
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/gazes/gazes/internal/auth"
)

// rateLimit allows limit requests per span for each client IP on the routes it wraps.
// Internal callers are exempt: ffmpeg reads its own stream through /stream/raw over loopback, and
// behind the proxy a request without X-Forwarded-For is the web server's own server-side render
// (every browser request crosses Caddy, which sets the header), so it must not share one budget.
func (s *Server) rateLimit(name string, limit int, span time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := auth.ClientIP(r, s.trustProxy)
			internal := s.trustProxy && r.Header.Get("X-Forwarded-For") == ""
			if parsed := net.ParseIP(ip); internal || (parsed != nil && parsed.IsLoopback()) {
				next.ServeHTTP(w, r)
				return
			}
			if !s.routeLimits.hit(name+"|"+ip, limit, span) {
				w.Header().Set("Retry-After", strconv.Itoa(int(span.Seconds())))
				http.Error(w, "rate limit", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
