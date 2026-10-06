package auth

import (
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
}

func NewProxyTrust(enabled bool, proxies []string) *ProxyTrust {
	p := &ProxyTrust{enabled: enabled, lookup: net.LookupIP}
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
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.resolved[ip.String()] && time.Since(p.resolvedAt) >= proxyRefresh {
		p.refresh()
	}
	return p.resolved[ip.String()]
}

func (p *ProxyTrust) refresh() {
	p.resolvedAt = time.Now()
	next := map[string]bool{}
	for _, h := range p.hosts {
		ips, err := p.lookup(h)
		if err != nil {
			return // keep the last known addresses through a DNS hiccup
		}
		for _, ip := range ips {
			next[ip.String()] = true
		}
	}
	p.resolved = next
}
