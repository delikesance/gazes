package indexer

import (
	"context"
	"net/url"
	"time"

	"github.com/gazes/gazes/internal/metadata"
)

// TorrentItem represents a discovered torrent from an indexer (e.g., Nyaa.si).
type TorrentItem struct {
	Provider   string `json:"provider,omitempty"`
	ID         string `json:"id"`
	Title      string `json:"title"`
	InfoHash   string `json:"info_hash"`
	MagnetURI  string `json:"magnet_uri"`
	TorrentURL string `json:"torrent_url,omitempty"`
	// IndexerOnly marks a result from a private tracker whose ratio playback consumes.
	// The resolver keeps it only as a VF fallback.
	IndexerOnly  bool                    `json:"indexer_only,omitempty"`
	SizeBytes    int64                   `json:"size_bytes"`
	SizeDisplay  string                  `json:"size_display"`
	Seeders      int                     `json:"seeders"`
	Leechers     int                     `json:"leechers"`
	Downloads    int                     `json:"downloads"`
	Category     string                  `json:"category"`
	PublishDate  time.Time               `json:"publish_date"`
	AnimeDetails *metadata.AnimeMetadata `json:"anime_details,omitempty"`
}

// SearchOptions defines query parameters for indexer searches.
type SearchOptions struct {
	Query    string
	Category string
	Page     int
	SortBy   string
	Order    string
	// Scope restricts which providers MultiProvider queries; the zero value asks them all.
	Scope ProviderScope
}

// ProviderScope selects the providers a MultiProvider search is fanned out to. Rate-paced providers
// (see PacedProvider) can only serve a few requests per resolve, so the resolver sends them a small
// prioritized subset (ScopePaced) and keeps every other query away from them (ScopeUnpaced).
type ProviderScope int

const (
	// ScopeAll queries every provider (default).
	ScopeAll ProviderScope = iota
	// ScopeUnpaced skips rate-paced providers.
	ScopeUnpaced
	// ScopePaced queries only rate-paced providers.
	ScopePaced
)

// Provider defines the interface for torrent indexers.
type Provider interface {
	Name() string
	Search(ctx context.Context, opts SearchOptions) ([]TorrentItem, error)
	GetLatest(ctx context.Context, category string, page int) ([]TorrentItem, error)
}

// PrivateSourcePrefix marks a magnet's "xs" with the private provider whose .torrent
// the backend must fetch, e.g. "gazes:c411". It never carries credentials: the
// .torrent's announce URL holds the passkey, so only the backend downloads it.
const PrivateSourcePrefix = "gazes:"

// PrivateMagnet is the magnet of an indexer-only result: no tracker, only the
// provider the backend fetches the .torrent from.
func PrivateMagnet(item TorrentItem) string {
	q := url.Values{"xt": {"urn:btih:" + item.InfoHash}, "dn": {item.Title}, "xs": {PrivateSourcePrefix + item.Provider}}
	return "magnet:?" + q.Encode()
}
