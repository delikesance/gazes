// Package torznab connects public indexers through their maintained Torznab gateway.
package torznab

import (
	"context"
	"encoding/base32"
	"encoding/hex"
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

type Client struct {
	NameValue, Endpoint, APIKey string
	HTTPClient                  *http.Client
	// IndexerOnly marks every result as discovery-only (see indexer.TorrentItem).
	IndexerOnly bool
}

func New(name, endpoint, key string) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return nil, fmt.Errorf("invalid %s Torznab endpoint", name)
	}
	return &Client{NameValue: name, Endpoint: endpoint, APIKey: key, HTTPClient: &http.Client{Timeout: 5 * time.Second}}, nil
}
func (c *Client) Name() string { return c.NameValue }
func (c *Client) GetLatest(ctx context.Context, _ string, page int) ([]indexer.TorrentItem, error) {
	return c.Search(ctx, indexer.SearchOptions{Page: page})
}
func (c *Client) Search(ctx context.Context, o indexer.SearchOptions) ([]indexer.TorrentItem, error) {
	u, _ := url.Parse(c.Endpoint)
	q := u.Query()
	q.Set("t", "search")
	q.Set("q", o.Query)
	q.Set("extended", "1")
	q.Set("limit", "75")
	q.Set("offset", strconv.Itoa(max(0, o.Page-1)*75))
	// Nyaa's category IDs have no meaning in Torznab. Include season packs filed as TV.
	q.Set("cat", "5000,5070")
	if c.APIKey != "" {
		q.Set("apikey", c.APIKey)
	}
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("%s: invalid request", c.Name())
	}
	req.Header.Set("Accept", "application/rss+xml, application/xml")
	req.Header.Set("User-Agent", "Gazes/1.0")
	res, err := c.HTTPClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%s: request interrupted: %w", c.Name(), ctx.Err())
		}
		return nil, fmt.Errorf("%s: request failed: %s", c.Name(), diagnostics.Redact(err.Error()))
	}
	diagnostics.Log(ctx, slog.LevelDebug, "provider.http", "provider", c.Name(), "status", res.StatusCode)
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", c.Name(), res.StatusCode)
	}
	items, err := Parse(io.LimitReader(res.Body, 4<<20), c.Name())
	for i := range items {
		items[i].IndexerOnly = c.IndexerOnly
		if c.IndexerOnly {
			items[i].MagnetURI = indexer.PrivateMagnet(items[i])
		} else if c.APIKey != "" && strings.Contains(items[i].MagnetURI, c.APIKey) {
			q := url.Values{"xt": {"urn:btih:" + items[i].InfoHash}, "dn": {items[i].Title}}
			items[i].MagnetURI = "magnet:?" + q.Encode()
		}
	}
	return items, err
}

type attribute struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}
type entry struct {
	Title     string `xml:"title"`
	GUID      string `xml:"guid"`
	Link      string `xml:"link"`
	Date      string `xml:"pubDate"`
	Size      int64  `xml:"size"`
	Enclosure struct {
		URL    string `xml:"url,attr"`
		Length int64  `xml:"length,attr"`
	} `xml:"enclosure"`
	Attrs []attribute `xml:"attr"`
}
type feed struct {
	XMLName xml.Name `xml:"rss"`
	Channel struct {
		Items []entry `xml:"item"`
	} `xml:"channel"`
}

func hash(value string) string {
	value = strings.TrimSpace(value)
	if len(value) == 40 {
		if _, err := hex.DecodeString(value); err == nil {
			return strings.ToLower(value)
		}
	}
	if len(value) == 32 {
		if b, err := base32.StdEncoding.DecodeString(strings.ToUpper(value)); err == nil {
			return hex.EncodeToString(b)
		}
	}
	return ""
}
func Parse(r io.Reader, provider string) ([]indexer.TorrentItem, error) {
	var f feed
	if err := xml.NewDecoder(r).Decode(&f); err != nil {
		return nil, fmt.Errorf("%s: invalid Torznab feed", provider)
	}
	out := make([]indexer.TorrentItem, 0, len(f.Channel.Items))
	for _, e := range f.Channel.Items {
		item := indexer.TorrentItem{Title: e.Title, Provider: provider, SizeBytes: e.Size, Category: "Anime"}
		if item.SizeBytes == 0 {
			item.SizeBytes = e.Enclosure.Length
		}
		for _, a := range e.Attrs {
			switch strings.ToLower(a.Name) {
			case "infohash":
				item.InfoHash = hash(a.Value)
			case "magneturl":
				item.MagnetURI = a.Value
			case "seeders":
				item.Seeders, _ = strconv.Atoi(a.Value)
			case "leechers":
				item.Leechers, _ = strconv.Atoi(a.Value)
			case "size":
				item.SizeBytes, _ = strconv.ParseInt(a.Value, 10, 64)
			}
		}
		if item.MagnetURI == "" {
			for _, s := range []string{e.Link, e.Enclosure.URL} {
				if strings.HasPrefix(s, "magnet:?") {
					item.MagnetURI = s
					break
				}
			}
		}
		if item.MagnetURI != "" {
			u, err := url.Parse(item.MagnetURI)
			if err != nil || u.Scheme != "magnet" {
				item.MagnetURI = ""
			} else {
				for _, xt := range u.Query()["xt"] {
					if strings.HasPrefix(strings.ToLower(xt), "urn:btih:") {
						h := hash(xt[9:])
						if item.InfoHash == "" {
							item.InfoHash = h
						}
						if h != item.InfoHash {
							item.MagnetURI = ""
						}
					}
				}
			}
		}
		// Do not expose gateway download URLs: they commonly contain API keys.
		if item.InfoHash == "" || strings.TrimSpace(item.Title) == "" {
			continue
		}
		if item.MagnetURI == "" {
			q := url.Values{"xt": {"urn:btih:" + item.InfoHash}, "dn": {item.Title}}
			item.MagnetURI = "magnet:?" + q.Encode()
		}
		// Gateway GUIDs may also be credential-bearing download URLs.
		item.ID = provider + ":" + item.InfoHash
		item.PublishDate, _ = time.Parse(time.RFC1123Z, e.Date)
		item.SizeDisplay = fmt.Sprintf("%.2f GiB", float64(item.SizeBytes)/(1<<30))
		item.Seeders = max(0, item.Seeders)
		item.Leechers = max(0, item.Leechers)
		out = append(out, item)
	}
	return out, nil
}
