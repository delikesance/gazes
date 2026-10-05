package api

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/gazes/gazes/internal/admin"
	"github.com/gazes/gazes/internal/diagnostics"
	"github.com/gazes/gazes/internal/indexer"
	"github.com/gazes/gazes/internal/playback"
	"github.com/gazes/gazes/internal/stream"
)

type captureSink struct{ got []admin.PlaybackError }

func (c *captureSink) RecordPlaybackError(_ context.Context, e admin.PlaybackError) {
	c.got = append(c.got, e)
}

func TestErrorClassification(t *testing.T) {
	remux := &playback.RemuxError{Err: errors.New("exit status 1"), Stderr: "boom"}
	killed := &playback.RemuxError{Err: errors.New("signal: killed"), Timeout: true}
	tests := []struct {
		name string
		do   func(s *Server)
		want []diagnostics.ErrorCode
	}{
		{"segment ffmpeg failure", func(s *Server) { s.recordSegmentError(context.Background(), remux) }, []diagnostics.ErrorCode{diagnostics.RemuxFailed}},
		{"segment ffmpeg killed by deadline", func(s *Server) { s.recordSegmentError(context.Background(), killed) }, []diagnostics.ErrorCode{diagnostics.StreamTimeout}},
		{"segment deadline", func(s *Server) {
			s.recordSegmentError(context.Background(), fmt.Errorf("x: %w", context.DeadlineExceeded))
		}, []diagnostics.ErrorCode{diagnostics.StreamTimeout}},
		{"segment missing: ambiguous, not recorded", func(s *Server) {
			s.recordSegmentError(context.Background(), errors.New("segment outside playback window"))
		}, nil},
		{"source providers down", func(s *Server) { s.recordSourceError(context.Background(), 1, 2, errors.New("all failed")) }, []diagnostics.ErrorCode{diagnostics.SourceDead}},
		{"source timeout", func(s *Server) { s.recordSourceError(context.Background(), 1, 2, context.DeadlineExceeded) }, []diagnostics.ErrorCode{diagnostics.SourceTimeout}},
		{"no mapping is not a source failure", func(s *Server) { s.recordSourceError(context.Background(), 1, 2, indexer.ErrAuthorityMappingMissing) }, nil},
		{"nothing approved is not a source failure", func(s *Server) { s.recordSourceError(context.Background(), 1, 2, indexer.ErrNoApprovedRelease) }, nil},
		{"viewer left", func(s *Server) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			s.recordSourceError(ctx, 1, 2, errors.New("canceled"))
		}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sink := &captureSink{}
			tc.do(&Server{errorSink: sink})
			if len(sink.got) != len(tc.want) {
				t.Fatalf("recorded %+v, want codes %v", sink.got, tc.want)
			}
			for i, w := range tc.want {
				if sink.got[i].Code != w {
					t.Errorf("code %s, want %s", sink.got[i].Code, w)
				}
			}
		})
	}
	// A nil sink is a no-op.
	s := &Server{}
	s.recordSegmentError(context.Background(), remux)
	s.recordSourceError(context.Background(), 1, 2, errors.New("x"))
	s.recordPlaybackError(context.Background(), admin.PlaybackError{Code: diagnostics.Unknown})

	if !errors.Is(fmt.Errorf("%w: %w, stderr: x", stream.ErrRemuxFailed, errors.New("e")), stream.ErrRemuxFailed) {
		t.Fatal("ErrRemuxFailed must be matchable")
	}
}
