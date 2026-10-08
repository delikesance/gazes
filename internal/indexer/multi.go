package indexer

import (
	"context"
	"errors"
	"fmt"
	"github.com/gazes/gazes/internal/diagnostics"
	"github.com/gazes/gazes/internal/kv"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"
)

type PartialError struct {
	Providers []string
	AllFailed bool
	Causes    map[string]string
}

func (e *PartialError) Error() string {
	return "unavailable indexers: " + strings.Join(e.Providers, ", ")
}

type cachedResult struct {
	items   []TorrentItem
	expires time.Time
}
type providerState struct {
	provider Provider
	// slots bounds in-flight calls: 2 by default, Pacing.MaxConcurrent (1 when only an interval is set) for paced providers.
	slots chan struct{}
	pace  Pacing
	// paceMu guards nextStart, the earliest start of the next call (MinInterval after the previous start).
	paceMu    sync.Mutex
	nextStart time.Time
	// callTimeout bounds the provider's own answer. It starts once the pacing slot is held, so queueing never eats it.
	callTimeout time.Duration
	// maxQueueWait caps a paced call's wait for its slot (see callFor).
	maxQueueWait time.Duration
	mu           sync.Mutex
	cache        map[string]cachedResult
	pending      map[string]*flight
	// gov is the circuit breaker (see kv.Governor.Admit): in-process by default, shared across
	// instances once SetRedis is called.
	gov    *kv.Governor
	rcache *kv.Cache[idxResult]
}

// flight is one in-progress upstream call that identical concurrent queries wait on.
type flight struct {
	done chan struct{}
	err  error
}

var errCoolingDown = errors.New("provider cooling down")

// idxResult is what the shared cache stores for one provider query.
type idxResult struct {
	Items []TorrentItem `json:"items"`
}

// MultiProvider keeps provider latency, parallelism and memory bounded independently.
type MultiProvider struct{ states []*providerState }

// NewMultiProvider wraps the providers. A provider implementing PacedProvider is called
// one request at a time (or Pacing.MaxConcurrent) with Pacing.MinInterval between call starts.
func NewMultiProvider(providers ...Provider) *MultiProvider {
	m := &MultiProvider{}
	for _, p := range providers {
		capacity := 2
		var pace Pacing
		if pp, ok := p.(PacedProvider); ok {
			pace = pp.Pacing()
		}
		if pace.MaxConcurrent > 0 {
			capacity = pace.MaxConcurrent
		} else if pace.MinInterval > 0 {
			capacity = 1
		}
		m.states = append(m.states, &providerState{provider: p, slots: make(chan struct{}, capacity), pace: pace, callTimeout: 4 * time.Second, maxQueueWait: maxQueueWait,
			cache: map[string]cachedResult{}, pending: map[string]*flight{}, gov: kv.NewLocalGovernor("idx:" + p.Name())})
	}
	return m
}

// Paced reports whether the named provider is rate-paced (its calls are serialized or spaced, so
// a burst of queries is served slowly). The resolver uses it to budget how many queries it sends.
func (m *MultiProvider) Paced(name string) bool {
	for _, s := range m.states {
		if s.provider.Name() == name {
			return !s.pace.IsZero()
		}
	}
	return false
}

// PacedLimits lists the pacing of every rate-paced provider (empty when none is paced).
func (m *MultiProvider) PacedLimits() []Pacing {
	var out []Pacing
	for _, s := range m.states {
		if !s.pace.IsZero() {
			out = append(out, s.pace)
		}
	}
	return out
}

// Pacing returns the pacing of the named provider (zero when unpaced or unknown).
func (m *MultiProvider) Pacing(name string) Pacing {
	for _, s := range m.states {
		if s.provider.Name() == name {
			return s.pace
		}
	}
	return Pacing{}
}

// SetRedis shares provider results, single-flight and the circuit breaker (kv.HealthPolicy) across instances,
// so several backends do not multiply the load on the trackers.
func (m *MultiProvider) SetRedis(c *kv.Client) {
	for _, s := range m.states {
		s.rcache = kv.NewCache[idxResult](c, "idx:"+s.provider.Name(), kv.CacheOptions{L1TTL: 10 * time.Second, L1Max: 256, FetchTimeout: 25 * time.Second, WaitBudget: 25 * time.Second})
		s.gov = c.NewGovernor("idx:"+s.provider.Name(), 600, 600, 0)
	}
}

