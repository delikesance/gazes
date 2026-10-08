package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/gazes/gazes/internal/diagnostics"
	"github.com/gazes/gazes/internal/torrent"
)

const (
	scanEvery         = 60 * time.Second
	maxActiveDownload = 4
	newCopiesPerHour  = 20
	rateWindow        = time.Hour
)

// ErrRateLimited means the user already created the maximum number of new copies this hour.
var ErrRateLimited = errors.New("library: too many new copies")

// ErrWaitingViewers means the episode is not cached yet: fewer distinct users than MinViewers have watched it.
var ErrWaitingViewers = errors.New("library: waiting for more viewers")

// maxPendingKeys bounds the in-memory viewer tallies of episodes not cached yet.
const maxPendingKeys = 10000

// Options configures Open.
type Options struct {
	PoolDir, IndexDir, FFmpeg, FFprobe string
	ReservePercent                     int
	ReserveBytes                       int64
	Stall                              time.Duration
	// MinViewers is the number of distinct users who must register an episode before it is downloaded; below 2 every registration downloads.
	MinViewers int
	Encode     EncodeSettings
}

// Copy is a ready-to-serve copy of an episode, as exposed by the API.
type Copy struct {
	Lang           string `json:"lang"`
	State          string `json:"state"`
	VideoCodec     string `json:"video_codec"`
	StreamID       string `json:"stream_id"`
	DurationMS     int64  `json:"duration_ms"`
	AudioTracks    int    `json:"audio_tracks"`
	SubtitleTracks int    `json:"subtitle_tracks"`
}

// Service assembles the library components.
type Service struct {
	store   *Store
	pool    *Pool
	engine  *Engine
	acq     *Acquirer
	enc     *Encoder
	janitor *Janitor
	logger  *slog.Logger
	clock   func() time.Time

	regMu sync.Mutex
	rate  map[int64][]time.Time
	// minViewers and viewers gate the first download; viewers is in memory, so a restart resets the tallies.
	minViewers int
	viewers    map[Key]map[int64]struct{}

	wg        sync.WaitGroup
	closeOnce sync.Once
	closeErr  error
	cancel    context.CancelFunc
	mu        sync.Mutex
}

// Open builds the service: opens the index, repairs it after a crash, scans the disks and creates the workers.
func Open(opts Options, inner torrent.Engine, fetcher Fetcher, logger *slog.Logger) (*Service, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if opts.FFmpeg == "" {
		opts.FFmpeg = "ffmpeg"
	}
	if opts.FFprobe == "" {
		opts.FFprobe = "ffprobe"
	}
	opts.Encode.FFmpeg = opts.FFmpeg
	store, err := OpenStore(opts.IndexDir)
	if err != nil {
		return nil, fmt.Errorf("library: open index: %w", err)
	}
	if rep, err := store.Recover(); err != nil {
		store.Close()
		return nil, fmt.Errorf("library: recover index: %w", err)
	} else if rep.Encoding > 0 || rep.Downloading > 0 {
		logger.Info("library.recovered", "encoding", rep.Encoding, "downloading", rep.Downloading)
	}
	pool := NewPool(opts.PoolDir, store, SysStatFS, opts.ReservePercent, opts.ReserveBytes)
	if _, _, err := pool.Scan(); err != nil {
		logger.Warn("library.disk", "error", err.Error())
	}
	clock := time.Now
	eng := NewEngine(inner, store, pool, clock)
	prober := FFprobe(opts.FFprobe)
	svc := &Service{
		store: store, pool: pool, engine: eng, logger: logger, clock: clock,
		acq:        NewAcquirer(store, pool, fetcher, prober, clock, opts.Stall, maxActiveDownload),
		enc:        NewEncoder(store, pool, prober, eng.ActiveStreams, opts.Encode, clock),
		janitor:    NewJanitor(store, pool, eng.InUse, clock),
		rate:       map[int64][]time.Time{},
		minViewers: opts.MinViewers, viewers: map[Key]map[int64]struct{}{},
	}
	svc.enc.inUse = eng.InUse
	svc.enc.free = func(diskID string, need int64) { _, _ = svc.janitor.Free(diskID, need) }
	svc.watch()
	return svc, nil
}

// Engine returns the torrent engine that also serves library copies.
func (s *Service) Engine() torrent.Engine { return s.engine }

// Start launches the background workers; they stop when ctx ends or Close is called.
func (s *Service) Start(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return
	}
	ctx, s.cancel = context.WithCancel(ctx)
	run := func(f func(context.Context)) {
		s.wg.Add(1)
		go func() { defer s.wg.Done(); f(ctx) }()
	}
	run(s.scanLoop)
	run(func(ctx context.Context) {
		if err := s.acq.Resume(ctx); err != nil && ctx.Err() == nil {
			diagnostics.Log(ctx, slog.LevelWarn, "library.state", "error", err.Error())
		}
		s.acq.Run(ctx)
	})
	run(s.enc.Run)
	run(s.janitor.Run)
}

func (s *Service) scanLoop(ctx context.Context) {
	t := time.NewTicker(scanEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			added, removed, err := s.pool.Scan()
			if err != nil {
				diagnostics.Log(ctx, slog.LevelWarn, "library.disk", "error", err.Error())
			}
			for _, d := range added {
				diagnostics.Log(ctx, slog.LevelInfo, "library.disk", "disk_id", d.ID, "present", true)
			}
			for _, d := range removed {
				diagnostics.Log(ctx, slog.LevelWarn, "library.disk", "disk_id", d.ID, "present", false)
			}
		}
	}
}

// watch logs every state transition of the index from the store's change hook.
func (s *Service) watch() { s.store.SetOnChange(logChange) }

