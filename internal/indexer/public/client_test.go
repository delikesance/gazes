package public

import (
	"context"
	"github.com/gazes/gazes/internal/indexer"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublicMagnetRows(t *testing.T) {
	h := strings.Repeat("a", 40)
	for _, provider := range []string{"ext", "magnetdl", "anidex"} {
		t.Run(provider, func(t *testing.T) {
			fixture := `<table><tr><td><a href="/release">Tensura S01 FRENCH</a></td><td><a href="magnet:?xt=urn:btih:` + h + `">Download</a></td><td class="seeders">12</td><td class="leechers">2</td></tr></table>`
			items, _, err := ParseHTML([]byte(fixture), "https://example.test/", provider)
			if err != nil || len(items) != 1 || items[0].Title != "Tensura S01 FRENCH" || items[0].Seeders != 12 || items[0].Provider != provider {
				t.Fatalf("%+v %v", items, err)
			}
		})
	}
}
func TestDetailLinksStayOnProvider(t *testing.T) {
	_, links, err := ParseHTML([]byte(`<a class="torrent-name" href="/release">Title</a><a class="torrent-name" href="https://evil.test/release">Other</a>`), "https://example.test/search/", "ext")
	if err != nil || len(links) != 1 || links[0] != "https://example.test/release" {
		t.Fatalf("%v %v", links, err)
	}
}
func TestBayParser(t *testing.T) {
	items, err := ParseBay([]byte(`[{"name":"Slime S01 VF","info_hash":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","seeders":"7","size":"123","added":"10"},{"name":"No results","info_hash":"0000000000000000000000000000000000000000"}]`))
	if err != nil || len(items) != 1 || items[0].Seeders != 7 || items[0].SizeBytes != 123 {
		t.Fatalf("%+v %v", items, err)
	}
	if _, err := ParseBay([]byte("<html>blocked</html>")); err == nil {
		t.Fatal("HTML accepted as JSON")
	}
}
func TestEndpointsAndHTTPFailures(t *testing.T) {
	for _, kind := range []string{"ext", "magnetdl", "thepiratebay", "anidex"} {
		t.Run(kind, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if kind == "magnetdl" && !strings.Contains(r.URL.Path, "slime-s01-vf") {
					t.Errorf("wrong search URL: %s", r.URL)
				}
				if kind != "magnetdl" && r.URL.Query().Get("q") != "Slime S01 VF" {
					t.Errorf("missing search query")
				}
				w.WriteHeader(403)
			}))
			defer srv.Close()
			c := New(kind, srv.URL)
			if _, err := c.Search(context.Background(), indexer.SearchOptions{Query: "Slime S01 VF"}); err == nil {
				t.Fatal("blocked provider treated as empty success")
			}
			if items, err := c.Search(context.Background(), indexer.SearchOptions{Page: 2}); err != nil || len(items) != 0 {
				t.Fatal("pagination repeats first page")
			}
		})
	}
}