// ProviderNames lists the wrapped providers, in their configured order.
func (m *MultiProvider) ProviderNames() []string {
	out := make([]string, 0, len(m.states))
	for _, s := range m.states {
		out = append(out, s.provider.Name())
	}
	return out
}

func (m *MultiProvider) state(name string) *providerState {
	for _, s := range m.states {
		if s.provider.Name() == name {
			return s
		}
	}
	return nil
}

// ProviderHealth is the operational state of one provider.
type ProviderHealth struct {
	Paused   time.Duration // remaining deliberate pause (Pause), 0 when none
	Cooldown time.Duration // remaining circuit-breaker open time, 0 when closed
}

// Health reports the pause and breaker state of the named provider (false when unknown).
func (m *MultiProvider) Health(ctx context.Context, name string) (ProviderHealth, bool) {
	s := m.state(name)
	if s == nil {
		return ProviderHealth{}, false
	}
	return ProviderHealth{Paused: s.gov.Blocked(ctx), Cooldown: s.gov.BreakerOpen(ctx)}, true
}

// Pause stops calling the named provider for d, on every instance sharing Redis. Its queries fail
// at once as a cooling-down provider, so resolutions use the other providers.
func (m *MultiProvider) Pause(ctx context.Context, name string, d time.Duration) bool {
	s := m.state(name)
	if s == nil {
		return false
	}
	s.gov.Penalize(ctx, d)
	return true
}

// Resume lifts a Pause of the named provider.
func (m *MultiProvider) Resume(ctx context.Context, name string) bool {
	s := m.state(name)
	if s == nil {
		return false
	}
	s.gov.ClearCooldown(ctx)
	return true
}

// Retry closes the circuit breaker of the named provider, so its next query is sent instead of
// waiting for the backoff. A pause is left alone.
func (m *MultiProvider) Retry(ctx context.Context, name string) bool {
	s := m.state(name)
	if s == nil {
		return false
	}
	s.gov.Success(ctx)
	return true
}

