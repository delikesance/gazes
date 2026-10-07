package admin

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gazes/gazes/internal/indexer"
	"github.com/gazes/gazes/internal/library"
)

// Runtime actions: the ones whose effect lands outside the admin database (stream sources, caches,
// the AV1 queue, playback admission, the public status). Their collaborators are given by the API
// server through SetOpsHooks; an action whose collaborator is missing answers 503 unavailable and
// has no effect (the standalone MCP binary, the legacy playback engine, a disabled library).

// SourceControl is the operational control of the stream sources (indexer providers).
type SourceControl interface {
	ProviderNames() []string
	Health(ctx context.Context, name string) (indexer.ProviderHealth, bool)
	Pause(ctx context.Context, name string, d time.Duration) bool
	Resume(ctx context.Context, name string) bool
	Retry(ctx context.Context, name string) bool
}

// AV1Queue is the part of the episode library the requeue_av1 action uses.
type AV1Queue interface {
	EncodeQueue(k library.Key) (library.EncodeQueueState, bool, error)
	RequeueEncode(k library.Key) (library.EncodeQueueState, error)
	RestoreEncodeQueue(k library.Key, prev library.EncodeQueueState) error
}

// OpsHooks are the effects of the runtime actions. A nil field disables its action(s).
type OpsHooks struct {
	Sources SourceControl // retry_source, pause_source
	Library AV1Queue      // requeue_av1
	// EpisodeSources reports whether the source findings of one episode are cached, and how many.
	EpisodeSources func(ctx context.Context, seasonID, episode int) (count int, cached bool)
	// WarmEpisode starts resolving one episode's sources in the background (dropping the cached
	// findings first when refresh is set). It returns false when too many warms already run.
	WarmEpisode func(seasonID, episode int, refresh bool) bool // warm_cache
	// PurgeCache empties one cache scope (see cacheScopes) and returns the number of keys removed.
	PurgeCache func(ctx context.Context, scope string) (int, error) // purge_cache
}

// SetOpsHooks sets the collaborators of the runtime actions (called by the API server).
func (s *Service) SetOpsHooks(h OpsHooks) {
	s.mu.Lock()
	s.hooks = h
	s.mu.Unlock()
}

func (s *Service) opsHooks() OpsHooks {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hooks
}

func unavailable(msg string) error {
	return &ActionError{Status: http.StatusServiceUnavailable, Code: "unavailable", Msg: msg}
}

func conflict(code, msg string) error {
	return &ActionError{Status: http.StatusConflict, Code: code, Msg: msg}
}

// Settings of the runtime actions.
const (
	settingStreamLimit = "ops.max_concurrent_streams"
	settingMaintenance = "ops.maintenance"
)

// Bounds of the runtime actions.
const (
	pauseMinMinutes      = 5
	pauseMaxMinutes      = 24 * 60
	maxStreamLimit       = 1000
	maintenanceMaxLength = 24 * time.Hour
	maintenanceMaxAhead  = 30 * 24 * time.Hour
	maintenanceMaxMsg    = 200
)

// cacheScopes are the caches purge_cache may empty. The catalog caches are deliberately absent:
// refilling them would hit the AniList rate limit for every visitor.
var cacheScopes = map[string]string{
	"episode_sources": "source findings of every episode (each one is resolved again on its next play)",
	"indexer_results": "tracker answers of every provider (the next searches query the trackers again)",
}

// ---- argument helpers -----------------------------------------------------------------------

func intArg(args map[string]any, name string, required bool, min, max int) (int, bool, error) {
	v, present := args[name]
	if !present || v == nil {
		if required {
			return 0, false, badArgs("missing required argument %q", name)
		}
		return 0, false, nil
	}
	f, ok := v.(float64)
	if !ok || math.IsNaN(f) || math.IsInf(f, 0) || f != math.Trunc(f) {
		return 0, false, badArgs("argument %q must be an integer", name)
	}
	if f < float64(min) || f > float64(max) {
		return 0, false, badArgs("argument %q must be between %d and %d", name, min, max)
	}
	return int(f), true, nil
}

func boolArg(args map[string]any, name string) (bool, error) {
	v, present := args[name]
	if !present || v == nil {
		return false, nil
	}
	b, ok := v.(bool)
	if !ok {
		return false, badArgs("argument %q must be a boolean", name)
	}
	return b, nil
}

