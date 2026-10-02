package api

import (
	"context"
	"sync"
	"time"

	"github.com/gazes/gazes/internal/indexer"
	"golang.org/x/sync/singleflight"
)

// Resolving an episode searches several trackers and takes up to 20 s. The
// answer barely changes within minutes, so complete findings are remembered
// and identical requests in flight share one resolution.
const (
	sourcesTTLComplete = 15 * time.Minute
	sourcesTTLPartial  = 5 * time.Minute  // some providers failed: usable, but look again soon
	sourcesTTLEmpty    = 30 * time.Second // nothing found yet: do not hammer the trackers
	sourcesMaxEntries  = 512
	sourcesWorkBudget  = 40 * time.Second
)

type cachedSources struct {
	res     *indexer.EpisodeSourcesResponse
	expires time.Time
}

type sourceCache struct {
	mu      sync.Mutex
	entries map[string]cachedSources
	group   singleflight.Group
	now     func() time.Time
}

func newSourceCache() *sourceCache {
	return &sourceCache{entries: map[string]cachedSources{}, now: time.Now}
}

func sourcesTTL(res *indexer.EpisodeSourcesResponse) time.Duration {
	switch {
	case len(res.Sources) == 0:
		return sourcesTTLEmpty
	case res.Partial:
		return sourcesTTLPartial
	default:
		return sourcesTTLComplete
	}
}

// resolve returns the cached findings for key or runs fn once for all callers.
// The shared work is detached from any single caller's cancellation (one viewer
// leaving must not fail the others) but each caller still honours its own ctx.
func (c *sourceCache) resolve(ctx context.Context, key string, fn func(context.Context) (*indexer.EpisodeSourcesResponse, error)) (*indexer.EpisodeSourcesResponse, bool, error) {
	c.mu.Lock()
	if entry, ok := c.entries[key]; ok && c.now().Before(entry.expires) {
		c.mu.Unlock()
		return entry.res, true, nil
	}
	c.mu.Unlock()

	work := context.WithoutCancel(ctx)
	ch := c.group.DoChan(key, func() (any, error) {
		workCtx, cancel := context.WithTimeout(work, sourcesWorkBudget)
		defer cancel()
		res, err := fn(workCtx)
		if err != nil {
			return nil, err
		}
		c.mu.Lock()
		if len(c.entries) >= sourcesMaxEntries {
			clear(c.entries)
		}
		c.entries[key] = cachedSources{res: res, expires: c.now().Add(sourcesTTL(res))}
		c.mu.Unlock()
		return res, nil
	})
	select {
	case <-ctx.Done():
		return nil, false, ctx.Err()
	case result := <-ch:
		if result.Err != nil {
			return nil, false, result.Err
		}
		return result.Val.(*indexer.EpisodeSourcesResponse), false, nil
	}
}
