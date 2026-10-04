package library

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gazes/gazes/internal/torrent"
)

type nopSeekCloser struct{ *bytes.Reader }

func (nopSeekCloser) Close() error { return nil }

type fakeFetcher struct {
	mu          sync.Mutex
	data        []byte
	completed   int64
	verifyErr   error
	progErr     error
	downloadErr error
	downloads   int
	released    int
}

func (f *fakeFetcher) GetFileStream(ctx context.Context, ih string, idx int) (io.ReadSeekCloser, *torrent.FileInfo, error) {
	return nopSeekCloser{bytes.NewReader(f.data)}, &torrent.FileInfo{Index: idx, Length: int64(len(f.data))}, nil
}
func (f *fakeFetcher) DownloadFile(ih string, idx int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.downloads++
	return f.downloadErr
}
func (f *fakeFetcher) ReleaseFile(ih string, idx int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.released++
}
func (f *fakeFetcher) FileProgress(ih string, idx int) (int64, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.completed, int64(len(f.data)), f.progErr
}
func (f *fakeFetcher) VerifyFile(ctx context.Context, ih string, idx int) error { return f.verifyErr }

type fakeProber struct{}

func (fakeProber) Probe(ctx context.Context, path string) (MediaInfo, error) {
	return MediaInfo{DurationMS: 1420000, VideoCodec: "h264", VideoStreams: 1, AudioCodecs: []string{"aac", "opus"}, AudioChannels: []int{2, 2}, SubtitleTracks: 3}, nil
}

type testClock struct{ now time.Time }

func (c *testClock) Now() time.Time { return c.now }

type acqEnv struct {
	a     *Acquirer
	store *Store
	pool  *Pool
	root  string
	f     *fakeFetcher
	clk   *testClock
}

func newAcqEnv(t *testing.T, maxActive int, free int64) *acqEnv {
	t.Helper()
	pool, store, root := newPoolTest(t, fakeFS{"d1": {1 << 40, free}}, 0, 0, "d1")
	if _, _, err := pool.Scan(); err != nil {
		t.Fatal(err)
	}
	f := &fakeFetcher{data: []byte("hello episode bytes, plenty of them")}
	clk := &testClock{now: time.Unix(1_700_000_000, 0)}
	a := NewAcquirer(store, pool, f, fakeProber{}, clk.Now, 24*time.Hour, maxActive)
	return &acqEnv{a, store, pool, root, f, clk}
}

func req(ep int) Request {
	return Request{Key: Key{SeasonID: 7, Episode: ep, Lang: "vostfr"}, AnimeID: 1, Title: "T", InfoHash: "abc", FileIndex: 2, ReleaseName: "rel"}
}

func (e *acqEnv) reserved() int64 {
	e.pool.mu.Lock()
	defer e.pool.mu.Unlock()
	var n int64
	for _, v := range e.pool.reserved {
		n += v
	}
	return n
}

func TestStartCreatesDownloadingAndPins(t *testing.T) {
	env := newAcqEnv(t, 4, 1<<30)
	e, created, err := env.a.Start(req(1))
	if err != nil || !created {
		t.Fatalf("Start = %v, %v", created, err)
	}
	if e.State != StateDownloading || env.f.downloads != 1 {
		t.Fatalf("state %s downloads %d", e.State, env.f.downloads)
	}
	if env.reserved() != int64(len(env.f.data)) {
		t.Fatalf("reserved %d", env.reserved())
	}
	got, _ := env.store.Get(req(1).Key)
	if got.State != StateDownloading || got.ReservedBytes != int64(len(env.f.data)) {
		t.Fatalf("stored %+v", got)
	}
}