func episodeArgs(args map[string]any) (season, ep int, err error) {
	if season, _, err = intArg(args, "season_id", true, 1, math.MaxInt32); err != nil {
		return
	}
	ep, _, err = intArg(args, "episode", true, 1, 100000)
	return
}

// settingUndo is the undo token of the actions that only write one setting.
type settingUndo struct {
	Key  string  `json:"key"`
	Prev *string `json:"prev"`
}

// replaceSetting writes key and returns the undo token restoring its previous value.
func (s *Service) replaceSetting(ctx context.Context, key, value string) (string, error) {
	prev, found, err := s.getSetting(ctx, key)
	if err != nil {
		return "", err
	}
	if err := s.setSetting(ctx, key, value, actorFrom(ctx)); err != nil {
		return "", err
	}
	u := settingUndo{Key: key}
	if found {
		u.Prev = &prev
	}
	b, _ := json.Marshal(u)
	return string(b), nil
}

func (s *Service) undoSetting(key string) func(context.Context, string) error {
	return func(ctx context.Context, token string) error {
		var u settingUndo
		if json.Unmarshal([]byte(token), &u) != nil || u.Key != key {
			return badArgs("invalid undo token")
		}
		if u.Prev == nil {
			return s.deleteSetting(ctx, key)
		}
		return s.setSetting(ctx, key, *u.Prev, actorFrom(ctx))
	}
}

func seconds(d time.Duration) int64 { return int64(math.Ceil(d.Seconds())) }

// ---- stream sources ---------------------------------------------------------------------------

func (s *Service) sourceArg(args map[string]any) (SourceControl, string, error) {
	name, err := strArg(args, "source", true, 100)
	if err != nil {
		return nil, "", err
	}
	src := s.opsHooks().Sources
	if src == nil {
		return nil, "", unavailable("stream sources are not reachable from this process")
	}
	names := src.ProviderNames()
	if !slices.Contains(names, name) {
		return nil, "", badArgs("source must be one of: %s", strings.Join(names, ", "))
	}
	return src, name, nil
}

func (s *Service) actRetrySource() Action {
	check := func(ctx context.Context, args map[string]any) (SourceControl, string, indexer.ProviderHealth, error) {
		src, name, err := s.sourceArg(args)
		if err != nil {
			return nil, "", indexer.ProviderHealth{}, err
		}
		h, _ := src.Health(ctx, name)
		if h.Paused > 0 {
			return nil, "", h, conflict("source_paused", "this source is paused; undo the pause_source approval to resume it")
		}
		return src, name, h, nil
	}
	return Action{
		Name: "retry_source", Level: ActionReversible, Scope: ScopeOpsWrite, Implemented: true,
		Summary: "Retry a failing stream source now: close its circuit breaker instead of waiting for the backoff",
		Validate: func(args map[string]any) error {
			if err := onlyKeys(args, "source"); err != nil {
				return err
			}
			_, err := strArg(args, "source", true, 100)
			return err
		},
		Plan: func(ctx context.Context, args map[string]any) (Plan, error) {
			_, name, h, err := check(ctx, args)
			if err != nil {
				return Plan{}, err
			}
			summary := "Close the circuit breaker of " + name + " (open for " + strconv.FormatInt(seconds(h.Cooldown), 10) + " s more)"
			if h.Cooldown == 0 {
				summary = "The circuit breaker of " + name + " is already closed: no effect"
			}
			return Plan{Summary: summary, Details: map[string]any{"source": name, "breaker_open_s": seconds(h.Cooldown)}}, nil
		},
		Do: func(ctx context.Context, args map[string]any) (Result, string, error) {
			src, name, h, err := check(ctx, args)
			if err != nil {
				return nil, "", err
			}
			src.Retry(ctx, name)
			return Result{"source": name, "breaker_was_open_s": seconds(h.Cooldown)}, "", nil
		},
	}
}

