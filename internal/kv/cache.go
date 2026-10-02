package kv

import (
	"container/list"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	mrand "math/rand/v2"
	"sync"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

const (
	envelopeMagic = 0x01
	flagZstd      = 0x01
	compressAbove = 2048
	lockTTL       = 30 * time.Second
	// staleWindow is how long past its soft TTL an entry stays in Redis as a fallback.
	staleWindow = 7 * 24 * time.Hour
	// refreshBackoff pauses background refreshes of an entry after a failed attempt.
	refreshBackoff = 30 * time.Second
)

var (
	zstdEnc, _ = zstd.NewWriter(nil)
	zstdDec, _ = zstd.NewReader(nil)
)

// Policy says how long a fetched value stays fresh. TTLFor, when set, overrides TTL per value
// (return 0 to not store it, e.g. an incomplete answer that must be retried).
type Policy[T any] struct {
	TTL    time.Duration
	TTLFor func(*T) time.Duration
	// StaleFor says how long past its TTL a value may still be served while it refreshes in the
	// background (default 7 days). Return 0 for values that must never be served stale.
	StaleFor func(*T) time.Duration
}

func (p Policy[T]) staleFor(v *T) time.Duration {
	if p.StaleFor != nil {
		return p.StaleFor(v)
	}
	return staleWindow
}

func (p Policy[T]) ttl(v *T) time.Duration {
	if p.TTLFor != nil {
		return p.TTLFor(v)
	}
	return p.TTL
}

// CacheOptions tunes one Cache.
type CacheOptions struct {
	L1TTL        time.Duration // in-process lifetime of an entry (default 15 s)
	L1Max        int           // in-process entries (default 512)
	FetchTimeout time.Duration // detached budget of one upstream fetch (default 30 s)
	WaitBudget   time.Duration // how long to wait for another instance's fetch (default 35 s)
}

// Cache is a typed two-level cache (process memory, then Redis) with stale-while-revalidate and a
// distributed single-flight: N concurrent callers across N instances trigger one upstream fetch.
type Cache[T any] struct {
	c      *Client
	domain string
	opts   CacheOptions
	l1     *lru[T]
	flight singleflight.Group
}

// NewCache creates a cache for one key domain ("catalog", "schedule", "sources", ...).
func NewCache[T any](c *Client, domain string, opts CacheOptions) *Cache[T] {
	if opts.L1TTL <= 0 {
		opts.L1TTL = 15 * time.Second
	}
	if opts.L1Max <= 0 {
		opts.L1Max = 512
	}
	if opts.FetchTimeout <= 0 {
		opts.FetchTimeout = 30 * time.Second
	}
	if opts.WaitBudget <= 0 {
		opts.WaitBudget = 35 * time.Second
	}
	return &Cache[T]{c: c, domain: domain, opts: opts, l1: newLRU[T](opts.L1Max)}
}

// Get returns the cached value for key, fetching it at most once across the whole fleet.
//   - fresh in memory or Redis: returned immediately;
//   - stale: returned immediately and refreshed in the background;
//   - missing: fetched under a distributed lock, others wait for the result;
//   - fetch fails: the stale value (if any) is returned instead of the error.
func (ca *Cache[T]) Get(ctx context.Context, key string, p Policy[T], fetch func(context.Context) (*T, error)) (*T, error) {
	if ca.c == nil {
		return ca.getLocal(ctx, key, p, fetch)
	}
	if v, ok := ca.l1.get(key); ok {
		ca.c.stats.l1Hits.Add(1)
		return v, nil
	}
	rk := ca.c.Up(ca.domain, key)
	entry, found := ca.read(ctx, rk)
	if found && entry.fresh() {
		ca.c.stats.l2Hits.Add(1)
		ca.l1.put(key, entry.val, min(ca.opts.L1TTL, entry.remaining()))
		return entry.val, nil
	}
	if found && -entry.remaining() > p.staleFor(entry.val) {
		found = false // too old to serve even as a stand-in: treat as a miss
	}
	if found { // stale: serve now, refresh behind
		ca.c.stats.stale.Add(1)
		ca.l1.put(key, entry.val, 2*time.Second)
		go ca.refresh(rk, key, p, fetch)
		return entry.val, nil
	}
	ca.c.stats.misses.Add(1)
	res := ca.flight.DoChan(rk, func() (any, error) { return ca.loadShared(rk, key, p, fetch) })
	select {
	case r := <-res:
		if r.Err != nil {
			return nil, r.Err
		}
		return r.Val.(*T), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// getLocal is the memory-only mode (nil client): same API, one process, no stale fallback.
func (ca *Cache[T]) getLocal(ctx context.Context, key string, p Policy[T], fetch func(context.Context) (*T, error)) (*T, error) {
	if v, ok := ca.l1.get(key); ok {
		return v, nil
	}
	res := ca.flight.DoChan(key, func() (any, error) {
		fctx, cancel := context.WithTimeout(context.Background(), ca.opts.FetchTimeout)
		defer cancel()
		v, err := fetch(fctx)
		if err == nil {
			if ttl := p.ttl(v); ttl > 0 {
				ca.l1.put(key, v, ttl)
			}
		}
		return v, err
	})
	select {
	case r := <-res:
		if r.Err != nil {
			return nil, r.Err
		}
		return r.Val.(*T), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Put stores a value directly (cache warming, tests).
func (ca *Cache[T]) Put(ctx context.Context, key string, v *T, ttl time.Duration) {
	if ca.c == nil {
		ca.l1.put(key, v, ttl)
		return
	}
	ca.write(ctx, ca.c.Up(ca.domain, key), v, ttl)
	ca.l1.put(key, v, min(ca.opts.L1TTL, ttl))
}

// Invalidate drops a key from both levels.
func (ca *Cache[T]) Invalidate(ctx context.Context, key string) {
	ca.l1.drop(key)
	if ca.c == nil {
		return
	}
	_ = ca.c.rdb.Del(ctx, ca.c.Up(ca.domain, key)).Err()
}

// refresh renews a stale entry in the background, at most once per process and per backoff window.
func (ca *Cache[T]) refresh(rk, key string, p Policy[T], fetch func(context.Context) (*T, error)) {
	ca.flight.DoChan("refresh:"+rk, func() (any, error) {
		bg, cancel := context.WithTimeout(context.Background(), ca.opts.FetchTimeout)
		defer cancel()
		if n, err := ca.c.rdb.Exists(bg, rk+":nofetch").Result(); err == nil && n > 0 {
			return nil, nil
		}
		ca.c.stats.refreshes.Add(1)
		if _, err := ca.loadShared(rk, key, p, fetch); err != nil {
			_ = ca.c.rdb.Set(bg, rk+":nofetch", 1, refreshBackoff).Err()
		}
		return nil, nil
	})
}

// loadShared runs one fetch for rk across every instance: the lock holder fetches, the others poll
// Redis for the result. Redis errors fall back to a plain local fetch.
func (ca *Cache[T]) loadShared(rk, key string, p Policy[T], fetch func(context.Context) (*T, error)) (*T, error) {
	fctx, cancel := context.WithTimeout(context.Background(), ca.opts.FetchTimeout)
	defer cancel()
	deadline := time.Now().Add(ca.opts.WaitBudget)
	delay := 50 * time.Millisecond
	waited := false
	for {
		token, held, err := ca.c.lock(fctx, rk)
		if err != nil { // Redis trouble: still serve the caller
			ca.c.stats.errors.Add(1)
			return fetch(fctx)
		}
		if held {
			defer ca.c.unlock(rk, token)
			if e, ok := ca.read(fctx, rk); ok && e.fresh() { // filled while we queued
				return e.val, nil
			}
			return ca.fetchAndStore(fctx, rk, key, p, fetch)
		}
		if !waited {
			ca.c.stats.lockWaits.Add(1)
			waited = true
		}
		if e, ok := ca.read(fctx, rk); ok && e.fresh() {
			return e.val, nil
		}
		if time.Now().After(deadline) { // the holder is too slow: do not block the caller forever
			return ca.fetchAndStore(fctx, rk, key, p, fetch)
		}
		time.Sleep(delay)
		delay = min(delay*2, 250*time.Millisecond)
	}
}

func (ca *Cache[T]) fetchAndStore(ctx context.Context, rk, key string, p Policy[T], fetch func(context.Context) (*T, error)) (*T, error) {
	v, err := fetch(ctx)
	if err != nil {
		if e, ok := ca.read(ctx, rk); ok { // an older answer beats an error
			ca.c.stats.stale.Add(1)
			return e.val, nil
		}
		return nil, err
	}
	if ttl := p.ttl(v); ttl > 0 {
		ca.write(ctx, rk, v, ttl)
		ca.l1.put(key, v, min(ca.opts.L1TTL, ttl))
	}
	return v, nil
}

type entry[T any] struct {
	val     *T
	savedAt time.Time
	soft    time.Duration
}

func (e entry[T]) fresh() bool              { return e.remaining() > 0 }
func (e entry[T]) remaining() time.Duration { return e.savedAt.Add(e.soft).Sub(time.Now()) }

func (ca *Cache[T]) read(ctx context.Context, rk string) (entry[T], bool) {
	raw, err := ca.c.rdb.Get(ctx, rk).Bytes()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			ca.c.stats.errors.Add(1)
		}
		return entry[T]{}, false
	}
	e, ok := decode[T](raw)
	return e, ok
}

func (ca *Cache[T]) write(ctx context.Context, rk string, v *T, ttl time.Duration) {
	// ±10 % jitter so entries written together do not all expire together.
	soft := time.Duration(float64(ttl) * (0.9 + 0.2*mrand.Float64()))
	raw, err := encode(v, time.Now(), soft)
	if err != nil {
		return
	}
	if err := ca.c.rdb.Set(ctx, rk, raw, soft+staleWindow).Err(); err != nil {
		ca.c.stats.errors.Add(1)
	}
}

// encode: magic, flags, savedAt(ms), soft(ms), payload (JSON, zstd above 2 KB).
func encode[T any](v *T, savedAt time.Time, soft time.Duration) ([]byte, error) {
	payload, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	flags := byte(0)
	if len(payload) > compressAbove {
		payload = zstdEnc.EncodeAll(payload, nil)
		flags |= flagZstd
	}
	out := make([]byte, 18, 18+len(payload))
	out[0], out[1] = envelopeMagic, flags
	binary.BigEndian.PutUint64(out[2:], uint64(savedAt.UnixMilli()))
	binary.BigEndian.PutUint64(out[10:], uint64(soft.Milliseconds()))
	return append(out, payload...), nil
}

func decode[T any](raw []byte) (entry[T], bool) {
	if len(raw) < 18 || raw[0] != envelopeMagic {
		return entry[T]{}, false
	}
	payload := raw[18:]
	if raw[1]&flagZstd != 0 {
		var err error
		if payload, err = zstdDec.DecodeAll(payload, nil); err != nil {
			return entry[T]{}, false
		}
	}
	var v T
	if err := json.Unmarshal(payload, &v); err != nil {
		return entry[T]{}, false
	}
	return entry[T]{
		val:     &v,
		savedAt: time.UnixMilli(int64(binary.BigEndian.Uint64(raw[2:]))),
		soft:    time.Duration(binary.BigEndian.Uint64(raw[10:])) * time.Millisecond,
	}, true
}

// --- distributed lock -------------------------------------------------------------------------

var unlockScript = redis.NewScript(`if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("del", KEYS[1]) else return 0 end`)

func (c *Client) lock(ctx context.Context, rk string) (token string, held bool, err error) {
	var b [12]byte
	_, _ = rand.Read(b[:])
	token = hex.EncodeToString(b[:])
	held, err = c.rdb.SetNX(ctx, rk+":lock", token, lockTTL).Result()
	return token, held, err
}

func (c *Client) unlock(rk, token string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = unlockScript.Run(ctx, c.rdb, []string{rk + ":lock"}, token).Err()
}

// --- small LRU --------------------------------------------------------------------------------

type lruItem[T any] struct {
	key   string
	val   *T
	until time.Time
}

type lru[T any] struct {
	mu  sync.Mutex
	max int
	ll  *list.List
	m   map[string]*list.Element
}

func newLRU[T any](max int) *lru[T] {
	return &lru[T]{max: max, ll: list.New(), m: make(map[string]*list.Element)}
}

func (l *lru[T]) get(key string) (*T, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	el, ok := l.m[key]
	if !ok {
		return nil, false
	}
	it := el.Value.(*lruItem[T])
	if time.Now().After(it.until) {
		l.ll.Remove(el)
		delete(l.m, key)
		return nil, false
	}
	l.ll.MoveToFront(el)
	return it.val, true
}

func (l *lru[T]) put(key string, v *T, ttl time.Duration) {
	if ttl <= 0 || math.IsNaN(float64(ttl)) {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if el, ok := l.m[key]; ok {
		it := el.Value.(*lruItem[T])
		it.val, it.until = v, time.Now().Add(ttl)
		l.ll.MoveToFront(el)
		return
	}
	l.m[key] = l.ll.PushFront(&lruItem[T]{key: key, val: v, until: time.Now().Add(ttl)})
	for l.ll.Len() > l.max {
		last := l.ll.Back()
		l.ll.Remove(last)
		delete(l.m, last.Value.(*lruItem[T]).key)
	}
}

func (l *lru[T]) drop(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if el, ok := l.m[key]; ok {
		l.ll.Remove(el)
		delete(l.m, key)
	}
}