// PurgeCache drops the shared query results of every provider and returns how many were removed.
func (m *MultiProvider) PurgeCache(ctx context.Context) (int, error) {
	n := 0
	for _, s := range m.states {
		s.mu.Lock()
		clear(s.cache)
		s.mu.Unlock()
		if s.rcache == nil {
			continue
		}
		k, err := s.rcache.Purge(ctx)
		n += k
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

func (m *MultiProvider) Name() string { return "public indexers" }
func (m *MultiProvider) Search(ctx context.Context, o SearchOptions) ([]TorrentItem, error) {
	return m.run(ctx, o, false)
}
func (m *MultiProvider) GetLatest(ctx context.Context, category string, page int) ([]TorrentItem, error) {
	return m.run(ctx, SearchOptions{Category: category, Page: page}, true)
}
func (m *MultiProvider) run(ctx context.Context, o SearchOptions, latest bool) ([]TorrentItem, error) {
	type result struct {
		items []TorrentItem
		name  string
		err   error
	}
	active := make([]*providerState, 0, len(m.states))
	for _, s := range m.states {
		paced := !s.pace.IsZero()
		if (o.Scope == ScopeUnpaced && paced) || (o.Scope == ScopePaced && !paced) {
			continue
		}
		active = append(active, s)
	}
	if len(active) == 0 {
		return nil, nil
	}
	ch := make(chan result, len(active))
	remaining := map[string]int{}
	for _, s := range active {
		remaining[s.provider.Name()]++
		go func(s *providerState) {
			items, err := s.search(ctx, o, latest)
			ch <- result{items, s.provider.Name(), err}
		}(s)
	}
	unique := map[string]TorrentItem{}
	failed := []string{}
	causes := map[string]string{}
	successes := 0
collect:
	for range active {
		select {
		case <-ctx.Done():
			// A slow provider must not erase sources already returned by healthy ones.
			if !errors.Is(ctx.Err(), context.DeadlineExceeded) || len(unique) == 0 {
				return nil, ctx.Err()
			}
			for name, count := range remaining {
				if count > 0 {
					failed = append(failed, name)
					causes[name] = ctx.Err().Error()
				}
			}
			break collect
		case r := <-ch:
			remaining[r.name]--
			if r.err != nil {
				failed = append(failed, r.name)
				causes[r.name] = diagnostics.Redact(r.err.Error())
			} else {
				successes++
			}
			for _, item := range r.items {
				h := strings.ToLower(strings.TrimSpace(item.InfoHash))
				if h == "" {
					continue
				}
				item.InfoHash = h
				if item.Provider == "" {
					item.Provider = r.name
				}
				old, ok := unique[h]
				if !ok || item.Seeders > old.Seeders || (item.Seeders == old.Seeders && item.Provider < old.Provider) {
					unique[h] = item
				}
			}
		}
	}
	items := make([]TorrentItem, 0, len(unique))
	for _, item := range unique {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Seeders != items[j].Seeders {
			return items[i].Seeders > items[j].Seeders
		}
		return items[i].InfoHash < items[j].InfoHash
	})
	if len(failed) > 0 {
		sort.Strings(failed)
		return items, &PartialError{Providers: failed, AllFailed: successes == 0, Causes: causes}
	}
	return items, nil
}
func (s *providerState) search(ctx context.Context, o SearchOptions, latest bool) (out []TorrentItem, returnedErr error) {
	started := time.Now()
	diagnostics.Log(ctx, slog.LevelDebug, "provider.started", "provider", s.provider.Name(), "query", o.Query, "category", o.Category, "page", o.Page)
	defer func() {
		level := slog.LevelDebug
		if returnedErr != nil {
			level = slog.LevelWarn
		}
		diagnostics.Log(ctx, level, "provider.completed", "provider", s.provider.Name(), "query", o.Query, "category", o.Category, "page", o.Page, "count", len(out), "duration_ms", time.Since(started).Milliseconds(), "err", returnedErr)
	}()

	keyOptions := o
	if s.provider.Name() != "nyaa.si" {
		keyOptions.Category = ""
		keyOptions.SortBy = ""
		keyOptions.Order = ""
	}
	key := fmt.Sprintf("%t:%s:%s:%d:%s:%s", latest, strings.ToLower(strings.Join(strings.Fields(o.Query), " ")), keyOptions.Category, o.Page, keyOptions.SortBy, keyOptions.Order)
	if s.provider.Name() == "nyaa.si" && (o.SortBy == "seeders" || o.Page > 1) {
		key = "listing-v2:" + key
	}
	if s.rcache != nil {
		return s.searchShared(ctx, o, latest, key, started)
	}
	for {
		s.mu.Lock()
		if c, ok := s.cache[key]; ok && time.Now().Before(c.expires) {
			items := append([]TorrentItem(nil), c.items...)
			s.mu.Unlock()
			diagnostics.Log(ctx, slog.LevelDebug, "provider.cache_hit", "provider", s.provider.Name(), "count", len(items))
			return items, nil
		}
		if f, ok := s.pending[key]; ok {
			s.mu.Unlock()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-f.done:
				diagnostics.Log(ctx, slog.LevelDebug, "provider.coalesced", "provider", s.provider.Name())
				// An identical query shares the leader's failure instead of hitting the upstream again
				// (which would also count the same outage several times); the leader giving up does not count.
				if f.err != nil && !errors.Is(f.err, context.Canceled) && !errors.Is(f.err, context.DeadlineExceeded) {
					return nil, f.err
				}
				continue
			}
		}
		f := &flight{done: make(chan struct{})}
		s.pending[key] = f
		s.mu.Unlock()
		items, err := s.call(ctx, o, latest, started)
		s.mu.Lock()
		if err == nil {
			for k, c := range s.cache {
				if time.Now().After(c.expires) {
					delete(s.cache, k)
				}
			}
			if len(s.cache) >= 256 {
				for k := range s.cache {
					delete(s.cache, k)
					break
				}
			}
			ttl := 2 * time.Minute
			if len(items) == 0 {
				ttl = 15 * time.Second
			}
			s.cache[key] = cachedResult{append([]TorrentItem(nil), items...), time.Now().Add(ttl)}
		}
		f.err = err
		delete(s.pending, key)
		close(f.done)
		s.mu.Unlock()
		return items, err
	}
}

// maxQueueWait bounds how long a paced provider's call may wait for its slot and pacing interval.
// Past it the resolver that asked is long gone, and sending the query late would only delay the
// next resolve's own queries.
const maxQueueWait = 8 * time.Second

var errBusy = errors.New("provider busy")

// claim records a pacing-clock reservation so it can be handed back if the request never goes out.
type claim struct {
	prev, at time.Time
	set      bool
}

