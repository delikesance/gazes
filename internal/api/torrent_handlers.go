package api

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gazes/gazes/internal/diagnostics"
	"net/http"
	"strconv"
	"time"

	"github.com/gazes/gazes/internal/metadata"
	"github.com/gazes/gazes/internal/torrent"
)

type LoadTorrentRequest struct {
	MagnetURI    string `json:"magnet"`
	MetadataOnly bool   `json:"metadata_only,omitempty"`
}

type LoadTorrentResponse struct {
	InfoHash          string                  `json:"info_hash"`
	Files             []torrent.FileInfo      `json:"files"`
	MainVideoIndex    int                     `json:"main_video_index"`
	MainVideoMetadata *metadata.VideoMetadata `json:"main_video_metadata,omitempty"`
}

// HandleLoadTorrent loads metadata for a given magnet URI and returns its file hierarchy.
func (s *Server) HandleLoadTorrent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error": "method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req LoadTorrentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.MagnetURI == "" {
		http.Error(w, `{"error": "invalid request body: 'magnet' is required"}`, http.StatusBadRequest)
		return
	}

	// Timeout context for metadata resolution
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	start := time.Now()
	diagnostics.Logger(r.Context(), s.logger).Info("torrent.metadata_started")
	infoHash, files, err := s.torrentEngine.AddTorrent(ctx, req.MagnetURI)
	if err != nil {
		diagnostics.Logger(r.Context(), s.logger).Error("failed to load torrent metadata", "err", err, "magnet", req.MagnetURI)
		http.Error(w, `{"error": "failed to resolve torrent metadata"}`, http.StatusGatewayTimeout)
		return
	}

	diagnostics.Logger(r.Context(), s.logger).Info("torrent.metadata_completed", "infohash", infoHash, "file_count", len(files), "duration_ms", time.Since(start).Milliseconds())
	mainVideoIdx := -1
	if mainVideo := torrent.FindMainVideoFile(files); mainVideo != nil {
		mainVideoIdx = mainVideo.Index
	}

	// Fast metadata probing on pre-fetched container headers
	var mainVideoMeta *metadata.VideoMetadata
	if mainVideoIdx >= 0 && !req.MetadataOnly {
		probeCtx, probeCancel := context.WithTimeout(r.Context(), 2*time.Second)
		if reader, fileInfo, err := s.torrentEngine.GetFileStream(probeCtx, infoHash, mainVideoIdx); err == nil {
			if meta, err := s.analyzer.ProbeReader(probeCtx, reader, fileInfo.Length); err == nil && meta != nil {
				mainVideoMeta = meta
			}
			_ = reader.Close()
		}
		probeCancel()
	}

	if mainVideoMeta == nil && mainVideoIdx >= 0 {
		mainVideoMeta = &metadata.VideoMetadata{
			TotalBytes: files[mainVideoIdx].Length,
		}
	}

	resp := LoadTorrentResponse{
		InfoHash:          infoHash,
		Files:             files,
		MainVideoIndex:    mainVideoIdx,
		MainVideoMetadata: mainVideoMeta,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// HandleTorrentStats returns real-time swarm telemetry for a loaded torrent.
func (s *Server) HandleTorrentStats(w http.ResponseWriter, r *http.Request) {
	ih := r.URL.Query().Get("ih")
	if ih == "" {
		http.Error(w, `{"error": "missing infohash parameter 'ih'"}`, http.StatusBadRequest)
		return
	}

	stats, err := s.torrentEngine.GetStats(ih)
	if err != nil {
		http.Error(w, `{"error": "torrent not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	diagnostics.Logger(r.Context(), s.logger).Debug("torrent.swarm", "infohash", ih, "stats", stats)
	_ = json.NewEncoder(w).Encode(stats)
}

// HandleMetadata probes and returns video duration, dimensions, and codecs for a specific file.
func (s *Server) HandleMetadata(w http.ResponseWriter, r *http.Request) {
	ih := r.URL.Query().Get("ih")
	if ih == "" {
		http.Error(w, `{"error": "missing infohash parameter 'ih'"}`, http.StatusBadRequest)
		return
	}

	fileIdx := 0
	if idxStr := r.URL.Query().Get("file_idx"); idxStr != "" {
		if idx, err := strconv.Atoi(idxStr); err == nil && idx >= 0 {
			fileIdx = idx
		}
	}

	probeCtx, cancel := contextWithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	reader, fileInfo, err := s.torrentEngine.GetFileStream(probeCtx, ih, fileIdx)
	if err != nil {
		http.Error(w, `{"error": "failed to access torrent stream for metadata"}`, http.StatusNotFound)
		return
	}
	defer reader.Close()

	meta, err := s.analyzer.ProbeReader(probeCtx, reader, fileInfo.Length)
	if err != nil || meta == nil {
		meta = &metadata.VideoMetadata{
			TotalBytes:     fileInfo.Length,
			ProbeStatus:    "failed",
			ProbeErrorCode: "probe_failed",
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || probeCtx.Err() != nil {
			meta.ProbeStatus = "timeout"
			meta.ProbeErrorCode = "probe_timeout"
		}
	} else {
		meta.ProbeStatus = "complete"
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(meta)
}
