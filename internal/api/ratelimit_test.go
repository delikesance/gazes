package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gazes/gazes/internal/auth"
)

func limited(s *Server, remote, xff string) int {
	h := s.rateLimit("t", 2, time.Minute)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = remote
	if xff != "" {
		r.Header.Set("X-Forwarded-For", xff)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Code
}

func TestRateLimitKeysOnClientIP(t *testing.T) {
	s := &Server{proxy: auth.NewProxyTrust(true, nil)}
	for range 2 {
		if c := limited(s, "172.18.0.5:1", "203.0.113.7"); c != 200 {
			t.Fatalf("within limit: %d", c)
		}
	}
	if c := limited(s, "172.18.0.5:1", "203.0.113.7"); c != http.StatusTooManyRequests {
		t.Fatalf("over limit: %d", c)
	}
	// Another client behind the same proxy has its own budget.
	if c := limited(s, "172.18.0.5:1", "203.0.113.8"); c != 200 {
		t.Fatalf("other client throttled: %d", c)
	}
}

func TestRateLimitIgnoresForwardedHeaderWithoutProxy(t *testing.T) {
	s := &Server{}
	for range 2 {
		limited(s, "198.51.100.1:1", "10.0.0.1")
	}
	if c := limited(s, "198.51.100.1:1", "10.0.0.2"); c != http.StatusTooManyRequests {
		t.Fatalf("forged X-Forwarded-For escaped the limit: %d", c)
	}
}

func TestRateLimitSkipsLoopback(t *testing.T) {
	s := &Server{}
	for range 10 {
		if c := limited(s, "127.0.0.1:1", ""); c != 200 {
			t.Fatalf("loopback (ffmpeg reading its own stream) throttled: %d", c)
		}
	}
}

func TestRateLimiterEvictsInsteadOfLockingOut(t *testing.T) {
	var l rateLimiter
	for i := range rateLimiterMaxKeys + 10 {
		l.hit(string(rune('a'))+time.Duration(i).String(), 5, time.Minute)
	}
	if !l.hit("fresh-client", 5, time.Minute) {
		t.Fatal("a full table must evict old keys, not refuse new clients")
	}
}

func TestRateLimitSkipsWebServerRenderBehindProxy(t *testing.T) {
	s := &Server{proxy: auth.NewProxyTrust(true, nil)}
	for range 10 {
		// Next's server-side fetches reach the backend from the web container without the edge's X-Forwarded-For.
		if c := limited(s, "172.18.0.3:1", ""); c != 200 {
			t.Fatalf("server-side render throttled as one shared client: %d", c)
		}
	}
}

func TestRateLimitIgnoresForwardedHeaderFromUntrustedPeer(t *testing.T) {
	s := &Server{proxy: auth.NewProxyTrust(true, []string{"10.0.0.2/32"})}
	// Another container on a shared docker network forging the edge's header: limited by its own address.
	for range 2 {
		limited(s, "10.0.0.9:1", "127.0.0.1")
	}
	if c := limited(s, "10.0.0.9:1", ""); c != http.StatusTooManyRequests {
		t.Fatalf("untrusted peer escaped the limit: %d", c)
	}
	// The trusted web container without the header is a server-side render: exempt.
	for range 5 {
		if c := limited(s, "10.0.0.2:1", ""); c != 200 {
			t.Fatalf("trusted SSR throttled: %d", c)
		}
	}
}

func TestRateLimitBoundsServerSideRenders(t *testing.T) {
	s := &Server{proxy: auth.NewProxyTrust(true, nil)}
	// Renders share one larger budget: a page URL must not be an unlimited way around the limit.
	for range 2 * ssrBudgetFactor {
		if c := limited(s, "172.18.0.3:1", ""); c != 200 {
			t.Fatalf("render throttled within the shared budget: %d", c)
		}
	}
	if c := limited(s, "172.18.0.3:1", ""); c != http.StatusTooManyRequests {
		t.Fatalf("renders beyond the shared budget: %d", c)
	}
	// Browsers behind the proxy keep their own budgets.
	if c := limited(s, "172.18.0.3:1", "203.0.113.7"); c != 200 {
		t.Fatalf("browser throttled by the render budget: %d", c)
	}
}

func TestRateLimitIgnoresForgedLoopbackHeader(t *testing.T) {
	s := &Server{proxy: auth.NewProxyTrust(true, nil)}
	for range 2 {
		limited(s, "172.18.0.9:1", "127.0.0.1")
	}
	if c := limited(s, "172.18.0.9:1", "127.0.0.1"); c != http.StatusTooManyRequests {
		t.Fatalf("a forwarded 127.0.0.1 escaped the limit: %d", c)
	}
}

func TestClientBucketGroupsIPv6Prefix(t *testing.T) {
	if a, b := clientBucket("2001:db8:1:2::1"), clientBucket("2001:db8:1:2:ffff::9"); a != b {
		t.Fatalf("same /64 split: %q vs %q", a, b)
	}
	if a, b := clientBucket("2001:db8:1:2::1"), clientBucket("2001:db8:1:3::1"); a == b {
		t.Fatal("different /64 merged")
	}
	if got := clientBucket("203.0.113.7"); got != "203.0.113.7" {
		t.Fatalf("IPv4 changed: %q", got)
	}
}

func TestRateLimitRenderBudgetLeavesRouteLimitAlone(t *testing.T) {
	s := &Server{proxy: auth.NewProxyTrust(true, nil)}
	// One middleware instance serves every request, as on the router.
	h := s.rateLimit("t", 2, time.Minute)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	do := func(xff string) int {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = "172.18.0.3:1"
		if xff != "" {
			r.Header.Set("X-Forwarded-For", xff)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	for range 2 * ssrBudgetFactor {
		do("")
	}
	for i := range 3 {
		want := http.StatusOK
		if i == 2 {
			want = http.StatusTooManyRequests
		}
		if c := do("203.0.113.7"); c != want {
			t.Fatalf("browser request %d after renders: %d, want %d", i, c, want)
		}
	}
}

func TestRateLimiterEvictsSingleHitKeysFirst(t *testing.T) {
	var l rateLimiter
	for range 3 {
		l.hit("brute", 2, time.Minute)
	}
	for i := range rateLimiterMaxKeys + 10 {
		l.hit("flood-"+time.Duration(i).String(), 2, time.Minute)
	}
	if l.hit("brute", 2, time.Minute) {
		t.Fatal("a flood of fresh keys reset a client that is being throttled")
	}
}

func TestRateLimitLimitsLoopbackProxy(t *testing.T) {
	s := &Server{}
	// A proxy on this host relaying browsers (dev) is not ffmpeg.
	for range 2 {
		limited(s, "127.0.0.1:1", "203.0.113.7")
	}
	if c := limited(s, "127.0.0.1:1", "203.0.113.7"); c != http.StatusTooManyRequests {
		t.Fatalf("loopback proxy escaped the limit: %d", c)
	}
}
