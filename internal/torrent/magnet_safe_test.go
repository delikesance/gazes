package torrent

import (
	"context"
	"net"
	"strings"
	"testing"
)

func TestSanitizeMagnetDropsServerSideFetchSources(t *testing.T) {
	const hash = "0123456789abcdef0123456789abcdef01234567"
	in := "magnet:?xt=urn:btih:" + hash +
		"&ws=http://127.0.0.1:8080/x&xs=http://10.0.0.1/a.torrent&as=http://169.254.169.254/" +
		"&tr=http://192.168.1.5/announce&tr=udp://[::1]:80&tr=http://localhost/announce&tr=udp://tracker.example.org:1337/announce"
	got, err := sanitizeMagnet(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"ws=", "xs=", "as=", "127.0.0.1", "10.0.0.1", "192.168", "localhost", "::1"} {
		if strings.Contains(got, bad) {
			t.Fatalf("%q survived in %q", bad, got)
		}
	}
	if !strings.Contains(got, "tracker.example.org") || !strings.Contains(got, hash) {
		t.Fatalf("public tracker or hash lost: %q", got)
	}
}

func TestTrackerDialRefusesInternalAddresses(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	// A hostname is checked after resolution: "localhost" (or a DNS name pointing at a docker service) is refused too.
	for _, addr := range []string{ln.Addr().String(), "localhost:" + port} {
		if conn, err := safeDialContext(context.Background(), "tcp", addr); err == nil {
			conn.Close()
			t.Fatalf("dialed internal tracker %s", addr)
		}
	}
}

func TestTrackerPacketConnRefusesInternalAddresses(t *testing.T) {
	pc, err := safeListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	for _, ip := range []string{"127.0.0.1", "10.1.2.3", "172.18.0.4", "100.64.0.1", "169.254.169.254", "::1"} {
		if _, err := pc.WriteTo([]byte("x"), &net.UDPAddr{IP: net.ParseIP(ip), Port: 6969}); err == nil {
			t.Fatalf("sent a UDP announce to %s", ip)
		}
	}
}
