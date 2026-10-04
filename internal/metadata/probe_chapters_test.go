package metadata

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

var quietLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestAttachChaptersFillsSegments(t *testing.T) {
	file := mkv(ebmlElement(idChapters, edition(nil,
		atom(0, 90, "Opening"),
		atom(90, 1330, "Part A"),
		atom(1330, 1420, "Ending"),
	)))
	r := bytes.NewReader(file)
	r.Seek(5, io.SeekStart) // the probe leaves the reader mid-file
	meta := &VideoMetadata{DurationSec: 1420, TotalBytes: int64(len(file))}
	attachChapters(context.Background(), quietLogger, r, meta)
	if len(meta.Chapters) != 3 {
		t.Fatalf("chapters = %+v", meta.Chapters)
	}
	kinds := map[string]bool{}
	for _, s := range meta.SkipSegments {
		kinds[s.Kind] = true
	}
	if len(meta.SkipSegments) != 2 || !kinds["opening"] || !kinds["ending"] {
		t.Fatalf("segments = %+v", meta.SkipSegments)
	}
	if pos, _ := r.Seek(0, io.SeekCurrent); pos != 0 {
		t.Fatalf("reader left at %d, want 0", pos)
	}
}

func TestAttachChaptersNonSeekable(t *testing.T) {
	file := mkv(ebmlElement(idChapters, edition(nil, atom(0, 90, "Opening"))))
	meta := &VideoMetadata{DurationSec: 1420, TotalBytes: int64(len(file))}
	attachChapters(context.Background(), quietLogger, io.MultiReader(bytes.NewReader(file)), meta)
	if meta.Chapters != nil || meta.SkipSegments != nil {
		t.Fatalf("meta changed: %+v", meta)
	}
}

// stallReader blocks every contextual read at or past stallFrom until ctx ends.
type stallReader struct {
	*bytes.Reader
	stallFrom int64
}

func (s stallReader) ReadContext(ctx context.Context, p []byte) (int, error) {
	if pos, _ := s.Reader.Seek(0, io.SeekCurrent); pos >= s.stallFrom {
		<-ctx.Done()
		return 0, ctx.Err()
	}
	return s.Reader.Read(p)
}

func TestAttachChaptersGivesUpOnStall(t *testing.T) {
	old := chapterReadTimeout
	chapterReadTimeout = 50 * time.Millisecond
	defer func() { chapterReadTimeout = old }()

	info := ebmlElement(idInfo, []byte("info"))
	cluster := ebmlElement(idCluster, bytes.Repeat([]byte{7}, 4096))
	chapters := ebmlElement(idChapters, edition(nil, atom(0, 90, "Opening")))
	seekHead := func(position uint64) []byte {
		return ebmlElement(idSeekHead, ebmlElement(idSeek, cat(
			ebmlElement(idSeekID, chaptersIDAsBytes),
			ebmlUint(idSeekPosition, position),
		)))
	}
	position := uint64(len(seekHead(0)) + len(info) + len(cluster))
	file := cat(ebmlElement(idEBML, []byte("webm")), ebmlElement(idSegment, cat(seekHead(position), info, cluster, chapters)))
	stallFrom := int64(len(file) - len(chapters))

	meta := &VideoMetadata{DurationSec: 1420, TotalBytes: int64(len(file))}
	done := make(chan struct{})
	go func() {
		defer close(done)
		attachChapters(context.Background(), quietLogger, stallReader{bytes.NewReader(file), stallFrom}, meta)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("attachChapters did not give up on a stalled read")
	}
	if meta.Chapters != nil || meta.SkipSegments != nil {
		t.Fatalf("meta has chapters: %+v", meta)
	}
}

func TestAttachChaptersCorrupt(t *testing.T) {
	file := mkv(ebmlElement(idChapters, edition(nil, atom(0, 90, "Opening"))))
	file = file[:len(file)-10]
	meta := &VideoMetadata{DurationSec: 1420, TotalBytes: int64(len(file))}
	attachChapters(context.Background(), quietLogger, bytes.NewReader(file), meta)
	if meta.Chapters != nil || meta.SkipSegments != nil {
		t.Fatalf("meta has chapters: %+v", meta)
	}
}
