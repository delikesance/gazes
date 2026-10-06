package torrent

import (
	"net"
	"net/url"
	"strings"

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
		return !(ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified())
	}
	return true
}
