package nyaa_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gazes/gazes/internal/indexer"
	"github.com/gazes/gazes/internal/indexer/nyaa"
)

func TestSortedSearchUsesPaginatedListing(t *testing.T) {
	fixture, err := os.ReadFile("testdata/tensura-listing.html")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("page") != "" || q.Get("s") != "seeders" || q.Get("p") != "2" || q.Get("c") != "1_0" {
			t.Errorf("sorting/pagination lost: %v", q)
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write(fixture)
	}))
	defer ts.Close()
	items, err := nyaa.NewClient(ts.URL, ts.Client()).Search(context.Background(), indexer.SearchOptions{Query: "Tensura MULTI", Category: "1_0", SortBy: "seeders", Order: "desc", Page: 2})
	if err != nil || len(items) != 1 {
		t.Fatalf("%+v %v", items, err)
	}
	item := items[0]
	if item.ID != "2166021" || item.InfoHash != "d665082ce8d007d287c8f180567c12d879f62663" || item.Seeders != 1605 || item.Leechers != 17 || item.Downloads != 14089 || item.SizeBytes <= 0 || item.PublishDate.IsZero() {
		t.Fatalf("listing metadata lost: %+v", item)
	}
	magnet, _ := url.Parse(item.MagnetURI)
	if magnet.Query().Get("xs") != ts.URL+"/download/2166021.torrent" {
		t.Fatal("direct torrent metadata URL missing")
	}
}

func TestListingChallengeIsNotAnEmptySearch(t *testing.T) {
	for _, tt := range []struct {
		body   string
		failed bool
	}{
		{"<html>Cloudflare captcha</html>", true},
		{"<h3>No results found</h3>", false},
		{`<table class="torrent-list"><tbody></tbody></table>`, false},
		{`<table class="torrent-list"><tr><td>broken row</td></tr></table>`, false},
	} {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(tt.body)) }))
		_, err := nyaa.NewClient(ts.URL, ts.Client()).Search(context.Background(), indexer.SearchOptions{SortBy: "seeders"})
		ts.Close()
		if (err != nil) != tt.failed {
			t.Errorf("%s: %v", tt.body, err)
		}
	}
}

func TestRSSDownloadLinksProvideDirectMetadataSource(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(strings.ReplaceAll(sampleRSS, "https://nyaa.si/view/1789012", "https://nyaa.si/download/1789012.torrent?download=1")))
	}))
	defer ts.Close()
	items, err := nyaa.NewClient(ts.URL, ts.Client()).Search(context.Background(), indexer.SearchOptions{Query: "Naruto VF"})
	if err != nil {
		t.Fatal(err)
	}
	item := items[0]
	u, err := url.Parse(item.MagnetURI)
	if err != nil {
		t.Fatal(err)
	}
	want := ts.URL + "/download/1789012.torrent"
	if item.ID != "1789012" || item.TorrentURL != want || u.Query().Get("xs") != want {
		t.Fatalf("RSS download link lost direct metadata: %+v", item)
	}
}

const sampleRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom" xmlns:nyaa="https://nyaa.si/xmlns/nyaa">
    <channel>
        <title>Nyaa - Anime - English-translated</title>
        <description>RSS Feed for Anime - English-translated</description>
        <link>https://nyaa.si/</link>
        <item>
            <title>[SubsPlease] Frieren - Beyond Journey's End - 28 (1080p) [1234ABCD].mkv</title>
            <link>https://nyaa.si/view/1789012</link>
            <guid isPermaLink="true">https://nyaa.si/view/1789012</guid>
            <pubDate>Fri, 22 Mar 2024 15:30:00 -0000</pubDate>
            <nyaa:seeders>150</nyaa:seeders>
            <nyaa:leechers>12</nyaa:leechers>
            <nyaa:downloads>4200</nyaa:downloads>
            <nyaa:infoHash>0123456789abcdef0123456789abcdef01234567</nyaa:infoHash>
            <nyaa:categoryId>1_2</nyaa:categoryId>
            <nyaa:category>Anime - English-translated</nyaa:category>
            <nyaa:size>1.4 GiB</nyaa:size>
            <nyaa:comments>5</nyaa:comments>
            <nyaa:trusted>Yes</nyaa:trusted>
            <nyaa:remake>No</nyaa:remake>
        </item>
        <item>
            <title>[Erai-raws] Dungeon Meshi - 12 [1080p][Multiple Subtitle].mkv</title>
            <link>https://nyaa.si/view/1789013</link>
            <guid isPermaLink="true">https://nyaa.si/view/1789013</guid>
            <pubDate>Thu, 21 Mar 2024 14:00:00 -0000</pubDate>
            <nyaa:seeders>85</nyaa:seeders>
            <nyaa:leechers>4</nyaa:leechers>
            <nyaa:downloads>2100</nyaa:downloads>
            <nyaa:infoHash>abcdef0123456789abcdef0123456789abcdef01</nyaa:infoHash>
            <nyaa:categoryId>1_2</nyaa:categoryId>
            <nyaa:category>Anime - English-translated</nyaa:category>
            <nyaa:size>850.5 MiB</nyaa:size>
            <nyaa:comments>2</nyaa:comments>
            <nyaa:trusted>No</nyaa:trusted>
            <nyaa:remake>No</nyaa:remake>
        </item>
    </channel>
