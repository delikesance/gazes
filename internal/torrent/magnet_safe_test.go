package torrent

import (
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
