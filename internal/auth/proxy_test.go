package auth

import (
	"net"
	"net/http/httptest"
	"testing"
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
