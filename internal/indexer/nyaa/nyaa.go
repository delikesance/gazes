package nyaa

import (
	"bufio"
	"context"
	"encoding/xml"
	"fmt"
	"github.com/gazes/gazes/internal/diagnostics"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gazes/gazes/internal/indexer"
)

const (
	DefaultBaseURL   = "https://nyaa.si"
	DefaultUserAgent = "Gazes/1.0 (+https://github.com/gazes/gazes)"

	// SukebeiBaseURL hosts the adult releases (hentai) that Nyaa.si does not carry.
	SukebeiBaseURL = "https://sukebei.nyaa.si"
	// CategorySukebeiAnime is Sukebei's "Art - Anime"; its category tree differs from Nyaa's.
	CategorySukebeiAnime = "1_1"

	// Categories
	CategoryAllAnime                  = "1_0"
	CategoryAnimeEnglishTranslated    = "1_2"
	CategoryAnimeNonEnglishTranslated = "1_3"
	CategoryAnimeRaw                  = "1_4"
)

// DefaultTrackers list for constructing reliable Magnet URIs.
var DefaultTrackers = []string{
	"http://nyaa.tracker.wf:7777/announce",
	"udp://open.stealth.si:80/announce",
	"udp://tracker.opentrackr.org:1337/announce",
	"udp://exodus.desync.com:6969/announce",
	"udp://tracker.torrent.eu.org:451/announce",
}

// Client implements indexer.Provider for Nyaa.si.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	UserAgent  string
	sukebei    bool
}

// NewClient creates a new Nyaa client.
func NewClient(baseURL string, httpClient *http.Client) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 15 * time.Second,
		}
	}
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		HTTPClient: httpClient,
		UserAgent:  DefaultUserAgent,
	}
}

// NewSukebeiClient creates a client for sukebei.nyaa.si, restricted to its anime category.
func NewSukebeiClient(baseURL string, httpClient *http.Client) *Client {
	if baseURL == "" {
		baseURL = SukebeiBaseURL
	}
	c := NewClient(baseURL, httpClient)
	c.sukebei = true
	return c
}

func (c *Client) Name() string {
	if c.sukebei {
		return "sukebei.nyaa.si"
	}
	return "nyaa.si"
}

// Search queries Nyaa.si RSS feed with the given search options.
func (c *Client) Search(ctx context.Context, opts indexer.SearchOptions) ([]indexer.TorrentItem, error) {
	reqURL, err := c.buildURL(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to build nyaa url: %w", err)
	}

	return c.fetchAndParse(ctx, reqURL)
}

// GetLatest retrieves the latest anime releases from Nyaa.si.
func (c *Client) GetLatest(ctx context.Context, category string, page int) ([]indexer.TorrentItem, error) {
	if category == "" {
		category = CategoryAnimeEnglishTranslated
	}
	opts := indexer.SearchOptions{
		Category: category,
		Page:     page,
		SortBy:   "id",
		Order:    "desc",
	}
	return c.Search(ctx, opts)
}

func (c *Client) buildURL(opts indexer.SearchOptions) (string, error) {
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return "", err
	}

	q := u.Query()
	// RSS is a latest-release feed: Nyaa ignores its sorting and pagination.
	// Discovery needs the sorted, paginated listing to reach older season packs.
	if opts.SortBy != "seeders" && opts.Page <= 1 {
		q.Set("page", "rss")
	} else {
		q.Del("page")
	}

	if opts.Query != "" {
		q.Set("q", opts.Query)
	}
	if c.sukebei {
		q.Set("c", CategorySukebeiAnime)
	} else if opts.Category != "" {
		q.Set("c", opts.Category)
	} else {
		q.Set("c", CategoryAnimeEnglishTranslated)
	}
	if opts.SortBy != "" {
		q.Set("s", opts.SortBy)
	}
	if opts.Order != "" {
		q.Set("o", opts.Order)
	}
	if opts.Page > 1 {
		q.Set("p", strconv.Itoa(opts.Page))
	}

	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (c *Client) fetchAndParse(ctx context.Context, reqURL string) ([]indexer.TorrentItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	diagnostics.Log(ctx, slog.LevelDebug, "provider.http", "provider", c.Name(), "status", resp.StatusCode)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, indexer.NewHTTPError(c.Name(), resp)
	}

	reader := bufio.NewReader(io.LimitReader(resp.Body, 4<<20))
	prefix, _ := reader.Peek(256)
	if !strings.HasPrefix(strings.TrimSpace(string(prefix)), "<?xml") && !strings.HasPrefix(strings.TrimSpace(string(prefix)), "<rss") {
		return c.parseListing(reader)
	}
	var rssDoc NyaaRSS
	if err := xml.NewDecoder(reader).Decode(&rssDoc); err != nil {
		return nil, fmt.Errorf("failed to decode rss xml: %w", err)
	}

	items := make([]indexer.TorrentItem, 0, len(rssDoc.Channel.Items))
	for _, item := range rssDoc.Channel.Items {
		parsedItem := c.mapItem(item)
		items = append(items, parsedItem)
	}

	return items, nil
}

