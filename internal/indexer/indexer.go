package indexer

import (
	"context"
	"time"

	"github.com/gazes/gazes/internal/metadata"
)

// TorrentItem represents a discovered torrent from an indexer (e.g., Nyaa.si).
type TorrentItem struct {
	Provider     string                  `json:"provider,omitempty"`
	ID           string                  `json:"id"`
	Title        string                  `json:"title"`
	InfoHash     string                  `json:"info_hash"`
	MagnetURI    string                  `json:"magnet_uri"`
	TorrentURL   string                  `json:"torrent_url,omitempty"`
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
}

// Provider defines the interface for torrent indexers.
type Provider interface {
	Name() string
	Search(ctx context.Context, opts SearchOptions) ([]TorrentItem, error)
	GetLatest(ctx context.Context, category string, page int) ([]TorrentItem, error)
}
