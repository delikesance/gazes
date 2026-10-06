package auth

import (
	"context"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// proxyRefresh bounds how often an unknown peer triggers a new lookup of the proxy hostnames
// (a recreated web container comes back with a new address).
const proxyRefresh = 2 * time.Second

// ProxyTrust decides whether a request's X-Forwarded-* headers were set by the edge proxy. With a
// list of hosts (names, IPs or CIDRs), only those socket peers are believed: another container on a
// shared docker network could otherwise forge X-Forwarded-For. Without a list, any peer is.
type ProxyTrust struct {
	enabled bool
	nets    []*net.IPNet
	hosts   []string
	lookup  func(host string) ([]net.IP, error)

	mu         sync.Mutex
	resolved   map[string]bool
	resolvedAt time.Time
	refreshing bool
}

// proxyLookupTimeout bounds one resolution of the proxy hostnames.
const proxyLookupTimeout = 2 * time.Second

func lookupProxy(host string) ([]net.IP, error) {
	ctx, cancel := context.WithTimeout(context.Background(), proxyLookupTimeout)
	defer cancel()
	return net.DefaultResolver.LookupIP(ctx, "ip", host)
}

func NewProxyTrust(enabled bool, proxies []string) *ProxyTrust {
	p := &ProxyTrust{enabled: enabled, lookup: lookupProxy}
	for _, entry := range proxies {
		if ip := net.ParseIP(entry); ip != nil {
			entry += map[bool]string{true: "/32", false: "/128"}[ip.To4() != nil]
		}
		if _, n, err := net.ParseCIDR(entry); err == nil {
			p.nets = append(p.nets, n)
		} else {
			p.hosts = append(p.hosts, entry)
		}
	}
	return p
}

// Trusts reports whether r came straight from a trusted proxy. Nil-safe.
func (p *ProxyTrust) Trusts(r *http.Request) bool {
	if p == nil || !p.enabled {
		return false
	}
	if len(p.nets) == 0 && len(p.hosts) == 0 {
		return true
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	if ip == nil {
		return false
	}
	for _, n := range p.nets {
		if n.Contains(ip) {
			return true
		}
	}
	if len(p.hosts) == 0 {
		return false
	}
	// The lookup runs outside the lock: a slow resolver must not stall every request, and only
	// one caller refreshes at a time (the others answer from the last known addresses).
	key := ip.String()
	p.mu.Lock()
	known := p.resolved[key]
	refresh := !known && !p.refreshing && time.Since(p.resolvedAt) >= proxyRefresh
	if refresh {
		p.refreshing, p.resolvedAt = true, time.Now()
	}
	p.mu.Unlock()
	if !refresh {
		return known
	}
	next, ok := p.resolve()
	p.mu.Lock()
	defer p.mu.Unlock()
	p.refreshing = false
	if ok {
		p.resolved = next
	}
	return p.resolved[key]
}

// resolve looks every proxy hostname up; ok is false on a DNS error, so the caller keeps the last
// known addresses through a hiccup.
func (p *ProxyTrust) resolve() (map[string]bool, bool) {
	next := map[string]bool{}
	for _, h := range p.hosts {
		ips, err := p.lookup(h)
		if err != nil {
			return nil, false
		}
		for _, ip := range ips {
			next[ip.String()] = true
		}
	}
	return next, true
}

// Open reports trust in any peer: TRUST_PROXY without TRUSTED_PROXIES.
func (p *ProxyTrust) Open() bool {
	return p != nil && p.enabled && len(p.nets) == 0 && len(p.hosts) == 0
}
