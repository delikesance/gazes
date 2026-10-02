// Package public implements direct, configurable public indexer clients.
package public

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gazes/gazes/internal/indexer"
	"golang.org/x/net/html"
)

type Client struct {
	Kind, BaseURL string
	HTTPClient    *http.Client
}

func New(kind, base string) *Client {
	return &Client{Kind: kind, BaseURL: strings.TrimRight(base, "/"), HTTPClient: &http.Client{Timeout: 4 * time.Second}}
}
func (c *Client) Name() string { return c.Kind }
func (c *Client) GetLatest(ctx context.Context, _ string, page int) ([]indexer.TorrentItem, error) {
	return c.Search(ctx, indexer.SearchOptions{Page: page})
}
func (c *Client) Search(ctx context.Context, o indexer.SearchOptions) ([]indexer.TorrentItem, error) {
	if o.Page > 1 {
		return nil, nil
	} // Public endpoints have incompatible pagination; never repeat page one.
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("%s: invalid endpoint", c.Kind)
	}
	q := u.Query()
	switch c.Kind {
	case "thepiratebay":
		u.Path += "/q.php"
		q.Set("q", o.Query)
		q.Set("cat", "200")
	case "anidex":
		q.Set("q", o.Query)
	case "ext":
		u.Path += "/search/"
		q.Set("q", o.Query)
	case "magnetdl":
		term := strings.Join(strings.Fields(strings.ToLower(o.Query)), "-")
		if term == "" {
			return nil, nil
		}
		u.Path += "/" + term[:1] + "/" + term + "/"
	default:
		return nil, fmt.Errorf("unknown public provider")
	}
	u.RawQuery = q.Encode()
	body, err := c.fetch(ctx, u.String())
	if err != nil {
		return nil, err
	}
	if c.Kind == "thepiratebay" {
		return ParseBay(body)
	}
	items, details, err := ParseHTML(body, u.String(), c.Kind)
	if err != nil {
		return nil, err
	}
	// EXT and AniDex may put their magnet only on a release detail page.
	for _, detail := range details[:min(5, len(details))] {
		if ctx.Err() != nil {
			break
		}
		b, e := c.fetch(ctx, detail)
		if e != nil {
			continue
		}
		found, _, e := ParseHTML(b, detail, c.Kind)
		if e == nil {
			items = append(items, found...)
		}
	}
	if len(items) == 0 && (strings.Contains(strings.ToLower(string(body)), "captcha") || strings.Contains(strings.ToLower(string(body)), "cloudflare")) {
		return nil, fmt.Errorf("%s: provider challenge", c.Kind)
	}
	return items, nil
}
func (c *Client) fetch(ctx context.Context, address string) ([]byte, error) {
	req, e := http.NewRequestWithContext(ctx, "GET", address, nil)
	if e != nil {
		return nil, fmt.Errorf("%s: invalid request", c.Kind)
	}
	req.Header.Set("User-Agent", "Gazes/1.0")
	req.Header.Set("Accept", "text/html, application/json")
	res, e := c.HTTPClient.Do(req)
	if e != nil {
		return nil, fmt.Errorf("%s: unavailable", c.Kind)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("%s: HTTP %d", c.Kind, res.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(res.Body, (2<<20)+1))
	if e != nil || len(b) > 2<<20 {
		return nil, fmt.Errorf("%s: invalid response", c.Kind)
	}
	return b, nil
}
func ParseBay(b []byte) ([]indexer.TorrentItem, error) {
	var entries []struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Hash     string `json:"info_hash"`
		Seeders  string `json:"seeders"`
		Leechers string `json:"leechers"`
		Size     string `json:"size"`
		Added    string `json:"added"`
	}
	if e := json.Unmarshal(b, &entries); e != nil {
		return nil, fmt.Errorf("thepiratebay: invalid JSON")
	}
	out := []indexer.TorrentItem{}
	for _, e := range entries {
		h := validHash(e.Hash)
		if h == "" || h == strings.Repeat("0", 40) {
			continue
		}
		s, _ := strconv.Atoi(e.Seeders)
		l, _ := strconv.Atoi(e.Leechers)
		size, _ := strconv.ParseInt(e.Size, 10, 64)
		added, _ := strconv.ParseInt(e.Added, 10, 64)
		item := makeItem("thepiratebay", h, e.Name, "")
		item.Seeders = max(0, s)
		item.Leechers = max(0, l)
		item.SizeBytes = size
		item.PublishDate = time.Unix(added, 0)
		out = append(out, item)
	}
	return out, nil
}
func validHash(s string) string {
	s = strings.ToLower(s)
	if len(s) != 40 {
		return ""
	}
	for _, c := range s {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return ""
		}
	}
	return s
}
func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
func content(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	if n.Data == "script" || n.Data == "style" {
		return ""
	}
	s := ""
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		s += " " + content(c)
	}
	return strings.Join(strings.Fields(s), " ")
}
func walk(n *html.Node, f func(*html.Node)) {
	f(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c, f)
	}
}
func makeItem(provider, h, title, magnet string) indexer.TorrentItem {
	if magnet == "" {
		q := url.Values{"xt": {"urn:btih:" + h}, "dn": {title}}
		magnet = "magnet:?" + q.Encode()
	}
	return indexer.TorrentItem{ID: provider + ":" + h, Provider: provider, InfoHash: h, Title: title, MagnetURI: magnet}
}
func ParseHTML(b []byte, address, provider string) ([]indexer.TorrentItem, []string, error) {
	root, e := html.Parse(strings.NewReader(string(b)))
	if e != nil {
		return nil, nil, e
	}
	base, e := url.Parse(address)
	if e != nil {
		return nil, nil, e
	}
	out := []indexer.TorrentItem{}
	details := []string{}
	seen := map[string]bool{}
	heading := ""
	walk(root, func(n *html.Node) {
		if n.Data == "h1" || n.Data == "h3" {
			if t := content(n); t != "" {
				heading = t
			}
		}
	})
	walk(root, func(n *html.Node) {
		if n.Data != "a" {
			return
		}
		href := attr(n, "href")
		u, e := url.Parse(href)
		if e != nil {
			return
		}
		if u.Scheme == "magnet" {
			h := ""
			for _, xt := range u.Query()["xt"] {
				if strings.HasPrefix(strings.ToLower(xt), "urn:btih:") {
					h = validHash(xt[9:])
				}
			}
			if h == "" || seen[h] {
				return
			}
			title := u.Query().Get("dn")
			row := n
			for row.Parent != nil && row.Data != "tr" {
				row = row.Parent
			}
			if row.Data == "tr" {
				walk(row, func(a *html.Node) {
					if a.Data == "a" && !strings.HasPrefix(attr(a, "href"), "magnet:") {
						t := content(a)
						if len(t) > len(title) {
							title = t
						}
					}
				})
			}
			if title == "" {
				title = heading
			}
			if title == "" {
				return
			}
			seen[h] = true
			item := makeItem(provider, h, title, href)
			if row.Data == "tr" {
				walk(row, func(cell *html.Node) {
					cl := strings.ToLower(attr(cell, "class"))
					value, _ := strconv.Atoi(content(cell))
					if strings.Contains(cl, "seed") || cl == "s" {
						item.Seeders = max(0, value)
					}
					if strings.Contains(cl, "leech") || cl == "l" {
						item.Leechers = max(0, value)
					}
				})
			}
			out = append(out, item)
			return
		}
		resolved := base.ResolveReference(u)
		if resolved.Host != base.Host || resolved.Scheme != base.Scheme {
			return
		}
		detail := (provider == "anidex" && strings.Contains(u.Path, "/torrent/")) || (provider == "ext" && strings.Contains(attr(n, "class"), "torrent-name"))
		if detail && !seen[resolved.String()] {
			seen[resolved.String()] = true
			details = append(details, resolved.String())
		}
	})
	return out, details, nil
}
