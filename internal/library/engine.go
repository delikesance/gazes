package library

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gazes/gazes/internal/torrent"
)

// ErrLibraryMiss means the index referenced a copy whose file could not be opened.
var ErrLibraryMiss = errors.New("library: local copy missing")

type streamKey struct {
	hash string
	file int
}

// Engine serves local library copies by stream id and delegates everything else to the torrent engine.
type Engine struct {
	inner torrent.Engine
	store *Store
	pool  *Pool
	clock func() time.Time

	mu      sync.Mutex
	readers map[streamKey]int
}

var _ torrent.Engine = (*Engine)(nil)

func NewEngine(inner torrent.Engine, store *Store, pool *Pool, clock func() time.Time) *Engine {
	if clock == nil {
		clock = time.Now
	}
	return &Engine{inner: inner, store: store, pool: pool, clock: clock, readers: map[streamKey]int{}}
}

func isStreamID(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// lookup returns the servable library entry behind hash, if any. Errors other than "not found"
// are swallowed so torrent playback is never broken by the library.
func (e *Engine) lookup(hash string) (Entry, string, bool) {
	id := strings.ToLower(hash)
	if !isStreamID(id) {
		return Entry{}, "", false
	}
	ent, err := e.store.ByStreamID(id)
	if err != nil || (ent.State != StateOriginal && ent.State != StateAV1) {
		return Entry{}, "", false
	}
	return ent, id, true
}

func (e *Engine) AddTorrent(ctx context.Context, magnetURI string) (string, []torrent.FileInfo, error) {
	return e.inner.AddTorrent(ctx, magnetURI)
}

func (e *Engine) GetFileStream(ctx context.Context, hash string, file int) (io.ReadSeekCloser, *torrent.FileInfo, error) {
	ent, id, ok := e.lookup(hash)
	if !ok {
		r, fi, err := e.inner.GetFileStream(ctx, hash, file)
		if err != nil {
			return nil, nil, err
		}
		return e.track(streamKey{hash, file}, r), fi, nil
	}
	if file != 0 {
		return nil, nil, fmt.Errorf("library: copy has a single file, got index %d", file)
	}
	path, err := e.pool.Path(ent.DiskID, ent.RelPath)
	if errors.Is(err, ErrDiskAbsent) {
		return nil, nil, ErrLibraryMiss
	}
	if err != nil {
		return nil, nil, err
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		_ = e.store.Delete(ent.Key)
		return nil, nil, ErrLibraryMiss
	}
	if err != nil {
		return nil, nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	_ = e.store.Touch(ent.Key, e.clock())
	fi := &torrent.FileInfo{
		Index:    0,
		Path:     fmt.Sprintf("%d-%s.mkv", ent.Episode, ent.Lang),
		Length:   st.Size(),
		IsVideo:  true,
		MimeType: "video/x-matroska",
	}
	return e.track(streamKey{id, 0}, f), fi, nil
}

func (e *Engine) GetStats(hash string) (*torrent.SwarmStats, error) {
	ent, id, ok := e.lookup(hash)
	if !ok {
		return e.inner.GetStats(hash)
	}
	return &torrent.SwarmStats{
		InfoHash:       id,
		Title:          fmt.Sprintf("%d-%s.mkv", ent.Episode, ent.Lang),
		TotalBytes:     ent.SizeBytes,
		CompletedBytes: ent.SizeBytes,
		ProgressPct:    100,
	}, nil
}

func (e *Engine) Close() error { return e.inner.Close() }

// ActiveStreams is the number of distinct (hash, file) pairs with at least one open reader.
func (e *Engine) ActiveStreams() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.readers)
}

// InUse reports whether any reader of k's stream is open.
func (e *Engine) InUse(k Key) bool {
	id, err := e.store.StreamID(k)
	if err != nil {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for sk := range e.readers {
		if sk.hash == id {
			return true
		}
	}
	return false
}

func (e *Engine) track(k streamKey, r io.ReadSeekCloser) io.ReadSeekCloser {
	e.mu.Lock()
	e.readers[k]++
	e.mu.Unlock()
	return &trackedReader{ReadSeekCloser: r, e: e, k: k}
}

type trackedReader struct {
	io.ReadSeekCloser
	e    *Engine
	k    streamKey
	once sync.Once
	err  error
}

func (t *trackedReader) Close() error {
	t.once.Do(func() {
		t.err = t.ReadSeekCloser.Close()
		t.e.mu.Lock()
		if t.e.readers[t.k]--; t.e.readers[t.k] <= 0 {
			delete(t.e.readers, t.k)
		}
		t.e.mu.Unlock()
	})
	return t.err
}
