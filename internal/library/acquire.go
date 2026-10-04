package library

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gazes/gazes/internal/torrent"
)

// ErrBusy means the maximum number of simultaneous full downloads is reached.
var ErrBusy = errors.New("library: too many active downloads")

// pollInterval is how often Run checks the active downloads.
const pollInterval = 30 * time.Second

// Fetcher is the part of the torrent engine the library uses.
type Fetcher interface {
	GetFileStream(ctx context.Context, infoHash string, fileIndex int) (io.ReadSeekCloser, *torrent.FileInfo, error)
	DownloadFile(infoHash string, fileIndex int) error
	ReleaseFile(infoHash string, fileIndex int)
	FileProgress(infoHash string, fileIndex int) (completed, length int64, err error)
	VerifyFile(ctx context.Context, infoHash string, fileIndex int) error
}

// Request describes an episode file to cache.
type Request struct {
	Key
	AnimeID     int
	Title       string
	InfoHash    string
	FileIndex   int
	ReleaseName string
}

type progress struct {
	bytes int64
	at    time.Time
}

// Acquirer downloads episode files whole and copies them into the library pool.
type Acquirer struct {
	store     *Store
	pool      *Pool
	fetch     Fetcher
	probe     Prober
	clock     func() time.Time
	stall     time.Duration
	maxActive int
	interval  time.Duration

	mu           sync.Mutex
	reservations map[Key]Reservation
	last         map[Key]progress
}

func NewAcquirer(store *Store, pool *Pool, f Fetcher, p Prober, clock func() time.Time, stall time.Duration, maxActive int) *Acquirer {
	return &Acquirer{store: store, pool: pool, fetch: f, probe: p, clock: clock, stall: stall, maxActive: maxActive,
		interval: pollInterval, reservations: map[Key]Reservation{}, last: map[Key]progress{}}
}

// Start begins caching req. It returns the entry and true when a new copy was created. An existing
// copy (in any state, including UNAVAILABLE) is only touched.
func (a *Acquirer) Start(req Request) (Entry, bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if e, err := a.store.Get(req.Key); err == nil {
		if err := a.store.Touch(req.Key, a.clock()); err != nil && !errors.Is(err, ErrNotFound) {
			return Entry{}, false, err
		}
		return e, false, nil
	} else if !errors.Is(err, ErrNotFound) {
		return Entry{}, false, err
	}

	active, err := a.store.List(Filter{States: []State{StateDownloading}})
	if err != nil {
		return Entry{}, false, err
	}
	if len(active) >= a.maxActive {
		return Entry{}, false, ErrBusy
	}

	_, length, err := a.fetch.FileProgress(req.InfoHash, req.FileIndex)
	if err != nil {
		return Entry{}, false, err
	}
	res, err := a.pool.Reserve(length)
	if err != nil {
		return Entry{}, false, err
	}
	if err := a.fetch.DownloadFile(req.InfoHash, req.FileIndex); err != nil {
		a.fetch.ReleaseFile(req.InfoHash, req.FileIndex)
		a.pool.Release(res)
		return Entry{}, false, err
	}
	now := a.clock()
	e := Entry{Key: req.Key, AnimeID: req.AnimeID, Title: req.Title, State: StateDownloading,
		InfoHash: req.InfoHash, FileIndex: req.FileIndex, ReleaseName: req.ReleaseName,
		DiskID: res.DiskID, SizeBytes: length, OriginalSizeBytes: length, ReservedBytes: length,
		CreatedAt: now, UpdatedAt: now, LastAccessAt: now}
	if err := a.store.Create(e); err != nil {
		a.fetch.ReleaseFile(req.InfoHash, req.FileIndex)
		a.pool.Release(res)
		if errors.Is(err, ErrExists) {
			cur, gerr := a.store.Get(req.Key)
			if gerr == nil {
				_ = a.store.Touch(req.Key, now)
				return cur, false, nil
			}
		}
		return Entry{}, false, err
	}
	a.reservations[req.Key] = res
	a.last[req.Key] = progress{at: now}
	return e, true, nil
}

// Resume re-pins the DOWNLOADING entries after a restart; entries that cannot be resumed are deleted.
func (a *Acquirer) Resume(ctx context.Context) error {
	entries, err := a.store.List(Filter{States: []State{StateDownloading}})
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := a.fetch.DownloadFile(e.InfoHash, e.FileIndex); err != nil {
			a.fetch.ReleaseFile(e.InfoHash, e.FileIndex)
			if derr := a.store.Delete(e.Key); derr != nil {
				return derr
			}
			continue
		}
		if _, length, err := a.fetch.FileProgress(e.InfoHash, e.FileIndex); err == nil {
			if res, rerr := a.pool.Reserve(length); rerr == nil {
				a.reservations[e.Key] = res
			}
		}
		a.last[e.Key] = progress{at: a.clock()}
	}
	return nil
}

