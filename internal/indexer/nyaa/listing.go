package nyaa

import (
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gazes/gazes/internal/indexer"
	"golang.org/x/net/html"
)

func attr(node *html.Node, key string) string {
	for _, a := range node.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func nodeText(node *html.Node) string {
	if node.Type == html.TextNode {
		return node.Data
	}
	var b strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		b.WriteString(nodeText(child))
	}
	return strings.TrimSpace(b.String())
}

// parseListing reads only the search table. No per-release HTTP requests are needed.
func (c *Client) parseListing(reader io.Reader) ([]indexer.TorrentItem, error) {
	doc, err := html.Parse(reader)
	if err != nil {
		return nil, fmt.Errorf("decode nyaa listing: %w", err)
	}
	items := []indexer.TorrentItem{}
	found := false
	empty := false
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "h3" && nodeText(n) == "No results found" {
			empty = true
		}
		if n.Type == html.ElementNode && n.Data == "table" && strings.Contains(" "+attr(n, "class")+" ", " torrent-list ") {
			found = true
			var rows func(*html.Node)
			rows = func(row *html.Node) {
				if row.Type == html.ElementNode && row.Data == "tr" {
					cells := []*html.Node{}
					for cell := row.FirstChild; cell != nil; cell = cell.NextSibling {
						if cell.Type == html.ElementNode && cell.Data == "td" {
							cells = append(cells, cell)
						}
					}
					if len(cells) != 8 {
						return
					}
					item := NyaaItem{Size: nodeText(cells[3]), Seeders: numberCell(cells[5]), Leechers: numberCell(cells[6]), Downloads: numberCell(cells[7])}
					var links func(*html.Node)
					links = func(link *html.Node) {
						if link.Type == html.ElementNode && link.Data == "a" {
							href := attr(link, "href")
							if strings.HasPrefix(href, "/view/") && !strings.Contains(href, "#") {
								item.Link = href
								item.Title = nodeText(link)
							}
							if strings.HasPrefix(href, "/?c=") {
								item.Category = attr(link, "title")
							}
							if strings.HasPrefix(href, "magnet:") {
								if u, e := url.Parse(href); e == nil {
									item.InfoHash = strings.TrimPrefix(u.Query().Get("xt"), "urn:btih:")
								}
							}
						}
						for child := link.FirstChild; child != nil; child = child.NextSibling {
							links(child)
						}
					}
					links(row)
					decoded, e := hex.DecodeString(item.InfoHash)
					if item.Title == "" || item.Link == "" || e != nil || len(decoded) != 20 {
						return
					}
					mapped := c.mapItem(item)
					if timestamp, e := strconv.ParseInt(attr(cells[4], "data-timestamp"), 10, 64); e == nil {
						mapped.PublishDate = time.Unix(timestamp, 0).UTC()
					}
					items = append(items, mapped)
					return
				}
				for child := row.FirstChild; child != nil; child = child.NextSibling {
					rows(child)
				}
			}
			rows(n)
			return
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	if !found && !empty {
		return nil, fmt.Errorf("nyaa listing missing (provider challenge or unexpected response)")
	}
	return items, nil
}

func numberCell(n *html.Node) int {
	v, _ := strconv.Atoi(strings.ReplaceAll(nodeText(n), ",", ""))
	return v
}
