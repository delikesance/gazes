package api

import (
	"sync/atomic"
	"time"

	"context"

	"github.com/gazes/gazes/internal/indexer"
	"github.com/gazes/gazes/internal/kv"
)

// Resolving an episode searches several trackers and takes up to 20 s. The answer barely changes
// within minutes, so findings are remembered in Redis (shared by every instance) and identical
// requests in flight, across the whole fleet, share one resolution.
const (
	sourcesTTLComplete = 15 * time.Minute
	sourcesTTLPartial  = 5 * time.Minute  // some providers failed: usable, but look again soon
	sourcesTTLEmpty    = 30 * time.Second // nothing found yet: do not hammer the trackers
	sourcesStaleOK     = time.Hour        // a complete list stays a useful stand-in while it refreshes
	sourcesWorkBudget  = 40 * time.Second
)

type sourceCache struct {
	cache *kv.Cache[indexer.EpisodeSourcesResponse]
}

// newSourceCache uses Redis when rc is set, process memory otherwise (tests).
func newSourceCache(rc *kv.Client) *sourceCache {
	return &sourceCache{cache: kv.NewCache[indexer.EpisodeSourcesResponse](rc, "sources", kv.CacheOptions{
		L1TTL:        20 * time.Second,
		L1Max:        512,
		FetchTimeout: sourcesWorkBudget,
		WaitBudget:   sourcesWorkBudget + 5*time.Second,
	})}
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

// sourcesPolicy never serves an empty or partial answer stale: those mean "look again", not "no".
var sourcesPolicy = kv.Policy[indexer.EpisodeSourcesResponse]{
	TTLFor: sourcesTTL,
	StaleFor: func(res *indexer.EpisodeSourcesResponse) time.Duration {
		if len(res.Sources) == 0 || res.Partial {
			return 0
		}
		return sourcesStaleOK
	},
}

// resolve returns the cached findings for key or runs fn once for all callers on all instances.
// hit reports that fn did not run for this call.
func (c *sourceCache) resolve(ctx context.Context, key string, fn func(context.Context) (*indexer.EpisodeSourcesResponse, error)) (*indexer.EpisodeSourcesResponse, bool, error) {
	var ran atomic.Bool
	res, err := c.cache.Get(ctx, key, sourcesPolicy, func(ctx context.Context) (*indexer.EpisodeSourcesResponse, error) {
		ran.Store(true)
		return fn(ctx)
	})
	if err != nil {
		return nil, false, err
	}
	return res, !ran.Load(), nil
}