func (s *Service) actPauseSource() Action {
	parse := func(args map[string]any) (int, error) {
		if err := onlyKeys(args, "source", "minutes"); err != nil {
			return 0, err
		}
		if _, err := strArg(args, "source", true, 100); err != nil {
			return 0, err
		}
		m, _, err := intArg(args, "minutes", true, pauseMinMinutes, pauseMaxMinutes)
		return m, err
	}
	// check refuses a pause that would leave no source to resolve episodes with.
	check := func(ctx context.Context, args map[string]any) (SourceControl, string, int, []string, error) {
		minutes, err := parse(args)
		if err != nil {
			return nil, "", 0, nil, err
		}
		src, name, err := s.sourceArg(args)
		if err != nil {
			return nil, "", 0, nil, err
		}
		var active []string
		for _, n := range src.ProviderNames() {
			if h, _ := src.Health(ctx, n); n != name && h.Paused == 0 {
				active = append(active, n)
			}
		}
		if len(active) == 0 {
			return nil, "", 0, nil, conflict("last_source", "pausing "+name+" would leave no active stream source")
		}
		return src, name, minutes, active, nil
	}
	type pauseUndo struct {
		Source string `json:"source"`
	}
	return Action{
		Name: "pause_source", Level: ActionSensitive, Scope: ScopeConfigWrite, Implemented: true,
		Summary: "Pause a stream source for 5 to 1440 minutes (its queries are skipped, the other sources answer)",
		Validate: func(args map[string]any) error {
			_, err := parse(args)
			return err
		},
		Plan: func(ctx context.Context, args map[string]any) (Plan, error) {
			_, name, minutes, active, err := check(ctx, args)
			if err != nil {
				return Plan{}, err
			}
			return Plan{Summary: "Pause " + name + " for " + strconv.Itoa(minutes) + " min",
				Details: map[string]any{"source": name, "minutes": minutes, "still_active": active}}, nil
		},
		Do: func(ctx context.Context, args map[string]any) (Result, string, error) {
			src, name, minutes, _, err := check(ctx, args)
			if err != nil {
				return nil, "", err
			}
			d := time.Duration(minutes) * time.Minute
			src.Pause(ctx, name, d)
			b, _ := json.Marshal(pauseUndo{Source: name})
			return Result{"source": name, "paused_until": s.now().Add(d).UTC().Format(time.RFC3339)}, string(b), nil
		},
		Undo: func(ctx context.Context, token string) error {
			var u pauseUndo
			if json.Unmarshal([]byte(token), &u) != nil || u.Source == "" {
				return badArgs("invalid undo token")
			}
			src := s.opsHooks().Sources
			if src == nil {
				return unavailable("stream sources are not reachable from this process")
			}
			if !src.Resume(ctx, u.Source) {
				return notFound("source")
			}
			return nil
		},
	}
}

// ---- caches -------------------------------------------------------------------------------

func (s *Service) actWarmCache() Action {
	parse := func(args map[string]any) (season, ep int, refresh bool, err error) {
		if err = onlyKeys(args, "season_id", "episode", "refresh"); err != nil {
			return
		}
		if season, ep, err = episodeArgs(args); err != nil {
			return
		}
		refresh, err = boolArg(args, "refresh")
		return
	}
	hooks := func() (OpsHooks, error) {
		h := s.opsHooks()
		if h.WarmEpisode == nil || h.EpisodeSources == nil {
			return h, unavailable("the source cache is not reachable from this process")
		}
		return h, nil
	}
	return Action{
		Name: "warm_cache", Level: ActionReversible, Scope: ScopeOpsWrite, Implemented: true,
		Summary: "Resolve the sources of an episode in the background so its next play starts from the cache",
		Validate: func(args map[string]any) error {
			_, _, _, err := parse(args)
			return err
		},
		Plan: func(ctx context.Context, args map[string]any) (Plan, error) {
			season, ep, refresh, err := parse(args)
			if err != nil {
				return Plan{}, err
			}
			h, err := hooks()
			if err != nil {
				return Plan{}, err
			}
			n, cached := h.EpisodeSources(ctx, season, ep)
			d := map[string]any{"season_id": season, "episode": ep, "cached": cached, "cached_sources": n, "refresh": refresh}
			switch {
			case cached && !refresh:
				return Plan{Summary: "Already cached (" + strconv.Itoa(n) + " sources): no effect, pass refresh to resolve again", Details: d}, nil
			case cached:
				return Plan{Summary: "Drop the cached findings and resolve the episode again in the background", Details: d}, nil
			}
			return Plan{Summary: "Resolve the episode's sources in the background", Details: d}, nil
		},
		Do: func(ctx context.Context, args map[string]any) (Result, string, error) {
			season, ep, refresh, err := parse(args)
			if err != nil {
				return nil, "", err
			}
			h, err := hooks()
			if err != nil {
				return nil, "", err
			}
			if n, cached := h.EpisodeSources(ctx, season, ep); cached && !refresh {
				return Result{"season_id": season, "episode": ep, "started": false, "cached_sources": n}, "", nil
			}
			if !h.WarmEpisode(season, ep, refresh) {
				return nil, "", &ActionError{Status: http.StatusTooManyRequests, Code: "busy", Msg: "other episodes are being warmed, retry in a minute"}
			}
			return Result{"season_id": season, "episode": ep, "started": true}, "", nil
		},
	}
}

