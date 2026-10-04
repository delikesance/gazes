package metadata

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

type unlimitedProbeReader struct{ bytes atomic.Int64 }

func (r *unlimitedProbeReader) Read(p []byte) (int, error) {
	return r.ReadContext(context.Background(), p)
}
func (r *unlimitedProbeReader) ReadContext(ctx context.Context, p []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	clear(p)
	r.bytes.Add(int64(len(p)))
	return len(p), nil
}
func TestProbeStopsAfterHeaderInsteadOfWaitingForEntireTorrent(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "ffprobe")
	script := `#!/bin/sh
cat >/dev/null
printf '%s' '{"streams":[{"index":0,"codec_type":"video","codec_name":"h264"},{"index":1,"codec_type":"audio","codec_name":"mp3","tags":{"language":"fre"}}],"format":{"duration":"1370"}}'
`
	if err := os.WriteFile(executable, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FFPROBE_PATH", executable)
	reader := &unlimitedProbeReader{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := NewFFprobeAnalyzer(slog.New(slog.NewTextHandler(io.Discard, nil))).ProbeReader(ctx, reader, 200*1024*1024)
	if err != nil || result == nil || result.ProbeStatus != "complete" || len(result.AudioTracks) != 1 || result.AudioTracks[0].Language != "fre" {
		t.Fatalf("%+v %v", result, err)
	}
	if read := reader.bytes.Load(); read != 64*1024 {
		t.Fatalf("probe read %d bytes instead of a bounded header", read)
	}
}

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

func TestProbeRetriesLargerHeadersWithinBoundedRead(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "ffprobe")
	script := `#!/bin/sh
size=$(wc -c)
if [ "$size" -lt 100000 ]; then exit 1; fi
printf '%s' '{"streams":[{"index":0,"codec_type":"video","codec_name":"h264"},{"index":1,"codec_type":"audio","codec_name":"aac","tags":{"language":"fra"}}],"format":{"duration":"1370"}}'
`
	if err := os.WriteFile(executable, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FFPROBE_PATH", executable)
	reader := &highWaterReader{Reader: bytes.NewReader(make([]byte, 2*1000000))}
	result, err := NewFFprobeAnalyzer(slog.New(slog.NewTextHandler(io.Discard, nil))).ProbeReader(context.Background(), reader, reader.Size())
	if err != nil || result == nil || len(result.AudioTracks) != 1 || result.AudioTracks[0].Language != "fra" {
		t.Fatalf("%+v %v", result, err)
	}
	if reader.high != 1000000 {
		t.Fatalf("larger header read %d bytes", reader.high)
	}
}

// highWaterReader records the furthest offset read (the probe rewinds the reader afterwards).
type highWaterReader struct {
	*bytes.Reader
	high int64
}

func (h *highWaterReader) Read(p []byte) (int, error) {
	n, err := h.Reader.Read(p)
	if pos := h.Reader.Size() - int64(h.Reader.Len()); pos > h.high {
		h.high = pos
	}
	return n, err
}