func TestStartExistingCopyOnlyTouches(t *testing.T) {
	env := newAcqEnv(t, 4, 1<<30)
	if _, _, err := env.a.Start(req(1)); err != nil {
		t.Fatal(err)
	}
	env.clk.now = env.clk.now.Add(time.Hour)
	_, created, err := env.a.Start(req(1))
	if err != nil || created {
		t.Fatalf("second Start = %v, %v", created, err)
	}
	if env.f.downloads != 1 {
		t.Fatalf("downloads %d", env.f.downloads)
	}
	got, _ := env.store.Get(req(1).Key)
	if !got.LastAccessAt.Equal(time.UnixMilli(env.clk.now.UnixMilli())) {
		t.Fatalf("LastAccessAt %v", got.LastAccessAt)
	}
}

func TestStartUnavailableCopyIsNotDuplicated(t *testing.T) {
	env := newAcqEnv(t, 4, 1<<30)
	env.a.Start(req(1))
	env.store.Update(req(1).Key, func(e *Entry) error { e.PrevState, e.State = e.State, StateUnavailable; return nil })
	_, created, err := env.a.Start(req(1))
	if err != nil || created || env.f.downloads != 1 {
		t.Fatalf("created=%v err=%v downloads=%d", created, err, env.f.downloads)
	}
}

func TestCompletedFileIsVerifiedCopiedAndProbed(t *testing.T) {
	env := newAcqEnv(t, 4, 1<<30)
	env.a.Start(req(1))
	env.f.completed = int64(len(env.f.data))
	env.a.step(context.Background())

	e, _ := env.store.Get(req(1).Key)
	if e.State != StateOriginal || e.RelPath != "7/1-vostfr.mkv" {
		t.Fatalf("entry %+v", e)
	}
	sum := sha256.Sum256(env.f.data)
	if e.SHA256 != hex.EncodeToString(sum[:]) || e.ReservedBytes != 0 || e.AudioTracks != 2 || e.SubtitleTracks != 3 || e.DurationMS != 1420000 || e.VideoCodec != "h264" {
		t.Fatalf("entry %+v", e)
	}
	p, err := env.pool.Path(e.DiskID, e.RelPath)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(p)
	if err != nil || !bytes.Equal(got, env.f.data) {
		t.Fatalf("copy mismatch: %v", err)
	}
	if _, err := os.Stat(p + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("tmp left: %v", err)
	}
	if env.reserved() != 0 || env.f.released != 1 {
		t.Fatalf("reserved %d released %d", env.reserved(), env.f.released)
	}
}

func TestVerifyFailureKeepsDownloading(t *testing.T) {
	env := newAcqEnv(t, 4, 1<<30)
	env.a.Start(req(1))
	env.f.completed = int64(len(env.f.data))
	env.f.verifyErr = errors.New("bad piece")
	env.a.step(context.Background())
	e, _ := env.store.Get(req(1).Key)
	if e.State != StateDownloading || e.RelPath != "" {
		t.Fatalf("entry %+v", e)
	}
	if e.LastError == "" || env.reserved() != int64(len(env.f.data)) || env.f.released != 0 {
		t.Fatalf("LastError %q reserved %d released %d", e.LastError, env.reserved(), env.f.released)
	}
	if _, err := os.Stat(filepath.Join(env.root, "d1", "7", "1-vostfr.mkv")); !os.IsNotExist(err) {
		t.Fatalf("final file exists: %v", err)
	}
}

func TestStallAbandonsAfter24h(t *testing.T) {
	env := newAcqEnv(t, 4, 1<<30)
	env.a.Start(req(1))
	env.f.completed = 5
	env.a.step(context.Background())
	env.clk.now = env.clk.now.Add(23 * time.Hour)
	env.a.step(context.Background())
	if _, err := env.store.Get(req(1).Key); err != nil {
		t.Fatalf("abandoned too early: %v", err)
	}
	env.clk.now = env.clk.now.Add(time.Hour + time.Minute)
	env.a.step(context.Background())
	if _, err := env.store.Get(req(1).Key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("entry still there: %v", err)
	}
	if env.f.released != 1 || env.reserved() != 0 {
		t.Fatalf("released %d reserved %d", env.f.released, env.reserved())
	}
}