func (s *Service) actPurgeCache() Action {
	parse := func(args map[string]any) (string, error) {
		if err := onlyKeys(args, "scope"); err != nil {
			return "", err
		}
		scope, err := strArg(args, "scope", true, 40)
		if err != nil {
			return "", err
		}
		if _, ok := cacheScopes[scope]; !ok {
			return "", badArgs("scope must be episode_sources or indexer_results")
		}
		return scope, nil
	}
	purge := func() (func(context.Context, string) (int, error), error) {
		if p := s.opsHooks().PurgeCache; p != nil {
			return p, nil
		}
		return nil, unavailable("the caches are not reachable from this process")
	}
	return Action{
		Name: "purge_cache", Level: ActionSensitive, Scope: ScopeConfigWrite, Implemented: true,
		Summary: "Purge a cache scope (episode_sources or indexer_results); it cannot be undone, it refills on demand",
		Validate: func(args map[string]any) error {
			_, err := parse(args)
			return err
		},
		Plan: func(_ context.Context, args map[string]any) (Plan, error) {
			scope, err := parse(args)
			if err != nil {
				return Plan{}, err
			}
			if _, err := purge(); err != nil {
				return Plan{}, err
			}
			return Plan{Summary: "Purge " + scope + ": " + cacheScopes[scope], Details: map[string]any{"scope": scope, "undoable": false}}, nil
		},
		Do: func(ctx context.Context, args map[string]any) (Result, string, error) {
			scope, err := parse(args)
			if err != nil {
				return nil, "", err
			}
			p, err := purge()
			if err != nil {
				return nil, "", err
			}
			n, err := p(ctx, scope)
			if err != nil {
				return nil, "", err
			}
			return Result{"scope": scope, "removed_keys": n}, "", nil
		},
	}
}

// ---- AV1 queue ------------------------------------------------------------------------------

