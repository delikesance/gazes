package kv

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// LimitedError means an upstream is throttling the whole fleet; RetryAfter says when to try again.
type LimitedError struct {
	Upstream   string
	RetryAfter time.Duration
}

func (e *LimitedError) Error() string {
	return fmt.Sprintf("%s rate limited, retry in %s", e.Upstream, e.RetryAfter.Round(time.Second))
}

// Governor spends tokens from a bucket shared by every instance and honours a shared cooldown, so
// N backends behave like one polite client of the upstream.
type Governor struct {
	c         *Client
	name      string
	perMinute int
	burst     int
	maxWait   time.Duration

	// Memory-only mode (nil client): the cooldown is local to the process, there is no token bucket.
	mu           sync.Mutex
	localBlocked time.Time
}

// NewGovernor allows perMinute sustained calls with bursts up to burst. Waits longer than maxWait
// are returned as a LimitedError instead of blocking the request.
func (c *Client) NewGovernor(name string, perMinute, burst int, maxWait time.Duration) *Governor {
	return &Governor{c: c, name: name, perMinute: perMinute, burst: burst, maxWait: maxWait}
}

// bucket: KEYS[1]=state hash; ARGV: rate per ms, burst. Redis TIME keeps every instance on one clock.
var bucketScript = redis.NewScript(`
local t = redis.call('TIME')
local now = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000)
local rate = tonumber(ARGV[1])
local burst = tonumber(ARGV[2])
local s = redis.call('HMGET', KEYS[1], 'tokens', 'ts')
local tokens = tonumber(s[1])
local ts = tonumber(s[2])
if tokens == nil then tokens = burst; ts = now end
tokens = math.min(burst, tokens + math.max(0, now - ts) * rate)
local wait = 0
if tokens >= 1 then
  tokens = tokens - 1
else
  wait = math.ceil((1 - tokens) / rate)
end
redis.call('HSET', KEYS[1], 'tokens', tostring(tokens), 'ts', tostring(now))
redis.call('PEXPIRE', KEYS[1], 120000)
return wait
`)

func (g *Governor) key(suffix string) string { return keyVersion + "gov:" + g.name + ":" + suffix }

// Acquire takes one token, waiting up to maxWait for it. It fails fast with a LimitedError while a
// cooldown is active. Redis errors let the call through: the limiter must not become an outage.
func (g *Governor) Acquire(ctx context.Context) error {
	if d := g.Cooldown(ctx); d > 0 {
		if g.c != nil {
			g.c.stats.rateLimited.Add(1)
		}
		return &LimitedError{Upstream: g.name, RetryAfter: d}
	}
	if g.c == nil {
		return nil
	}
	rate := float64(g.perMinute) / 60000.0
	deadline := time.Now().Add(g.maxWait)
	for {
		wait, err := bucketScript.Run(ctx, g.c.rdb, []string{g.key("bucket")}, strconv.FormatFloat(rate, 'f', -1, 64), g.burst).Int64()
		if err != nil {
			g.c.stats.errors.Add(1)
			return nil
		}
		if wait <= 0 {
			return nil
		}
		g.c.stats.tokenDenied.Add(1)
		d := time.Duration(wait) * time.Millisecond
		if time.Now().Add(d).After(deadline) {
			return &LimitedError{Upstream: g.name, RetryAfter: d}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(d):
		}
	}
}

// Penalize starts (or extends) the fleet-wide cooldown after an upstream 429.
func (g *Governor) Penalize(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	if cur := g.Cooldown(ctx); cur >= d {
		return
	}
	if g.c == nil {
		g.mu.Lock()
		g.localBlocked = time.Now().Add(d)
		g.mu.Unlock()
		return
	}
	if err := g.c.rdb.Set(ctx, g.key("cooldown"), 1, d).Err(); err != nil {
		g.c.stats.errors.Add(1)
	}
}

// Cooldown returns the remaining shared cooldown (0 when none).
func (g *Governor) Cooldown(ctx context.Context) time.Duration {
	if g.c == nil {
		g.mu.Lock()
		defer g.mu.Unlock()
		return max(0, time.Until(g.localBlocked))
	}
	d, err := g.c.rdb.PTTL(ctx, g.key("cooldown")).Result()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			g.c.stats.errors.Add(1)
		}
		return 0
	}
	if d < 0 { // -2 key missing, -1 no expiry
		return 0
	}
	return d
}