// Run polls the active downloads until ctx is done.
func (a *Acquirer) Run(ctx context.Context) {
	t := time.NewTicker(a.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.step(ctx)
		}
	}
}

// step checks every DOWNLOADING entry once: finishing complete ones and abandoning stalled ones.
func (a *Acquirer) step(ctx context.Context) {
	entries, err := a.store.List(Filter{States: []State{StateDownloading}})
	if err != nil {
		return
	}
	for _, e := range entries {
		if ctx.Err() != nil {
			return
		}
		a.check(ctx, e)
	}
}

func (a *Acquirer) check(ctx context.Context, e Entry) {
	now := a.clock()
	completed, length, perr := a.fetch.FileProgress(e.InfoHash, e.FileIndex)

	a.mu.Lock()
	prev, seen := a.last[e.Key]
	if !seen {
		prev = progress{at: now}
		a.last[e.Key] = prev
	}
	if perr == nil && completed > prev.bytes {
		prev = progress{bytes: completed, at: now}
		a.last[e.Key] = prev
	}
	a.mu.Unlock()

	if perr == nil && completed >= length {
		if err := a.finish(ctx, e); err != nil {
			a.noteError(e.Key, err)
			// fall through to the stall check: a file that never verifies must not live forever.
		} else {
			return
		}
	}
	if now.Sub(prev.at) > a.stall {
		a.abandon(e)
	}
}

func (a *Acquirer) noteError(k Key, err error) {
	_, _ = a.store.Update(k, func(e *Entry) error { e.LastError = err.Error(); return nil })
}

// abandon drops a stalled download.
func (a *Acquirer) abandon(e Entry) {
	if err := a.store.Delete(e.Key); err != nil {
		// keep the pin, reservation and tracking so the next tick retries the abandon.
		a.noteError(e.Key, fmt.Errorf("abandon: %w", err))
		return
	}
	a.fetch.ReleaseFile(e.InfoHash, e.FileIndex)
	a.releaseReservation(e.Key)
}

func (a *Acquirer) releaseReservation(k Key) {
	a.mu.Lock()
	res := a.reservations[k]
	delete(a.reservations, k)
	delete(a.last, k)
	a.mu.Unlock()
	a.pool.Release(res)
}

// finish verifies a fully downloaded file, copies it into the pool and flips the entry to ORIGINAL.
func (a *Acquirer) finish(ctx context.Context, e Entry) error {
	if err := a.fetch.VerifyFile(ctx, e.InfoHash, e.FileIndex); err != nil {
		return fmt.Errorf("verify: %w", err)
	}
	rel := fmt.Sprintf("%d/%d-%s.mkv", e.SeasonID, e.Episode, e.Lang)
	final, err := a.pool.Path(e.DiskID, rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(final), 0o755); err != nil {
		return err
	}
	sum, size, err := a.copyTo(ctx, e, final+".tmp", final)
	if err != nil {
		return err
	}
	info, err := a.probe.Probe(ctx, final)
	if err != nil {
		os.Remove(final)
		return fmt.Errorf("probe: %w", err)
	}
	_, err = a.store.Update(e.Key, func(en *Entry) error {
		en.State = StateOriginal
		en.RelPath = rel
		en.SHA256 = sum
		en.SizeBytes, en.OriginalSizeBytes, en.ReservedBytes = size, size, 0
		en.DurationMS = info.DurationMS
		en.VideoCodec = info.VideoCodec
		en.AudioTracks = len(info.AudioCodecs)
		en.SubtitleTracks = info.SubtitleTracks
		en.LastError = ""
		return nil
	})
	if err != nil {
		os.Remove(final)
		return err
	}
	a.releaseReservation(e.Key)
	a.fetch.ReleaseFile(e.InfoHash, e.FileIndex)
	return nil
}

// copyTo streams the torrent file into tmp, syncs it and renames it to final.
func (a *Acquirer) copyTo(ctx context.Context, e Entry, tmp, final string) (sum string, size int64, err error) {
	src, _, err := a.fetch.GetFileStream(ctx, e.InfoHash, e.FileIndex)
	if err != nil {
		return "", 0, err
	}
	defer src.Close()
	dst, err := os.Create(tmp)
	if err != nil {
		return "", 0, err
	}
	ok := false
	defer func() {
		if !ok {
			dst.Close()
			os.Remove(tmp)
		}
	}()
	h := sha256.New()
	size, err = io.Copy(io.MultiWriter(dst, h), src)
	if err != nil {
		return "", 0, err
	}
	if err := dst.Sync(); err != nil {
		return "", 0, err
	}
	if err := dst.Close(); err != nil {
		return "", 0, err
	}
	if err := os.Rename(tmp, final); err != nil {
		return "", 0, err
	}
	ok = true
	return hex.EncodeToString(h.Sum(nil)), size, nil
}
