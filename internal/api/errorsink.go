package api

import (
	"context"
	"errors"

	"github.com/gazes/gazes/internal/admin"
	"github.com/gazes/gazes/internal/diagnostics"
	"github.com/gazes/gazes/internal/indexer"
	"github.com/gazes/gazes/internal/playback"
)

// WithErrorSink records playback errors (admin panel). Without it nothing is recorded.
func WithErrorSink(sink admin.ErrorSink) Option { return func(s *Server) { s.errorSink = sink } }

// recordPlaybackError hands e to the sink if any; it never blocks, fails or panics.
func (s *Server) recordPlaybackError(ctx context.Context, e admin.PlaybackError) {
	admin.RecordPlaybackError(ctx, s.errorSink, e)
}

// recordSegmentError classifies a failed HLS segment: a deadline is a stream timeout, an
// ffmpeg failure is a remux failure. Anything else (missing segment, window miss) is ambiguous
// and not recorded.
func (s *Server) recordSegmentError(ctx context.Context, err error) {
	var re *playback.RemuxError
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		s.recordPlaybackError(ctx, admin.PlaybackError{Code: diagnostics.StreamTimeout, Message: err.Error()})
	case errors.As(err, &re) && re.Timeout:
		s.recordPlaybackError(ctx, admin.PlaybackError{Code: diagnostics.StreamTimeout, Message: err.Error()})
	case errors.As(err, &re):
		s.recordPlaybackError(ctx, admin.PlaybackError{Code: diagnostics.RemuxFailed, Message: err.Error()})
	}
}

// recordSourceError records a failed episode source resolution. Resolver outcomes that are
// not a source failure (no mapping, nothing approved) are not recorded.
func (s *Server) recordSourceError(ctx context.Context, seasonID, ep int, err error) {
	if s.errorSink == nil || errors.Is(err, indexer.ErrAuthorityMappingMissing) || errors.Is(err, indexer.ErrNoApprovedRelease) {
		return
	}
	code := diagnostics.SourceDead
	if errors.Is(err, context.DeadlineExceeded) {
		code = diagnostics.SourceTimeout
	} else if ctx.Err() != nil {
		return // the viewer left; not a source failure
	}
	s.recordPlaybackError(ctx, admin.PlaybackError{Code: code, SeasonID: int64(seasonID), Episode: int64(ep), Message: err.Error()})
}

// wireAdminPlayback gives the admin playback/costs routes their collaborators: the active
// session count (HLS engine only), the cache diagnostics and the optional cost inputs.
func (s *Server) wireAdminPlayback() {
	if s.cfg.UsesHLS() {
		s.admin.SetPlaybackStats(activeSessions(func() int { return s.playbackManager().ActiveSessions() }))
	}
	s.admin.SetCacheDiagnostics(func(ctx context.Context) any { return s.cacheDiagnostics(ctx) })
	if s.cfg != nil {
		s.admin.SetCostInputs(admin.CostInputs{ServerMonth: s.cfg.CostServerMonth, BandwidthPerGB: s.cfg.CostBandwidthPerGB,
			StoragePerGBMonth: s.cfg.CostStoragePerGBMonth, GBPerWatchHour: s.cfg.GBPerWatchHour})
	}
}

type activeSessions func() int

func (f activeSessions) ActiveSessions() int { return f() }
