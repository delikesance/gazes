package library

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gazes/gazes/internal/stream"
	"github.com/gazes/gazes/internal/torrent"
)

type nopRSC struct{ io.ReadSeeker }

func (nopRSC) Close() error { return nil }

type fakeInner struct {
	streams, stats int
	closed         bool
}

func (f *fakeInner) AddTorrent(context.Context, string) (string, []torrent.FileInfo, error) {
	return "h", nil, nil
}
func (f *fakeInner) GetFileStream(context.Context, string, int) (io.ReadSeekCloser, *torrent.FileInfo, error) {
	f.streams++
	return nopRSC{strings.NewReader("torrent")}, &torrent.FileInfo{Length: 7}, nil
}
func (f *fakeInner) GetStats(string) (*torrent.SwarmStats, error) {
	f.stats++
	return &torrent.SwarmStats{}, nil
}
func (f *fakeInner) Close() error { f.closed = true; return nil }

func newEngineEnv(t *testing.T, state State, content string) (*Engine, *fakeInner, *Store, Entry, string, string) {
	t.Helper()
	pool, store, root := newPoolTest(t, fakeFS{"d1": {1000, 1000}}, 0, 0, "d1")
	if _, _, err := pool.Scan(); err != nil {
		t.Fatal(err)
	}
	disks := pool.Disks()
	if len(disks) != 1 {
		t.Fatalf("disks: %v", disks)
	}
	rel := "1/2-vf.mkv"
	full := filepath.Join(root, "d1", rel)
	if content != "" {
		os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	e := entry(1, 2, "vf", state)
	e.DiskID, e.RelPath, e.SizeBytes = disks[0].ID, rel, int64(len(content))
	if err := store.Create(e); err != nil {
		t.Fatal(err)
	}
	id, _ := store.StreamID(e.Key)
	in := &fakeInner{}
	return NewEngine(in, store, pool, func() time.Time { return time.UnixMilli(1700000000000) }), in, store, e, id, full
}

func TestLocalCopyServedByStreamID(t *testing.T) {
	eng, in, store, e, id, _ := newEngineEnv(t, StateAV1, "0123456789")
	r, fi, err := eng.GetFileStream(context.Background(), strings.ToUpper(id), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if in.streams != 0 || fi.Length != 10 || fi.Path != "2-vf.mkv" || !fi.IsVideo || fi.MimeType != "video/x-matroska" {
		t.Fatalf("bad info %+v inner=%d", fi, in.streams)
	}
	if got, _ := store.Get(e.Key); got.LastAccessAt.UnixMilli() != 1700000000000 {
		t.Fatalf("not touched: %v", got.LastAccessAt)
	}
	if _, err := r.Seek(5, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(r)
	if string(b) != "56789" {
		t.Fatalf("got %q", b)
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Range", "bytes=2-4")
	w := httptest.NewRecorder()
	if err := stream.ServeRange(w, req, r, fi.Path, fi.Length); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusPartialContent || w.Header().Get("Content-Range") != "bytes 2-4/10" || w.Header().Get("Accept-Ranges") != "bytes" || w.Body.String() != "234" {
		t.Fatalf("code=%d headers=%v body=%q", w.Code, w.Header(), w.Body.String())
	}
	st, err := eng.GetStats(id)
	if err != nil || st.ProgressPct != 100 || st.TotalBytes != 10 || st.CompletedBytes != 10 || st.InfoHash != id {
		t.Fatalf("stats %+v %v", st, err)
	}
	if _, _, err := eng.GetFileStream(context.Background(), id, 1); err == nil || in.streams != 0 {
		t.Fatalf("file!=0 must fail without delegating: %v", err)
	}
}

func TestUnknownHashDelegatesToTorrent(t *testing.T) {
	eng, in, _, _, _, _ := newEngineEnv(t, StateAV1, "x")
	for _, h := range []string{strings.Repeat("a", 40), "short"} {
		r, _, err := eng.GetFileStream(context.Background(), h, 3)
		if err != nil {
			t.Fatal(err)
		}
		r.Close()
		if _, err := eng.GetStats(h); err != nil {
			t.Fatal(err)
		}
	}
	if in.streams != 2 || in.stats != 2 {
		t.Fatalf("inner calls streams=%d stats=%d", in.streams, in.stats)
	}
	eng.Close()
	if !in.closed {
		t.Fatal("inner not closed")
	}
}

func TestMissingFileDeletesEntryAndReturnsMiss(t *testing.T) {
	eng, _, store, e, id, full := newEngineEnv(t, StateOriginal, "data")
	os.Remove(full)
	if _, _, err := eng.GetFileStream(context.Background(), id, 0); !errors.Is(err, ErrLibraryMiss) {
		t.Fatalf("want miss, got %v", err)
	}
	if _, err := store.Get(e.Key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("entry should be deleted: %v", err)
	}
}

func TestAbsentDiskKeepsEntry(t *testing.T) {
	eng, _, store, e, id, _ := newEngineEnv(t, StateOriginal, "data")
	store.Update(e.Key, func(x *Entry) error { x.DiskID = "gone"; return nil })
	if _, _, err := eng.GetFileStream(context.Background(), id, 0); !errors.Is(err, ErrLibraryMiss) {
		t.Fatalf("want miss, got %v", err)
	}
	if _, err := store.Get(e.Key); err != nil {
		t.Fatalf("entry must be kept: %v", err)
	}
}

func TestActiveStreamsCountsDistinctFiles(t *testing.T) {
	eng, _, _, e, id, _ := newEngineEnv(t, StateAV1, "data")
	ctx := context.Background()
	var rs []io.Closer
	for i := 0; i < 3; i++ {
		r, _, err := eng.GetFileStream(ctx, id, 0)
		if err != nil {
			t.Fatal(err)
		}
		rs = append(rs, r)
	}
	h2, _, err := eng.GetFileStream(ctx, "h2", 1)
	if err != nil {
		t.Fatal(err)
	}
	rs = append(rs, h2)
	if n := eng.ActiveStreams(); n != 2 {
		t.Fatalf("want 2, got %d", n)
	}
	if !eng.InUse(e.Key) || eng.InUse(Key{9, 9, "vf"}) {
		t.Fatal("InUse wrong")
	}
	rs[0].Close()
	rs[0].Close() // double close must not decrement twice
	if n := eng.ActiveStreams(); n != 2 || !eng.InUse(e.Key) {
		t.Fatalf("after double close want 2, got %d", n)
	}
	for _, r := range rs {
		r.Close()
	}
	if eng.ActiveStreams() != 0 || eng.InUse(e.Key) {
		t.Fatal("want 0 / not in use")
	}
}

func TestDownloadingEntryIsNotServed(t *testing.T) {
	eng, in, _, _, id, _ := newEngineEnv(t, StateDownloading, "data")
	r, _, err := eng.GetFileStream(context.Background(), id, 0)
	if err != nil {
		t.Fatal(err)
	}
	r.Close()
	if in.streams != 1 {
		t.Fatalf("expected delegation, inner=%d", in.streams)
	}
}
