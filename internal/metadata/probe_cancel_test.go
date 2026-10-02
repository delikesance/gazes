package metadata

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type waitingProbeReader struct{ done chan struct{} }

func (r *waitingProbeReader) Read([]byte) (int, error) { panic("probe must use cancellable reads") }
func (r *waitingProbeReader) ReadContext(ctx context.Context, _ []byte) (int, error) {
	<-ctx.Done()
	close(r.done)
	return 0, ctx.Err()
}
func TestProbeDoesNotWaitForMissingPieceAfterProcessExit(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "ffprobe")
	script := `#!/bin/sh
printf '%s' '{"streams":[{"index":0,"codec_type":"video","codec_name":"hevc","width":1920,"height":1080}],"format":{"duration":"120"}}'
`
	if err := os.WriteFile(executable, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FFPROBE_PATH", executable)
	reader := &waitingProbeReader{done: make(chan struct{})}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	started := time.Now()
	result, err := NewFFprobeAnalyzer(slog.New(slog.NewTextHandler(io.Discard, nil))).ProbeReader(ctx, reader, 1000)
	if err != nil || result == nil || result.VideoCodec != "hevc" {
		t.Fatalf("lost valid probe result: %v %v", result, err)
	}
	if time.Since(started) > 500*time.Millisecond {
		t.Fatal("probe waited for a missing torrent piece after process exit")
	}
	select {
	case <-reader.done:
	default:
		t.Fatal("stdin pump was left reading the torrent")
	}
}
func TestProbeMissingPieceHonorsRequestTimeout(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "ffprobe")
	// exec keeps the child process cancellable without an orphaned shell/sleep.
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexec sleep 10\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FFPROBE_PATH", executable)
	reader := &waitingProbeReader{done: make(chan struct{})}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := NewFFprobeAnalyzer(slog.New(slog.NewTextHandler(io.Discard, nil))).ProbeReader(ctx, reader, 1000)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("probe ignored timeout: %v", err)
	}
	select {
	case <-reader.done:
	default:
		t.Fatal("timed out probe left a torrent read active")
	}
}
