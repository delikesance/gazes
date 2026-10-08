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
	local        healthState // circuit breaker, see health.go
	health       HealthPolicy
}

// NewLocalGovernor is a Governor without Redis: its circuit breaker and cooldown are per process.
func NewLocalGovernor(name string) *Governor { return &Governor{name: name} }

// NewGovernor allows perMinute sustained calls with bursts up to burst. Waits longer than maxWait
// are returned as a LimitedError instead of blocking the request.
func (c *Client) NewGovernor(name string, perMinute, burst int, maxWait time.Duration) *Governor {
	return &Governor{c: c, name: name, perMinute: perMinute, burst: burst, maxWait: maxWait}
}

type backgroundKey struct{}

// IsBackground reports whether ctx was marked with Background (work nobody is waiting on).
func IsBackground(ctx context.Context) bool {
	bg, _ := ctx.Value(backgroundKey{}).(bool)
	return bg
}

// Background marks ctx as work nobody is waiting on (cache warming, bulk enrichment). Its calls
// leave half the burst untouched, so a visitor's request still finds a token right away.
func Background(ctx context.Context) context.Context {
	return context.WithValue(ctx, backgroundKey{}, true)
}

// bucket: KEYS[1]=state hash; ARGV: rate per ms, burst, tokens to leave. Redis TIME keeps every
// instance on one clock.
var bucketScript = redis.NewScript(`
local t = redis.call('TIME')
local now = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000)
local rate = tonumber(ARGV[1])
local burst = tonumber(ARGV[2])
local need = 1 + tonumber(ARGV[3])
local s = redis.call('HMGET', KEYS[1], 'tokens', 'ts')
local tokens = tonumber(s[1])
local ts = tonumber(s[2])
if tokens == nil then tokens = burst; ts = now end
tokens = math.min(burst, tokens + math.max(0, now - ts) * rate)
local wait = 0
if tokens >= need then
  tokens = tokens - 1
else
  wait = math.ceil((need - tokens) / rate)
end
redis.call('HSET', KEYS[1], 'tokens', tostring(tokens), 'ts', tostring(now))
redis.call('PEXPIRE', KEYS[1], 120000)
return wait
`)

func (g *Governor) key(suffix string) string { return keyVersion + "gov:" + g.name + ":" + suffix }

// Acquire takes one token, waiting up to maxWait for it. It fails fast with a LimitedError while a
// cooldown is active. Redis errors let the call through: the limiter must not become an outage.
func (g *Governor) Acquire(ctx context.Context) error {
	if d := g.flatCooldown(ctx); d > 0 {
		if g.c != nil {
			g.c.stats.rateLimited.Add(1)
		}
		return &LimitedError{Upstream: g.name, RetryAfter: d}
	}
	if g.c == nil {
		return nil
	}
	rate := float64(g.perMinute) / 60000.0
	reserve := 0
	if bg, _ := ctx.Value(backgroundKey{}).(bool); bg {
		reserve = g.burst / 2
	}
	deadline := time.Now().Add(g.maxWait)
	for {
		wait, err := bucketScript.Run(ctx, g.c.rdb, []string{g.key("bucket")}, strconv.FormatFloat(rate, 'f', -1, 64), g.burst, reserve).Int64()
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
	// Always remembered locally too: if Redis is down this instance must still back off.
	g.mu.Lock()
	if until := time.Now().Add(d); until.After(g.localBlocked) {
		g.localBlocked = until
	}
	g.mu.Unlock()
	if g.c == nil {
		return
	}
	if cur := g.flatCooldown(ctx); cur >= d {
		return
	}
	if err := g.c.rdb.Set(ctx, g.key("cooldown"), 1, d).Err(); err != nil {
		g.c.stats.errors.Add(1)
	}
}

// ClearCooldown lifts the flat cooldown everywhere, for when the cause is gone (the exit IP that
// earned the 429 was replaced). The circuit breaker is left alone.
func (g *Governor) ClearCooldown(ctx context.Context) {
	g.mu.Lock()
	g.localBlocked = time.Time{}
	g.mu.Unlock()
	if g.c == nil {
		return
	}
	if err := g.c.rdb.Del(ctx, g.key("cooldown")).Err(); err != nil {
		g.c.stats.errors.Add(1)
	}
}

// Blocked returns the remaining flat cooldown set by Penalize (0 when none), ignoring the circuit
// breaker: a deliberate pause rather than an upstream failure.
func (g *Governor) Blocked(ctx context.Context) time.Duration { return g.flatCooldown(ctx) }

// BreakerOpen returns how long the circuit breaker stays open (0 when closed).
func (g *Governor) BreakerOpen(ctx context.Context) time.Duration { return g.healthCooldown(ctx) }

// Cooldown returns the remaining shared cooldown (0 when none): the longer of the flat Penalize
// cooldown and the circuit breaker's open time. It never takes a half-open probe.
func (g *Governor) Cooldown(ctx context.Context) time.Duration {
	return max(g.flatCooldown(ctx), g.healthCooldown(ctx))
}

func (g *Governor) flatCooldown(ctx context.Context) time.Duration {
	g.mu.Lock()
	local := max(0, time.Until(g.localBlocked))
	g.mu.Unlock()
	if g.c == nil {
		return local
	}
	d, err := g.c.rdb.PTTL(ctx, g.key("cooldown")).Result()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			g.c.stats.errors.Add(1)
		}
		return local
	}
	if d < 0 { // -2 key missing, -1 no expiry
		return local
	}
	return max(d, local)
}
