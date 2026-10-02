package torrent

import (
	"context"
	"io"
)

// FileInfo represents a file within a torrent.
type FileInfo struct {
	Index    int    `json:"index"`
	Path     string `json:"path"`
	Length   int64  `json:"length"`
	IsVideo  bool   `json:"is_video"`
	MimeType string `json:"mime_type"`
}

// SwarmStats provides real-time telemetry of a torrent.
type SwarmStats struct {
	InfoHash       string  `json:"info_hash"`
	Title          string  `json:"title"`
	TotalBytes     int64   `json:"total_bytes"`
	CompletedBytes int64   `json:"completed_bytes"`
	ProgressPct    float64 `json:"progress_pct"`
	DownloadRate   int64   `json:"download_rate_bps"`
	UploadRate     int64   `json:"upload_rate_bps"`
	TotalPeers     int     `json:"total_peers"`
	ActiveSeeders  int     `json:"active_seeders"`
}

// Engine defines the interface for the torrent streaming engine.
type Engine interface {
	// AddTorrent adds a magnet URI or torrent link and returns its file list once metadata is ready.
	AddTorrent(ctx context.Context, magnetURI string) (string, []FileInfo, error)

	// GetFileStream returns a seekable reader prioritized for sequential streaming.
	GetFileStream(ctx context.Context, infoHash string, fileIndex int) (io.ReadSeekCloser, *FileInfo, error)

	// GetStats retrieves real-time swarm telemetry for an active stream.
	GetStats(infoHash string) (*SwarmStats, error)

	// Close terminates the engine and closes all active swarms.
	Close() error
}
