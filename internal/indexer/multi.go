package indexer

import (
	"context"
	"fmt"
	"github.com/gazes/gazes/internal/diagnostics"
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
	slots    chan struct{}
	mu       sync.Mutex
	cache    map[string]cachedResult
	pending  map[string]chan struct{}
	retry    time.Time
}

// MultiProvider keeps provider latency, parallelism and memory bounded independently.
type MultiProvider struct{ states []*providerState }

func NewMultiProvider(providers ...Provider) *MultiProvider {
	m := &MultiProvider{}
	for _, p := range providers {
		m.states = append(m.states, &providerState{provider: p, slots: make(chan struct{}, 2), cache: map[string]cachedResult{}, pending: map[string]chan struct{}{}})
	}
	return m
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
	ch := make(chan result, len(m.states))
	for _, s := range m.states {
		go func(s *providerState) {
			items, err := s.search(ctx, o, latest)
			ch <- result{items, s.provider.Name(), err}
		}(s)
	}
	unique := map[string]TorrentItem{}
	failed := []string{}
	causes := map[string]string{}
	successes := 0
	for range m.states {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case r := <-ch:
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
	for {
		s.mu.Lock()
		if c, ok := s.cache[key]; ok && time.Now().Before(c.expires) {
			items := append([]TorrentItem(nil), c.items...)
			s.mu.Unlock()
			diagnostics.Log(ctx, slog.LevelDebug, "provider.cache_hit", "provider", s.provider.Name(), "count", len(items))
			return items, nil
		}
		if time.Now().Before(s.retry) {
			s.mu.Unlock()
			diagnostics.Log(ctx, slog.LevelDebug, "provider.cooldown", "provider", s.provider.Name())
			return nil, fmt.Errorf("provider cooling down")
		}
		if wait, ok := s.pending[key]; ok {
			s.mu.Unlock()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-wait:
				diagnostics.Log(ctx, slog.LevelDebug, "provider.coalesced", "provider", s.provider.Name())
				continue
			}
		}
		done := make(chan struct{})
		s.pending[key] = done
		s.mu.Unlock()
		var items []TorrentItem
		var err error
		attempted := false
		// Waiting is bounded by the resolver deadline, not the network budget.
		// Otherwise a healthy provider can receive an already-expiring request.
		var callCtx context.Context
		cancel := func() {}
		select {
		case <-ctx.Done():
			err = ctx.Err()
		case s.slots <- struct{}{}:
			if ctx.Err() != nil {
				<-s.slots
				err = ctx.Err()
				break
			}
			callCtx, cancel = context.WithTimeout(ctx, 4*time.Second)
			attempted = true
			diagnostics.Log(ctx, slog.LevelDebug, "provider.slot_acquired", "provider", s.provider.Name(), "wait_ms", time.Since(started).Milliseconds())
			if latest {
				items, err = s.provider.GetLatest(callCtx, o.Category, o.Page)
			} else {
				items, err = s.provider.Search(callCtx, o)
			}
			<-s.slots
		}
		cancel()
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
		} else if ctx.Err() == nil && attempted {
			s.retry = time.Now().Add(30 * time.Second)
		}
		delete(s.pending, key)
		close(done)
		s.mu.Unlock()
		return items, err
	}
}