// acquire takes a call slot and, for a paced provider, waits until MinInterval has passed since the
// previous call started, then claims the next interval. ctx bounds the whole wait; a caller that
// gives up releases the slot without moving the pacing clock. With nowait (background refreshes) it
// returns errBusy instead of waiting for the slot or the interval. The caller must release() the claim.
func (s *providerState) acquire(ctx context.Context, nowait bool) (claim, error) {
	if nowait {
		select {
		case s.slots <- struct{}{}:
		default:
			return claim{}, errBusy
		}
	} else {
		select {
		case <-ctx.Done():
			return claim{}, ctx.Err()
		case s.slots <- struct{}{}:
		}
	}
	if ctx.Err() != nil {
		<-s.slots
		return claim{}, ctx.Err()
	}
	for s.pace.MinInterval > 0 {
		s.paceMu.Lock()
		now := time.Now()
		wait := s.nextStart.Sub(now)
		if wait <= 0 {
			c := claim{prev: s.nextStart, at: now.Add(s.pace.MinInterval), set: true}
			s.nextStart = c.at
			s.paceMu.Unlock()
			return c, nil
		}
		s.paceMu.Unlock()
		if nowait {
			<-s.slots
			return claim{}, errBusy
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			<-s.slots
			return claim{}, ctx.Err()
		case <-timer.C:
		}
	}
	return claim{}, nil
}

// release frees the slot. A claim whose request never went out (circuit opened meanwhile, caller left)
// gives its interval back, unless someone else has claimed the clock since.
func (s *providerState) release(c claim, sent bool) {
	if c.set && !sent {
		s.paceMu.Lock()
		if s.nextStart.Equal(c.at) {
			s.nextStart = c.prev
		}
		s.paceMu.Unlock()
	}
	<-s.slots
}

// call performs one upstream request for the caller whose context is ctx.
func (s *providerState) call(ctx context.Context, o SearchOptions, latest bool, started time.Time) ([]TorrentItem, error) {
	return s.callFor(ctx, ctx, o, latest, started)
}

// callFor performs one upstream request under the circuit breaker and the pacing limits. ctx runs
// the request (on the shared path it is detached from the caller); caller is who is waiting for it.
// Waiting for a slot is bounded by ctx; the provider's own timeout (callTimeout) starts only once the
// slot is held. For a paced provider the wait also ends when caller gives up or after maxQueueWait,
// so an abandoned query is never sent late, and a background refresh (kv.Background) never waits at
// all. The outcome is always reported to the breaker and logged, because on the shared path nothing
// else would show what tripped a cooldown.
func (s *providerState) callFor(caller, ctx context.Context, o SearchOptions, latest bool, started time.Time) ([]TorrentItem, error) {
	name := s.provider.Name()
	paced := !s.pace.IsZero()
	// Reports must survive the cancellation of the call's own context.
	report := context.WithoutCancel(ctx)
	if paused := s.gov.Blocked(report); paused > 0 {
		diagnostics.Log(caller, slog.LevelDebug, "provider.paused", "provider", name, "remaining_ms", paused.Milliseconds())
		return nil, errCoolingDown
	}
	adm := s.gov.Admit(report)
	if !adm.Allowed {
		diagnostics.Log(caller, slog.LevelDebug, "provider.cooldown", "provider", name, "remaining_ms", adm.Remaining.Milliseconds())
		return nil, errCoolingDown
	}
	waitCtx := ctx
	if paced {
		var cancelQueue, cancelCaller context.CancelFunc
		waitCtx, cancelQueue = context.WithTimeout(ctx, s.maxQueueWait)
		defer cancelQueue()
		waitCtx, cancelCaller = context.WithCancel(waitCtx)
		defer cancelCaller()
		if caller != ctx {
			defer context.AfterFunc(caller, cancelCaller)()
		}
	}
	c, err := s.acquire(waitCtx, paced && kv.IsBackground(ctx))
	if err != nil {
		s.gov.Neutral(report, adm.Probe)
		return nil, err
	}
	sent := false
	defer func() { s.release(c, sent) }()
	if paced {
		// Calls that queued behind a failing one must not follow it into an open circuit.
		if remaining := s.gov.Cooldown(report); remaining > 0 && !adm.Probe {
			diagnostics.Log(caller, slog.LevelDebug, "provider.cooldown", "provider", name, "remaining_ms", remaining.Milliseconds())
			return nil, errCoolingDown
		}
		if waitCtx.Err() != nil { // the caller left between the claim and the send
			s.gov.Neutral(report, adm.Probe)
			return nil, waitCtx.Err()
		}
	}
	sent = true
	callCtx, cancel := context.WithTimeout(ctx, s.callTimeout)
	defer cancel()
	callStarted := time.Now()
	diagnostics.Log(caller, slog.LevelDebug, "provider.slot_acquired", "provider", name, "wait_ms", callStarted.Sub(started).Milliseconds())
	var items []TorrentItem
	if latest {
		items, err = s.provider.GetLatest(callCtx, o.Category, o.Page)
	} else {
		items, err = s.provider.Search(callCtx, o)
	}
	s.report(caller, report, err, ctx.Err() != nil, ctx.Err() == nil && callCtx.Err() != nil, adm.Probe, time.Since(callStarted), len(items))
	return items, err
}

