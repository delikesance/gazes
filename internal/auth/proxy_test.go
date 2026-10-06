package auth

import (
	"net"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProxyTrustOnlyHonoursConfiguredPeers(t *testing.T) {
	p := NewProxyTrust(true, []string{"web", "10.9.0.0/16"})
	p.lookup = func(host string) ([]net.IP, error) {
		if host != "web" {
			t.Fatalf("unexpected lookup %q", host)
		}
		return []net.IP{net.ParseIP("172.20.0.5")}, nil
	}
	for addr, want := range map[string]bool{
		"172.20.0.5:4000": true,  // the web container
		"10.9.3.4:1":      true,  // configured network
		"172.20.0.9:4000": false, // another container on a shared network
		"203.0.113.7:1":   false,
	} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = addr
		r.Header.Set("X-Forwarded-For", "127.0.0.1")
		if got := p.Trusts(r); got != want {
			t.Errorf("%s: trusted=%v, want %v", addr, got, want)
		}
		if !want && ClientIP(r, p.Trusts(r)) == "127.0.0.1" {
			t.Errorf("%s: forged X-Forwarded-For honoured", addr)
		}
	}
}

func TestProxyTrustDefaults(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	if (*ProxyTrust)(nil).Trusts(r) || NewProxyTrust(false, []string{"web"}).Trusts(r) {
		t.Fatal("disabled trust must not honour forwarded headers")
	}
	if !NewProxyTrust(true, nil).Trusts(r) {
		t.Fatal("TRUST_PROXY without a list keeps trusting any peer")
	}
}

func TestProxyTrustLookupDoesNotBlockOtherRequests(t *testing.T) {
	p := NewProxyTrust(true, []string{"web", "10.9.0.0/16"})
	started, release := make(chan struct{}), make(chan struct{})
	p.lookup = func(string) ([]net.IP, error) {
		close(started)
		<-release
		return []net.IP{net.ParseIP("172.20.0.5")}, nil
	}
	slow := httptest.NewRequest("GET", "/", nil)
	slow.RemoteAddr = "172.20.0.5:1"
	done := make(chan bool)
	go func() { done <- p.Trusts(slow) }()
	<-started
	// While a lookup hangs, other peers (here an unknown one) answer at once from the last known state.
	other := httptest.NewRequest("GET", "/", nil)
	other.RemoteAddr = "172.20.0.9:1"
	answered := make(chan bool)
	go func() { answered <- p.Trusts(other) }()
	select {
	case got := <-answered:
		if got {
			t.Fatal("unknown peer trusted")
		}
	case <-time.After(time.Second):
		t.Fatal("Trusts blocked behind a slow DNS lookup")
	}
	close(release)
	if !<-done {
		t.Fatal("resolved proxy not trusted")
	}
}