func (s *Service) actRequeueAV1() Action {
	parse := func(args map[string]any) (library.Key, error) {
		if err := onlyKeys(args, "season_id", "episode", "lang"); err != nil {
			return library.Key{}, err
		}
		season, ep, err := episodeArgs(args)
		if err != nil {
			return library.Key{}, err
		}
		lang, err := strArg(args, "lang", true, 10)
		if err != nil {
			return library.Key{}, err
		}
		if lang != "vostfr" && lang != "vf" {
			return library.Key{}, badArgs("lang must be vostfr or vf")
		}
		return library.Key{SeasonID: season, Episode: ep, Lang: lang}, nil
	}
	queue := func() (AV1Queue, error) {
		if q := s.opsHooks().Library; q != nil {
			return q, nil
		}
		return nil, unavailable("the episode library is disabled")
	}
	libErr := func(err error) error {
		switch {
		case errors.Is(err, library.ErrNotFound):
			return notFound("library copy")
		case errors.Is(err, library.ErrNotRequeueable):
			return conflict("not_requeueable", "only an original copy whose encode was abandoned can be put back in the queue")
		}
		return err
	}
	return Action{
		Name: "requeue_av1", Level: ActionReversible, Scope: ScopeOpsWrite, Implemented: true,
		Summary: "Put a copy whose AV1 encode was abandoned back in the encode queue",
		Validate: func(args map[string]any) error {
			_, err := parse(args)
			return err
		},
		Plan: func(_ context.Context, args map[string]any) (Plan, error) {
			k, err := parse(args)
			if err != nil {
				return Plan{}, err
			}
			q, err := queue()
			if err != nil {
				return Plan{}, err
			}
			st, ok, err := q.EncodeQueue(k)
			if err != nil {
				return Plan{}, libErr(err)
			}
			if !ok {
				return Plan{}, libErr(library.ErrNotRequeueable)
			}
			return Plan{Summary: "Put " + k.String() + " back in the AV1 queue (" + strconv.Itoa(st.Attempts) + " failed attempts so far)",
				Details: map[string]any{"copy": k.String(), "attempts": st.Attempts, "last_error": st.LastError}}, nil
		},
		Do: func(_ context.Context, args map[string]any) (Result, string, error) {
			k, err := parse(args)
			if err != nil {
				return nil, "", err
			}
			q, err := queue()
			if err != nil {
				return nil, "", err
			}
			prev, err := q.RequeueEncode(k)
			if err != nil {
				return nil, "", libErr(err)
			}
			b, _ := json.Marshal(av1Undo{Key: k, Prev: prev})
			return Result{"copy": k.String(), "queued": true}, string(b), nil
		},
		Undo: func(_ context.Context, token string) error {
			var u av1Undo
			if json.Unmarshal([]byte(token), &u) != nil || u.Key.SeasonID <= 0 {
				return badArgs("invalid undo token")
			}
			q, err := queue()
			if err != nil {
				return err
			}
			if err := q.RestoreEncodeQueue(u.Key, u.Prev); err != nil {
				if errors.Is(err, library.ErrNotRequeueable) {
					return conflict("already_encoding", "the encoder already took this copy")
				}
				return libErr(err)
			}
			return nil
		},
	}
}

type av1Undo struct {
	Key  library.Key              `json:"key"`
	Prev library.EncodeQueueState `json:"prev"`
}

// ---- concurrent streams ---------------------------------------------------------------------

