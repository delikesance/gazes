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
	watchEvery        = 10 * time.Second
	maxActiveDownload = 4
	newCopiesPerHour  = 20
	rateWindow        = time.Hour
)

// ErrRateLimited means the user already created the maximum number of new copies this hour.
var ErrRateLimited = errors.New("library: too many new copies")

// Options configures Open.
type Options struct {
	PoolDir, IndexDir, FFmpeg, FFprobe string
	ReservePercent                     int
	ReserveBytes                       int64
	Stall                              time.Duration
	Encode                             EncodeSettings
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
	return &Service{
		store: store, pool: pool, engine: eng, logger: logger, clock: clock,
		acq:     NewAcquirer(store, pool, fetcher, prober, clock, opts.Stall, maxActiveDownload),
		enc:     NewEncoder(store, pool, prober, eng.ActiveStreams, opts.Encode, clock),
		janitor: NewJanitor(store, pool, eng.InUse, clock),
		rate:    map[int64][]time.Time{},
	}, nil
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
	run(s.watchLoop)
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

// watchLoop logs every state transition of the index (downloads finishing, encodes, abandons).
func (s *Service) watchLoop(ctx context.Context) {
	seen := map[Key]State{}
	first := true
	t := time.NewTicker(watchEvery)
	defer t.Stop()
	for {
		list, err := s.store.List(Filter{})
		if err == nil {
			cur := make(map[Key]State, len(list))
			for _, e := range list {
				cur[e.Key] = e.State
				prev, known := seen[e.Key]
				if first || (known && prev == e.State) {
					continue
				}
				event := "library.state"
				if e.State == StateAV1 {
					event = "library.encode"
				}
				diagnostics.Log(ctx, slog.LevelInfo, event, "season_id", e.SeasonID, "episode", e.Episode, "lang", e.Lang,
					"disk_id", e.DiskID, "state", string(e.State), "duration_ms", e.DurationMS)
			}
			seen, first = cur, false
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Register starts caching req for userID. It reports whether a new copy was created; an existing copy is only touched.
func (s *Service) Register(userID int64, req Request) (Entry, bool, error) {
	s.regMu.Lock()
	defer s.regMu.Unlock()
	now := s.clock()
	recent := s.recent(userID, now)
	if len(recent) >= newCopiesPerHour {
		// Over the limit: touching an existing copy is still fine, creating one is not.
		if e, err := s.store.Get(req.Key); err == nil {
			if err := s.store.Touch(req.Key, now); err != nil && !errors.Is(err, ErrNotFound) {
				return Entry{}, false, err
			}
			return e, false, nil
		}
		return Entry{}, false, ErrRateLimited
	}
	e, created, err := s.acq.Start(req)
	if errors.Is(err, ErrNoSpace) {
		for _, d := range s.pool.Disks() {
			if _, ferr := s.janitor.Free(d.ID); ferr != nil {
				s.logger.Warn("library.evict", "disk_id", d.ID, "error", ferr.Error())
			}
		}
		e, created, err = s.acq.Start(req)
	}
	if err != nil {
		return Entry{}, false, err
	}
	if created {
		s.rate[userID] = append(recent, now)
		diagnostics.Log(context.Background(), slog.LevelInfo, "library.state", "season_id", e.SeasonID, "episode", e.Episode,
			"lang", e.Lang, "disk_id", e.DiskID, "state", string(e.State), "duration_ms", e.DurationMS)
	}
	return e, created, nil
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

// Copies lists the ready (ORIGINAL or AV1) copies of an episode.
func (s *Service) Copies(seasonID, episode int) ([]Copy, error) {
	list, err := s.store.List(Filter{SeasonID: seasonID, Episode: episode, States: []State{StateOriginal, StateAV1}})
	if err != nil {
		return nil, err
	}
	out := make([]Copy, 0, len(list))
	for _, e := range list {
		id, err := s.store.StreamID(e.Key)
		if err != nil {
			return nil, err
		}
		out = append(out, Copy{Lang: e.Lang, State: string(e.State), VideoCodec: e.VideoCodec, StreamID: id,
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
