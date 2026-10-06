package torrent

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"syscall"
	"time"

	"github.com/anacrolix/torrent/metainfo"
)

// sanitizeMagnet strips what would make this host fetch attacker-chosen URLs: web seeds (ws),
// metainfo sources (xs), exact sources (as) and trackers pointing at loopback or private networks.
func sanitizeMagnet(uri string) (string, error) {
	m, err := metainfo.ParseMagnetUri(uri)
	if err != nil {
		return "", err
	}
	for _, k := range []string{"ws", "xs", "as"} {
		delete(m.Params, k)
	}
	kept := m.Trackers[:0]
	for _, tr := range m.Trackers {
		if publicTracker(tr) {
			kept = append(kept, tr)
		}
	}
	m.Trackers = kept
	return m.String(), nil
}

func publicTracker(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "udp") {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".internal") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return !internalIP(ip)
	}
	return true
}

var cgnat = &net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}

// internalIP reports addresses a tracker announce must never reach: this host, the docker
// networks, the LAN, carrier NAT and link-local (cloud metadata).
func internalIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() || cgnat.Contains(ip)
}

// trackerDialer checks the address actually dialed, after DNS resolution: publicTracker only sees
// the URL, so a hostname resolving to a docker service or a redirect would get past it.
var trackerDialer = &net.Dialer{
	Timeout: 15 * time.Second,
	Control: func(network, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		if ip := net.ParseIP(host); ip == nil || internalIP(ip) {
			return fmt.Errorf("tracker address %s refused: internal network", address)
		}
		return nil
	},
}

// safeDialContext is the dialer for HTTP and WebSocket tracker announces.
func safeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return trackerDialer.DialContext(ctx, network, addr)
}

// safeListenPacket opens the socket UDP tracker announces are written to, refusing internal destinations.
func safeListenPacket(network, addr string) (net.PacketConn, error) {
	pc, err := net.ListenPacket(network, addr)
	if err != nil {
		return nil, err
	}
	return publicPacketConn{pc}, nil
}

type publicPacketConn struct{ net.PacketConn }

func (c publicPacketConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	if udp, ok := addr.(*net.UDPAddr); !ok || internalIP(udp.IP) {
		return 0, fmt.Errorf("tracker address %s refused: internal network", addr)
	}
	return c.PacketConn.WriteTo(p, addr)
}
