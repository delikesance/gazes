package torznab

import (
	"context"
	"github.com/gazes/gazes/internal/indexer"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFeeds(t *testing.T) {
	for _, name := range []string{"ext", "magnetdl", "thepiratebay", "anidex"} {
		t.Run(name, func(t *testing.T) {
			h := strings.Repeat("a", 40)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("offset") != "75" || r.URL.Query().Get("apikey") != "secret" || r.URL.Query().Get("cat") != "5000,5070" {
					t.Errorf("incorrect query: %v", r.URL.Query())
				}
				w.Header().Set("Content-Type", "application/xml")
				w.Write([]byte(`<rss xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel><item><title>Slime S01 FRENCH</title><guid>http://gateway/download?apikey=secret</guid><link>http://gateway/download?apikey=secret</link><torznab:attr name="infohash" value="` + h + `"/><torznab:attr name="seeders" value="9"/><torznab:attr name="size" value="1234"/></item></channel></rss>`))
			}))
			defer srv.Close()
			c, err := New(name, srv.URL, "secret")
			if err != nil {
				t.Fatal(err)
			}
			items, err := c.Search(context.Background(), indexer.SearchOptions{Query: "Slime", Page: 2})
			if err != nil || len(items) != 1 || items[0].Seeders != 9 || items[0].SizeBytes != 1234 || items[0].Provider != name {
				t.Fatalf("%+v %v", items, err)
			}
			if items[0].TorrentURL != "" || strings.Contains(items[0].MagnetURI, "secret") || strings.Contains(items[0].ID, "secret") {
				t.Fatal("API key leaked")
			}
		})
	}
}
func TestInvalidFeedsAndEndpoints(t *testing.T) {
	for _, feed := range []string{`<error code="100"/>`, `<html>blocked</html>`, `broken`} {
		if _, err := Parse(strings.NewReader(feed), "ext"); err == nil {
			t.Fatal("invalid feed accepted")
		}
	}
	if _, err := New("ext", "file:///secret", ""); err == nil {
		t.Fatal("invalid scheme accepted")
	}
}

// Exercise gateway parsing and the actual multi-provider resolver together.
func TestGatewayPartialMergeAndFrenchRanking(t *testing.T) {
	makeGateway := func(name, body string, status int) *Client {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status); w.Write([]byte(body)) }))
		t.Cleanup(srv.Close)
		client, err := New(name, srv.URL, "private-key")
		if err != nil {
			t.Fatal(err)
		}
		return client
	}
	item := func(title, hash, seeds string) string {
		return `<item><title>` + title + `</title><attr name="infohash" value="` + hash + `"/><attr name="seeders" value="` + seeds + `"/></item>`
	}
	a, b := strings.Repeat("a", 40), strings.Repeat("b", 40)
	first := makeGateway("anidex", `<rss><channel>`+item("Example S01E01 VF", a, "2")+`</channel></rss>`, 200)
	second := makeGateway("thepiratebay", `<rss><channel>`+item("Example S01E01 VF", a, "3")+item("Example S01E01 VOSTFR", b, "200")+`</channel></rss>`, 200)
	failed := makeGateway("offline", "unavailable", 503)
	resolver := indexer.NewEpisodeResolver(indexer.NewMultiProvider(first, second, failed))
	result, err := resolver.ResolveSeasonSources(context.Background(), indexer.EpisodeIdentity{Titles: []string{"Example"}, SeasonNumber: 1, EpisodeNumber: 1})
	if err != nil || !result.Partial || result.TotalSources != 2 {
		t.Fatalf("partial merge failed: %+v %v", result, err)
	}
	if result.Sources[0].InfoHash != a || result.Sources[0].Seeders != 3 || !result.Sources[0].IsFrench {
		t.Fatalf("VF preference or deduplication lost: %+v", result.Sources)
	}
}
