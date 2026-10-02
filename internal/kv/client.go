// Package kv is the Redis layer shared by every backend instance: a two-level cache with
// stale-while-revalidate and distributed single-flight, a per-upstream rate governor with a shared
// cooldown, and the key schema. Redis is required at startup; at runtime an error degrades to a
// cache miss (and the callers that guard security state fail closed).
package kv

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// keyVersion namespaces every key; bump it to invalidate the whole cache.
const keyVersion = "gz:v1:"

// Client wraps a go-redis client with the key schema and counters.
type Client struct {
	rdb       *redis.Client
	namespace string
	stats     counters
}

// Open connects to url (redis://[:password@]host:port/db) and waits up to wait for Redis to answer.
func Open(ctx context.Context, url, namespace string, wait time.Duration) (*Client, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("invalid REDIS_URL: %w", err)
	}
	opts.MaxRetries = 0 // the breaker below decides when to try again
	opts.DialTimeout = time.Second
	opts.ReadTimeout = 2 * time.Second
	opts.WriteTimeout = 2 * time.Second
	rdb := redis.NewClient(opts)
	deadline := time.Now().Add(wait)
	for {
		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err = rdb.Ping(pingCtx).Err()
		cancel()
		if err == nil {
			return New(rdb, namespace), nil
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			_ = rdb.Close()
			return nil, fmt.Errorf("redis unreachable: %w", err)
		}
		time.Sleep(time.Second)
	}
}

// New wraps an existing client (used by tests).
func New(rdb *redis.Client, namespace string) *Client {
	if namespace == "" {
		namespace = "gazes"
	}
	c := &Client{rdb: rdb, namespace: namespace}
	rdb.AddHook(&breaker{c: c})
	return c
}

// breakerOpenFor is how long Redis is ignored after a network failure.
const breakerOpenFor = 5 * time.Second

// breaker is a circuit breaker on the Redis connection. During an outage every operation would
// otherwise wait for its own dial/read timeout, and a request making several of them stalls for
// tens of seconds. After one network failure Redis is skipped for a few seconds: operations fail
// at once, caches fall back to direct upstream calls, and the auth state fails closed instantly.
type breaker struct {
	c     *Client
	until atomic.Int64 // unix nanoseconds
}

var errBreakerOpen = errors.New("redis circuit open")

func (b *breaker) DialHook(next redis.DialHook) redis.DialHook { return next }

func (b *breaker) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if time.Now().UnixNano() < b.until.Load() {
			return errBreakerOpen
		}
		err := next(ctx, cmd)
		b.observe(err)
		return err
	}
}

func (b *breaker) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		if time.Now().UnixNano() < b.until.Load() {
			return errBreakerOpen
		}
		err := next(ctx, cmds)
		b.observe(err)
		return err
	}
}

// observe opens the circuit on connection-level failures only: a missing key, a server error or
// the caller's own cancellation say nothing about Redis' health.
func (b *breaker) observe(err error) {
	if err == nil || errors.Is(err, redis.Nil) || errors.Is(err, context.Canceled) {
		return
	}
	var serverErr redis.Error
	if errors.As(err, &serverErr) {
		return
	}
	b.until.Store(time.Now().Add(breakerOpenFor).UnixNano())
}

// Close releases the connection pool.
func (c *Client) Close() error { return c.rdb.Close() }

// Raw exposes the underlying client for stores that need their own commands.
func (c *Client) Raw() *redis.Client { return c.rdb }

// Up builds a key shared by every stack that talks to the same upstreams.
func (c *Client) Up(parts ...string) string { return keyVersion + "up:" + join(parts) }

// Auth builds a key private to this stack (each stack has its own KEM key and accounts).
func (c *Client) Auth(parts ...string) string {
	return keyVersion + c.namespace + ":auth:" + join(parts)
}

func join(parts []string) string {
	key := strings.Join(parts, ":")
	if len(key) > 120 {
		sum := sha256.Sum256([]byte(key))
		return key[:40] + "~" + hex.EncodeToString(sum[:16])
	}
	return key
}

type counters struct {
	l1Hits, l2Hits, stale, misses, lockWaits, refreshes, errors, rateLimited, tokenDenied atomic.Int64
}

// Stats is a point-in-time copy of the counters.
type Stats struct {
	L1Hits      int64 `json:"l1_hits"`
	L2Hits      int64 `json:"l2_hits"`
	StaleServed int64 `json:"stale_served"`
	Misses      int64 `json:"misses"`
	LockWaits   int64 `json:"lock_waits"`
	Refreshes   int64 `json:"refreshes"`
	Errors      int64 `json:"redis_errors"`
	RateLimited int64 `json:"upstream_cooldowns"`
	TokenDenied int64 `json:"tokens_denied"`
}

// Stats returns the counters for the diagnostics endpoint.
func (c *Client) Stats() Stats {
	s := &c.stats
	return Stats{s.l1Hits.Load(), s.l2Hits.Load(), s.stale.Load(), s.misses.Load(), s.lockWaits.Load(), s.refreshes.Load(), s.errors.Load(), s.rateLimited.Load(), s.tokenDenied.Load()}
}

// Elect reports whether this instance wins the periodic job name for the next period: of all the
// instances calling it, exactly one gets true per period (SET NX with the period as expiry).
func (c *Client) Elect(ctx context.Context, name string, period time.Duration) bool {
	ok, err := c.rdb.SetNX(ctx, keyVersion+"leader:"+name, 1, period).Result()
	if err != nil {
		c.stats.errors.Add(1)
		return false
	}
	return ok
}

// Health pings Redis and counts its keys, for the diagnostics endpoint.
func (c *Client) Health(ctx context.Context) (latency time.Duration, keys int64, err error) {
	start := time.Now()
	if err = c.rdb.Ping(ctx).Err(); err != nil {
		return 0, 0, err
	}
	latency = time.Since(start)
	keys, err = c.rdb.DBSize(ctx).Result()
	return latency, keys, err
}