</rss>`

func TestParseSizeBytes(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"1.4 GiB", 1503238553},
		{"850.5 MiB", 891813888},
		{"500 KiB", 500 * 1024},
		{"1024 Bytes", 1024},
		{"", 0},
	}

	for _, tc := range tests {
		got := nyaa.ParseSizeBytes(tc.input)
		if got != tc.expected {
			t.Errorf("ParseSizeBytes(%q) = %d, expected %d", tc.input, got, tc.expected)
		}
	}
}

func TestBuildMagnetURI(t *testing.T) {
	hash := "0123456789abcdef0123456789abcdef01234567"
	title := "[SubsPlease] Frieren 28"
	trackers := []string{"http://nyaa.tracker.wf:7777/announce"}

	uri := nyaa.BuildMagnetURI(hash, title, trackers)
	if !strings.HasPrefix(uri, "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567") {
		t.Errorf("unexpected magnet uri prefix: %s", uri)
	}
	if !strings.Contains(uri, "&dn=") {
		t.Errorf("magnet uri missing dn parameter: %s", uri)
	}
	if !strings.Contains(uri, "&tr=") {
		t.Errorf("magnet uri missing tr parameter: %s", uri)
	}
}

func TestClient_Search(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("page") != "rss" {
			t.Errorf("expected page=rss, got %s", q.Get("page"))
		}
		if q.Get("q") != "Frieren" {
			t.Errorf("expected q=Frieren, got %s", q.Get("q"))
		}

		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(sampleRSS))
	}))
	defer ts.Close()

	client := nyaa.NewClient(ts.URL, ts.Client())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	items, err := client.Search(ctx, indexer.SearchOptions{
		Query:    "Frieren",
		Category: "1_2",
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	// Verify first item
	item := items[0]
	if item.ID != "1789012" {
		t.Errorf("expected ID '1789012', got '%s'", item.ID)
	}
	if !strings.Contains(item.Title, "Frieren") {
		t.Errorf("unexpected title: %s", item.Title)
	}
	if item.Seeders != 150 {
		t.Errorf("expected 150 seeders, got %d", item.Seeders)
	}
	if item.Leechers != 12 {
		t.Errorf("expected 12 leechers, got %d", item.Leechers)
	}
	if item.InfoHash != "0123456789abcdef0123456789abcdef01234567" {
		t.Errorf("unexpected infoHash: %s", item.InfoHash)
	}
	if !strings.HasPrefix(item.MagnetURI, "magnet:?xt=urn:btih:") {
		t.Errorf("unexpected magnet uri: %s", item.MagnetURI)
	}
	if item.SizeBytes <= 0 {
		t.Errorf("expected positive SizeBytes, got %d", item.SizeBytes)
	}
}

func TestSukebeiClientSearchesAnimeCategory(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sukebei has its own category tree: Nyaa's 1_2/1_3/1_4 would hit doujinshi, games or nothing.
		if c := r.URL.Query().Get("c"); c != "1_1" {
			t.Errorf("category = %q, want 1_1 (Sukebei anime)", c)
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss xmlns:nyaa="https://nyaa.si/xmlns/nyaa"><channel></channel></rss>`))
	}))
	defer ts.Close()
	client := nyaa.NewSukebeiClient(ts.URL, ts.Client())
	if client.Name() != "sukebei.nyaa.si" {
		t.Fatalf("name = %q", client.Name())
	}
	for _, category := range []string{"", "1_0", "1_3"} {
		if _, err := client.Search(context.Background(), indexer.SearchOptions{Query: "Overflow", Category: category}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSukebeiRSSNamespaceIsParsed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss xmlns:nyaa="https://sukebei.nyaa.si/xmlns/nyaa" version="2.0"><channel><item>
<title>[Group] Overflow 01</title><link>https://sukebei.nyaa.si/download/123.torrent</link><guid>https://sukebei.nyaa.si/view/123</guid>
<nyaa:seeders>7</nyaa:seeders><nyaa:infoHash>ABCDEF0123456789ABCDEF0123456789ABCDEF01</nyaa:infoHash><nyaa:size>1.0 GiB</nyaa:size></item></channel></rss>`))
	}))
	defer ts.Close()
	items, err := nyaa.NewSukebeiClient(ts.URL, ts.Client()).Search(context.Background(), indexer.SearchOptions{Query: "Overflow"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Seeders != 7 || items[0].InfoHash != "abcdef0123456789abcdef0123456789abcdef01" || items[0].SizeBytes == 0 {
		t.Fatalf("items = %+v", items)
	}
}