func (c *Client) mapItem(item NyaaItem) indexer.TorrentItem {
	pubDate, _ := time.Parse(time.RFC1123Z, item.PubDate)
	if pubDate.IsZero() {
		pubDate, _ = time.Parse(time.RFC1123, item.PubDate)
	}

	sizeBytes := ParseSizeBytes(item.Size)
	magnet := BuildMagnetURI(item.InfoHash, item.Title, DefaultTrackers)

	id := extractIDFromLink(item.Link)
	if id == "" {
		id = item.InfoHash
	}
	// An exact-source hint lets the engine resolve a large pack's file list over HTTP.
	// The torrent library verifies downloaded metadata against the magnet info hash.
	torrentURL := ""
	if _, err := strconv.ParseUint(id, 10, 64); err == nil {
		torrentURL = c.BaseURL + "/download/" + id + ".torrent"
		magnet += "&xs=" + url.QueryEscape(torrentURL)
	}

	return indexer.TorrentItem{
		ID:          id,
		Title:       item.Title,
		InfoHash:    strings.ToLower(item.InfoHash),
		MagnetURI:   magnet,
		TorrentURL:  torrentURL,
		SizeBytes:   sizeBytes,
		SizeDisplay: item.Size,
		Seeders:     item.Seeders,
		Leechers:    item.Leechers,
		Downloads:   item.Downloads,
		Category:    item.Category,
		PublishDate: pubDate,
	}
}

func extractIDFromLink(link string) string {
	u, err := url.Parse(link)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.TrimRight(u.Path, "/"), "/")
	if len(parts) > 0 {
		return strings.TrimSuffix(parts[len(parts)-1], ".torrent")
	}
	return ""
}

// BuildMagnetURI constructs a valid magnet link with trackers.
func BuildMagnetURI(infoHash string, title string, trackers []string) string {
	if infoHash == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("magnet:?xt=urn:btih:")
	b.WriteString(strings.ToLower(infoHash))

	if title != "" {
		b.WriteString("&dn=")
		b.WriteString(url.QueryEscape(title))
	}

	for _, tr := range trackers {
		b.WriteString("&tr=")
		b.WriteString(url.QueryEscape(tr))
	}

	return b.String()
}

// ParseSizeBytes parses string formats like "1.4 GiB", "500.5 MiB", "12.0 KiB", "1024 Bytes".
func ParseSizeBytes(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	parts := strings.Fields(s)
	if len(parts) < 2 {
		val, err := strconv.ParseInt(s, 10, 64)
		if err == nil {
			return val
		}
		return 0
	}

	val, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0
	}

	unit := strings.ToUpper(parts[1])
	switch unit {
	case "TIB", "TB":
		return int64(val * 1024 * 1024 * 1024 * 1024)
	case "GIB", "GB":
		return int64(val * 1024 * 1024 * 1024)
	case "MIB", "MB":
		return int64(val * 1024 * 1024)
	case "KIB", "KB":
		return int64(val * 1024)
	case "B", "BYTES":
		return int64(val)
	default:
		return int64(val)
	}
}