// logChange turns one committed index change into a diagnostics event.
func logChange(old, cur *Entry) {
	ctx := context.Background()
	ref := cur
	if ref == nil {
		ref = old
	}
	attrs := func(state State) []any {
		var ms int64
		if old != nil && cur != nil {
			ms = cur.UpdatedAt.Sub(old.UpdatedAt).Milliseconds()
		}
		return []any{"season_id", ref.SeasonID, "episode", ref.Episode, "lang", ref.Lang, "disk_id", ref.DiskID,
			"state", string(state), "duration_ms", ms}
	}
	switch {
	case cur == nil:
		if old.State == StateOriginal || old.State == StateAV1 {
			diagnostics.Log(ctx, slog.LevelInfo, "library.evict", attrs(old.State)...)
		} else {
			diagnostics.Log(ctx, slog.LevelInfo, "library.state", attrs("REMOVED")...)
		}
	case old == nil:
		diagnostics.Log(ctx, slog.LevelInfo, "library.state", attrs(cur.State)...)
	case old.State == cur.State:
	case old.State == StateEncoding && (cur.State == StateAV1 || cur.State == StateOriginal):
		diagnostics.Log(ctx, slog.LevelInfo, "library.encode", attrs(cur.State)...)
	case old.State == StateUnavailable || cur.State == StateUnavailable:
		diagnostics.Log(ctx, slog.LevelWarn, "library.disk", attrs(cur.State)...)
	default:
		diagnostics.Log(ctx, slog.LevelInfo, "library.state", attrs(cur.State)...)
	}
}

// Register starts caching req for userID once MinViewers distinct users registered it (else ErrWaitingViewers). It reports whether a new copy was created; an existing copy is only touched.
func (s *Service) Register(userID int64, req Request) (Entry, bool, error) {
	s.regMu.Lock()
	defer s.regMu.Unlock()
	now := s.clock()
	if s.minViewers > 1 && !s.enoughViewers(userID, req.Key) {
		return Entry{}, false, ErrWaitingViewers
	}
	recent := s.recent(userID, now)
	if len(recent) >= newCopiesPerHour {
		// Over the limit: touching an existing copy is still fine, creating one is not.
		e, err := s.store.Get(req.Key)
		if errors.Is(err, ErrNotFound) {
			return Entry{}, false, ErrRateLimited
		}
		if err != nil {
			return Entry{}, false, err
		}
		if err := s.store.Touch(req.Key, now); err != nil && !errors.Is(err, ErrNotFound) {
			return Entry{}, false, err
		}
		return e, false, nil
	}
	e, created, err := s.acq.Start(req)
	var ns needSpaceError
	if errors.As(err, &ns) {
		// Evict least recently used copies until one disk can hold the file, then retry once.
		for _, d := range s.pool.Disks() {
			if _, ferr := s.janitor.Free(d.ID, ns.need); ferr != nil {
				s.logger.Warn("library.evict", "disk_id", d.ID, "error", ferr.Error())
			}
			if s.pool.Shortfall(d.ID, ns.need) <= 0 {
				break
			}
		}
		e, created, err = s.acq.Start(req)
		if errors.As(err, &ns) {
			err = ErrNoSpace
		}
	}
	if err != nil {
		return Entry{}, false, err
	}
	if created {
		s.rate[userID] = append(recent, now)
	}
	return e, created, nil
}

// enoughViewers records userID as a viewer of k and reports whether the episode may be downloaded: a copy already in the index always may.
func (s *Service) enoughViewers(userID int64, k Key) bool {
	if _, err := s.store.Get(k); err == nil {
		delete(s.viewers, k)
		return true
	}
	set := s.viewers[k]
	if set == nil {
		if len(s.viewers) >= maxPendingKeys {
			s.viewers = map[Key]map[int64]struct{}{}
		}
		set = map[int64]struct{}{}
		s.viewers[k] = set
	}
	set[userID] = struct{}{}
	if len(set) < s.minViewers {
		return false
	}
	delete(s.viewers, k)
	return true
}

func (s *Service) recent(userID int64, now time.Time) []time.Time {
	var keep []time.Time
	for _, t := range s.rate[userID] {
		if now.Sub(t) < rateWindow {
			keep = append(keep, t)
		}
	}
	if len(keep) == 0 {
		delete(s.rate, userID)
	}
	return keep
}

// Copies lists the ready (ORIGINAL, ENCODING or AV1) copies; an ENCODING copy is still the original file and is reported as ORIGINAL of an episode.
func (s *Service) Copies(seasonID, episode int) ([]Copy, error) {
	list, err := s.store.List(Filter{SeasonID: seasonID, Episode: episode, States: []State{StateOriginal, StateEncoding, StateAV1}})
	if err != nil {
		return nil, err
	}
	out := make([]Copy, 0, len(list))
	for _, e := range list {
		id, err := s.store.StreamID(e.Key)
		if err != nil {
			return nil, err
		}
		state := e.State
		if state == StateEncoding { // the original is untouched at RelPath until the encoder swaps it
			state = StateOriginal
		}
		out = append(out, Copy{Lang: e.Lang, State: string(state), VideoCodec: e.VideoCodec, StreamID: id,
			DurationMS: e.DurationMS, AudioTracks: e.AudioTracks, SubtitleTracks: e.SubtitleTracks})
	}
	return out, nil
}

// Close stops the workers and closes the index. The torrent engine is left to its owner.
func (s *Service) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		if s.cancel != nil {
			s.cancel()
		}
		s.mu.Unlock()
		s.wg.Wait()
		s.closeErr = s.store.Close()
	})
	return s.closeErr
}