// report feeds one call's result to the circuit breaker and logs the outcome.
//
//	success (even with zero results)   -> closes the breaker, resets the backoff
//	caller cancelled / gave up         -> neutral: says nothing about the provider
//	our own call timeout               -> neutral for a paced provider (its queue and delay are
//	                                      expected to be slow); for an unpaced one a soft failure,
//	                                      so a tracker that blackholes requests still opens the circuit
//	HTTP 429, 5xx, 401/403             -> open at once (Retry-After honoured)
//	anything else                      -> opens after kv.HealthPolicy.Threshold failures in the window
func (s *providerState) report(logCtx, ctx context.Context, err error, callerGone, ownTimeout, probe bool, took time.Duration, count int) {
	attrs := []any{"provider", s.provider.Name(), "duration_ms", took.Milliseconds(), "count", count, "probe", probe}
	if err == nil {
		s.gov.Success(ctx)
		diagnostics.Log(logCtx, slog.LevelDebug, "provider.outcome", append(attrs, "class", "ok")...)
		return
	}
	class, status, immediate, retryAfter := classify(err)
	paced := !s.pace.IsZero()
	neutral := callerGone || errors.Is(err, context.Canceled) || (ownTimeout && paced)
	if class == "error" && (ownTimeout || errors.Is(err, context.DeadlineExceeded) || callerGone) {
		class = "timeout"
	}
	attrs = append(attrs, "class", class, "err", err)
	if status != 0 {
		attrs = append(attrs, "status", status)
	}
	if neutral {
		s.gov.Neutral(ctx, probe)
		diagnostics.Log(logCtx, slog.LevelWarn, "provider.outcome", append(attrs, "penalized", false)...)
		return
	}
	cooldown := s.gov.Failure(ctx, immediate, retryAfter)
	diagnostics.Log(logCtx, slog.LevelWarn, "provider.outcome", append(attrs, "penalized", cooldown > 0, "cooldown_ms", cooldown.Milliseconds())...)
}

// classify maps a provider error to a class, its HTTP status (0 when none) and whether it opens the
// circuit immediately, with the upstream's Retry-After.
func classify(err error) (class string, status int, immediate bool, retryAfter time.Duration) {
	var he *HTTPError
	if !errors.As(err, &he) {
		return "error", 0, false, 0
	}
	switch {
	case he.Status == 429:
		return "http_429", he.Status, true, he.RetryAfter
	case he.Status >= 500:
		return "http_5xx", he.Status, true, he.RetryAfter
	case he.Status == 401 || he.Status == 403:
		return "auth", he.Status, true, he.RetryAfter
	}
	return "http_4xx", he.Status, false, 0
}

// searchShared is search() on Redis: one fetch per query across the whole fleet, results kept for
// 2 min (15 s when empty), stale answers (up to 10 min) stand in while a refresh runs or the
// provider is cooling down.
func (s *providerState) searchShared(ctx context.Context, o SearchOptions, latest bool, key string, started time.Time) ([]TorrentItem, error) {
	policy := kv.Policy[idxResult]{
		TTLFor: func(r *idxResult) time.Duration {
			if len(r.Items) == 0 {
				return 15 * time.Second
			}
			return 2 * time.Minute
		},
		StaleFor: func(r *idxResult) time.Duration {
			if len(r.Items) == 0 {
				return 0
			}
			return 10 * time.Minute
		},
	}
	res, err := s.rcache.Get(ctx, key, policy, func(fetchCtx context.Context) (*idxResult, error) {
		items, err := s.callFor(ctx, fetchCtx, o, latest, started)
		if err != nil {
			return nil, err
		}
		return &idxResult{Items: items}, nil
	})
	if err != nil {
		return nil, err
	}
	return append([]TorrentItem(nil), res.Items...), nil
}
