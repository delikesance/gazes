package admin

import (
	"context"
	"sync"
)

// PlaybackStats is what the playback manager exposes to the admin health route.
type PlaybackStats interface{ ActiveSessions() int }

// CostInputs are the optional monetary inputs of GET /costs. A nil field was not provided
// and is never defaulted.
type CostInputs struct {
	ServerMonth       *float64 // server cost per month
	BandwidthPerGB    *float64 // cost per GB served
	StoragePerGBMonth *float64 // cost per GB stored per month
	GBPerWatchHour    *float64 // data served per hour watched
}

// playbackDeps are the optional collaborators of the playback/costs routes.
type playbackDeps struct {
	stats PlaybackStats
	cache func(ctx context.Context) any
	costs CostInputs
}

// The deps live beside the Service (keyed by it) so the Service type, owned by api.go, does
// not need a field per route group.
var (
	playbackDepsMu sync.Mutex
	playbackDepsOf = map[*Service]*playbackDeps{}
)

func (s *Service) pbDeps() playbackDeps {
	playbackDepsMu.Lock()
	defer playbackDepsMu.Unlock()
	if d := playbackDepsOf[s]; d != nil {
		return *d
	}
	return playbackDeps{}
}

func (s *Service) pbSet(f func(*playbackDeps)) {
	playbackDepsMu.Lock()
	defer playbackDepsMu.Unlock()
	d := playbackDepsOf[s]
	if d == nil {
		d = &playbackDeps{}
		playbackDepsOf[s] = d
	}
	f(d)
}

// SetPlaybackStats sets the source of the active-session count (nil: not measured).
func (s *Service) SetPlaybackStats(p PlaybackStats) { s.pbSet(func(d *playbackDeps) { d.stats = p }) }

// SetCacheDiagnostics sets the function returning the /api/v1/diagnostics/cache body.
func (s *Service) SetCacheDiagnostics(f func(ctx context.Context) any) {
	s.pbSet(func(d *playbackDeps) { d.cache = f })
}

// SetCostInputs sets the optional cost inputs.
func (s *Service) SetCostInputs(c CostInputs) {
	s.pbSet(func(d *playbackDeps) { d.costs = c })
	s.respCache.clear() // /costs answers depend on these inputs
}

// maxStartupMS bounds a plausible time to first frame; larger values are a stale tab, not a startup.
const maxStartupMS = 120_000

// RecordStartup stores one client-measured time to first frame. Implausible values and write
// failures are dropped: a metric must never affect a playback.
func (s *Service) RecordStartup(ctx context.Context, ms float64) {
	if s == nil || !(ms > 0 && ms <= maxStartupMS) {
		return
	}
	_, _ = s.adminDB().ExecContext(ctx, `INSERT INTO playback_startups(ts, ms) VALUES(?, ?)`, s.now().Unix(), int64(ms))
}
