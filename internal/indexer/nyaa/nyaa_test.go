package nyaa_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gazes/gazes/internal/indexer"
	"github.com/gazes/gazes/internal/indexer/nyaa"
)

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