// StreamLimit returns the maximum number of concurrent playback sessions (0: no limit). It fails
// open: when the setting cannot be read, playback is not limited.
func (s *Service) StreamLimit(ctx context.Context) int {
	if s == nil {
		return 0
	}
	v, ok, err := s.getSetting(ctx, settingStreamLimit)
	if err != nil || !ok {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func (s *Service) actLimitStreams() Action {
	parse := func(args map[string]any) (int, error) {
		if err := onlyKeys(args, "max_streams"); err != nil {
			return 0, err
		}
		n, _, err := intArg(args, "max_streams", true, 0, maxStreamLimit)
		return n, err
	}
	stats := func() (PlaybackStats, error) {
		if st := s.pbDeps().stats; st != nil {
			return st, nil
		}
		return nil, unavailable("concurrent sessions are only counted by the HLS playback engine")
	}
	return Action{
		Name: "limit_concurrent_streams", Level: ActionSensitive, Scope: ScopeConfigWrite, Implemented: true,
		Summary: "Cap the concurrent playback sessions (0 lifts the cap); sessions already open keep playing",
		Validate: func(args map[string]any) error {
			_, err := parse(args)
			return err
		},
		Plan: func(ctx context.Context, args map[string]any) (Plan, error) {
			n, err := parse(args)
			if err != nil {
				return Plan{}, err
			}
			st, err := stats()
			if err != nil {
				return Plan{}, err
			}
			cur, active := s.StreamLimit(ctx), st.ActiveSessions()
			summary := "Limit concurrent streams to " + strconv.Itoa(n) + " (" + strconv.Itoa(active) + " open now)"
			if n == 0 {
				summary = "Lift the concurrent stream limit"
			}
			return Plan{Summary: summary, Details: map[string]any{"from": cur, "to": n, "active": active}}, nil
		},
		Do: func(ctx context.Context, args map[string]any) (Result, string, error) {
			n, err := parse(args)
			if err != nil {
				return nil, "", err
			}
			if _, err := stats(); err != nil {
				return nil, "", err
			}
			undo, err := s.replaceSetting(ctx, settingStreamLimit, strconv.Itoa(n))
			if err != nil {
				return nil, "", err
			}
			return Result{"max_streams": n}, undo, nil
		},
		Undo: s.undoSetting(settingStreamLimit),
	}
}

// ---- maintenance --------------------------------------------------------------------------

// MaintenanceWindow is a scheduled maintenance, shown on the public status page; the watch rules
// send no notification while it runs.
type MaintenanceWindow struct {
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
	Message  string    `json:"message,omitempty"`
}

// Maintenance returns the scheduled window when it has not ended yet.
func (s *Service) Maintenance(ctx context.Context) (MaintenanceWindow, bool) {
	var m MaintenanceWindow
	v, ok, err := s.getSetting(ctx, settingMaintenance)
	if err != nil || !ok || json.Unmarshal([]byte(v), &m) != nil || !s.now().Before(m.EndsAt) {
		return MaintenanceWindow{}, false
	}
	return m, true
}

// inMaintenance reports whether a maintenance window covers now.
func (s *Service) inMaintenance(ctx context.Context, now time.Time) bool {
	m, ok := s.Maintenance(ctx)
	return ok && !now.Before(m.StartsAt)
}

func (s *Service) actScheduleMaintenance() Action {
	parse := func(args map[string]any) (MaintenanceWindow, error) {
		var m MaintenanceWindow
		if err := onlyKeys(args, "starts_at", "ends_at", "message"); err != nil {
			return m, err
		}
		now := s.now()
		parseTime := func(name string, required bool) (time.Time, error) {
			v, err := strArg(args, name, required, 40)
			if err != nil || v == "" {
				return time.Time{}, err
			}
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				return time.Time{}, badArgs("argument %q must be an RFC 3339 time (2026-10-07T22:00:00Z)", name)
			}
			return t.UTC(), nil
		}
		var err error
		if m.StartsAt, err = parseTime("starts_at", false); err != nil {
			return m, err
		}
		if m.StartsAt.IsZero() {
			m.StartsAt = now.UTC().Truncate(time.Second)
		}
		if m.EndsAt, err = parseTime("ends_at", true); err != nil {
			return m, err
		}
		if m.Message, err = strArg(args, "message", false, maintenanceMaxMsg); err != nil {
			return m, err
		}
		if strings.ContainsAny(m.Message, "\r\n<>") {
			return m, badArgs("message must be one line of plain text")
		}
		switch {
		case !m.EndsAt.After(now):
			return m, badArgs("ends_at must be in the future")
		case !m.EndsAt.After(m.StartsAt):
			return m, badArgs("ends_at must be after starts_at")
		case m.EndsAt.Sub(m.StartsAt) > maintenanceMaxLength:
			return m, badArgs("a maintenance window lasts at most 24 hours")
		case m.StartsAt.After(now.Add(maintenanceMaxAhead)):
			return m, badArgs("a maintenance window starts within 30 days")
		}
		return m, nil
	}
	return Action{
		Name: "schedule_maintenance", Level: ActionSensitive, Scope: ScopeConfigWrite, Implemented: true,
		Summary: "Schedule a maintenance window (at most 24 h, within 30 days): announced on the public status page, watch notifications muted while it runs",
		Validate: func(args map[string]any) error {
			_, err := parse(args)
			return err
		},
		Plan: func(ctx context.Context, args map[string]any) (Plan, error) {
			m, err := parse(args)
			if err != nil {
				return Plan{}, err
			}
			d := map[string]any{"starts_at": m.StartsAt.Format(time.RFC3339), "ends_at": m.EndsAt.Format(time.RFC3339), "message": m.Message}
			if cur, ok := s.Maintenance(ctx); ok {
				d["replaces"] = cur
			}
			return Plan{Summary: "Announce a maintenance from " + m.StartsAt.Format(time.RFC3339) + " to " + m.EndsAt.Format(time.RFC3339), Details: d}, nil
		},
		Do: func(ctx context.Context, args map[string]any) (Result, string, error) {
			m, err := parse(args)
			if err != nil {
				return nil, "", err
			}
			b, _ := json.Marshal(m)
			undo, err := s.replaceSetting(ctx, settingMaintenance, string(b))
			if err != nil {
				return nil, "", err
			}
			return Result{"maintenance": m}, undo, nil
		},
		Undo: s.undoSetting(settingMaintenance),
	}
}
