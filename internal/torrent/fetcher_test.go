package torrent

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
)

func testTorrent(t *testing.T, private bool) ([]byte, string) {
	t.Helper()
	info := metainfo.Info{Name: "pack", PieceLength: 1 << 14, Length: 1 << 14, Pieces: make([]byte, 20)}
	if private {
		p := true
		info.Private = &p
	}
	raw, err := bencode.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	mi := metainfo.MetaInfo{InfoBytes: raw, Announce: "https://tracker.example/announce/passkey"}
	out, err := bencode.Marshal(mi)
	if err != nil {
		t.Fatal(err)
	}
	return out, mi.HashInfoBytes().HexString()
}

func TestURLTemplateFetcher(t *testing.T) {
	body, hash := testTorrent(t, true)
	other, missing := strings.Repeat("a", 40), strings.Repeat("b", 40)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("id") {
		case hash, other: // "other" receives a torrent whose hash differs from the request
			w.Write(body)
		case missing:
			w.WriteHeader(http.StatusNotFound)
		default:
			w.Write([]byte("<!DOCTYPE html><title>Connexion</title>"))
		}
	}))
	defer srv.Close()
	fetch := URLTemplateFetcher(srv.Client(), srv.URL+"/api?id={infohash}&apikey=sensitive-key")

	if mi, err := fetch(context.Background(), hash); err != nil || mi == nil || !isPrivate(mi) {
		t.Fatalf("expected the private torrent, got %v %v", mi, err)
	}
	if mi, err := fetch(context.Background(), missing); mi != nil || err != nil {
		t.Fatalf("an unknown torrent is a clean miss, got %v %v", mi, err)
	}
	for _, id := range []string{other, strings.Repeat("c", 40)} {
		if _, err := fetch(context.Background(), id); err == nil || strings.Contains(err.Error(), "sensitive-key") {
			t.Fatalf("expected a redacted error for %s, got %v", id, err)
		}
	}
}

func TestOnlyMarkedMagnetsFetchMetainfo(t *testing.T) {
	body, hash := testTorrent(t, true)
	calls := 0
	e := &ClientEngine{logger: discardLogger(), cfg: EngineConfig{MetainfoFetchers: map[string]MetainfoFetcher{
		"c411": func(context.Context, string) (*metainfo.MetaInfo, error) {
			calls++
			return metainfo.Load(strings.NewReader(string(body)))
		},
	}}}
	for _, uri := range []string{
		"magnet:?xt=urn:btih:" + hash,                     // public source
		"magnet:?xt=urn:btih:" + hash + "&xs=gazes:other", // unknown provider
		"magnet:?xt=urn:btih:" + hash + "&xs=https://example.com/x.torrent",
	} {
		m, _ := metainfo.ParseMagnetUri(uri)
		if e.fetchMetainfo(context.Background(), m) != nil {
			t.Fatalf("%s must not reach the private provider", uri)
		}
	}
	m, _ := metainfo.ParseMagnetUri("magnet:?xt=urn:btih:" + hash + "&xs=gazes:c411")
	if mi := e.fetchMetainfo(context.Background(), m); mi == nil || calls != 1 {
		t.Fatalf("marked magnet must fetch once, got %v after %d calls", mi, calls)
	}
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