func TestProgressErrorCountsAsNoProgress(t *testing.T) {
	env := newAcqEnv(t, 4, 1<<30)
	env.a.Start(req(1))
	env.f.progErr = errors.New("dropped")
	env.a.step(context.Background())
	if _, err := env.store.Get(req(1).Key); err != nil {
		t.Fatalf("deleted immediately: %v", err)
	}
	env.clk.now = env.clk.now.Add(25 * time.Hour)
	env.a.step(context.Background())
	if _, err := env.store.Get(req(1).Key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("not abandoned: %v", err)
	}
}

func TestMaxActiveDownloadsReturnsBusy(t *testing.T) {
	env := newAcqEnv(t, 4, 1<<30)
	for i := 1; i <= 4; i++ {
		if _, _, err := env.a.Start(req(i)); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := env.a.Start(req(5)); !errors.Is(err, ErrBusy) {
		t.Fatalf("err = %v", err)
	}
}

func TestNoSpaceDoesNotCreateEntry(t *testing.T) {
	env := newAcqEnv(t, 4, 3) // free space smaller than the file
	_, _, err := env.a.Start(req(1))
	if !errors.Is(err, ErrNoSpace) {
		t.Fatalf("err = %v", err)
	}
	if _, err := env.store.Get(req(1).Key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("entry created: %v", err)
	}
	if env.f.downloads != 0 {
		t.Fatalf("downloads %d", env.f.downloads)
	}
}

func TestResumeRepinsDownloading(t *testing.T) {
	env := newAcqEnv(t, 4, 1<<30)
	env.a.Start(req(1))
	b := NewAcquirer(env.store, env.pool, env.f, fakeProber{}, env.clk.Now, time.Hour, 4)
	if err := b.Resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	if env.f.downloads != 2 {
		t.Fatalf("downloads %d", env.f.downloads)
	}
	if env.reserved() != 2*int64(len(env.f.data)) {
		t.Fatalf("reserved %d", env.reserved())
	}
}

func TestResumeDeletesEntryWhenDownloadFails(t *testing.T) {
	env := newAcqEnv(t, 4, 1<<30)
	env.a.Start(req(1))
	env.f.downloadErr = errors.New("unknown torrent")
	b := NewAcquirer(env.store, env.pool, env.f, fakeProber{}, env.clk.Now, time.Hour, 4)
	if err := b.Resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := env.store.Get(req(1).Key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("entry still there: %v", err)
	}
	if env.f.released != 1 {
		t.Fatalf("released %d", env.f.released)
	}
}

func TestStartDownloadFailureReleasesEverything(t *testing.T) {
	env := newAcqEnv(t, 4, 1<<30)
	env.f.downloadErr = errors.New("no metadata")
	if _, _, err := env.a.Start(req(1)); err == nil {
		t.Fatal("expected error")
	}
	if env.f.released != 1 || env.reserved() != 0 {
		t.Fatalf("released %d reserved %d", env.f.released, env.reserved())
	}
	if _, err := env.store.Get(req(1).Key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("entry created: %v", err)
	}
}

func TestParseProbe(t *testing.T) {
	const doc = `{"streams":[{"codec_type":"video","codec_name":"hevc"},{"codec_type":"audio","codec_name":"aac","channels":6},{"codec_type":"audio","codec_name":"flac","channels":2},{"codec_type":"subtitle","codec_name":"ass"},{"codec_type":"attachment","codec_name":"ttf"}],"format":{"duration":"1421.504000"}}`
	mi, err := parseProbe([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if mi.DurationMS != 1421504 || mi.VideoCodec != "hevc" || len(mi.AudioCodecs) != 2 || mi.AudioChannels[0] != 6 || mi.SubtitleTracks != 1 || mi.AttachmentTracks != 1 {
		t.Fatalf("%+v", mi)
	}
}
