package api

import (
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/gazes/gazes/internal/auth"
)

// ssrBudgetFactor sizes the budget shared by the web server's own server-side renders.
const ssrBudgetFactor = 10

// rateLimit allows limit requests per span for each client IP on the routes it wraps.
// ffmpeg reading its own stream through /stream/raw over loopback (no X-Forwarded-For: a proxy on
// this host relaying browsers is not exempt) is exempt. A request from the
// trusted proxy without X-Forwarded-For is the web server's server-side render (every browser
// request crosses Caddy, which sets the header): renders share one larger budget instead of being
// unlimited, since a page URL (/?q=..., /genre/x?page=N) would otherwise bypass every limit. A
// render refused here falls back to the browser fetching the data under its own budget.
func (s *Server) rateLimit(name string, limit int, span time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if peerIsLoopback(r) && r.Header.Get("X-Forwarded-For") == "" {
				next.ServeHTTP(w, r)
				return
			}
			// limit is shared by every request through this middleware: never reassign it.
			key, budget := s.clientKey(r), limit
			if s.proxy.Trusts(r) && r.Header.Get("X-Forwarded-For") == "" {
				key, budget = "ssr", limit*ssrBudgetFactor
			}
			if !s.routeLimits.hit(name+"|"+key, budget, span) {
				w.Header().Set("Retry-After", strconv.Itoa(int(span.Seconds())))
				http.Error(w, "rate limit", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// peerIsLoopback reports a request from this host's own socket (ffmpeg), never from a header.
func peerIsLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// clientKey identifies the caller for per-client caps: its IP, or its /64 for IPv6, since one
// subscriber usually holds a whole /64 and could otherwise rotate addresses past any cap.
func (s *Server) clientKey(r *http.Request) string {
	return clientBucket(auth.ClientIP(r, s.proxy.Trusts(r)))
}

func clientBucket(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil || parsed.To4() != nil {
		return ip
	}
	return parsed.Mask(net.CIDRMask(64, 128)).String() + "/64"
}
