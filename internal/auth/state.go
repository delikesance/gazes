package auth

import (
	"context"
	"encoding/hex"
	"errors"
	"time"

	"github.com/gazes/gazes/internal/kv"
	"github.com/redis/go-redis/v9"
)

// State is the short-lived, security-relevant state of the auth flow (KEM nonces, captcha replay
// protection, rate-limit counters). The in-memory versions inside KEM, Captcha and limiter serve a
// single instance; a State shared through Redis makes several instances behave as one.
// Every method fails closed: an error means "deny".
type State interface {
	// Put records key for ttl (a freshly issued nonce).
	Put(key string, ttl time.Duration) error
	// Take atomically removes key and reports whether it was there (nonce consumption).
	Take(key string) (bool, error)
	// Once records key for ttl and reports whether it was absent (captcha replay check).
	Once(key string, ttl time.Duration) (bool, error)
	// Hit counts one event in a fixed window and reports whether the key is still within limit.
	Hit(key string, limit int, span time.Duration) (bool, error)
	// Reset forgets a counter.
	Reset(key string) error
}

type redisState struct{ c *kv.Client }

// NewRedisState keeps the auth state in Redis, namespaced per stack (each stack has its own KEM key).
func NewRedisState(c *kv.Client) State { return redisState{c: c} }

func (r redisState) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 2*time.Second)
}

func (r redisState) Put(key string, ttl time.Duration) error {
	ctx, cancel := r.ctx()
	defer cancel()
	return r.c.Raw().Set(ctx, r.c.Auth(key), 1, ttl).Err()
}

func (r redisState) Take(key string) (bool, error) {
	ctx, cancel := r.ctx()
	defer cancel()
	n, err := r.c.Raw().Del(ctx, r.c.Auth(key)).Result()
	return n > 0, err
}

func (r redisState) Once(key string, ttl time.Duration) (bool, error) {
	ctx, cancel := r.ctx()
	defer cancel()
	return r.c.Raw().SetNX(ctx, r.c.Auth(key), 1, ttl).Result()
}

var hitScript = redis.NewScript(`
local n = redis.call('INCR', KEYS[1])
if n == 1 then redis.call('PEXPIRE', KEYS[1], ARGV[1]) end
return n`)

func (r redisState) Hit(key string, limit int, span time.Duration) (bool, error) {
	ctx, cancel := r.ctx()
	defer cancel()
	n, err := hitScript.Run(ctx, r.c.Raw(), []string{r.c.Auth("limit", key)}, span.Milliseconds()).Int64()
	if err != nil {
		return false, err
	}
	return n <= int64(limit), nil
}

func (r redisState) Reset(key string) error {
	ctx, cancel := r.ctx()
	defer cancel()
	return r.c.Raw().Del(ctx, r.c.Auth("limit", key)).Err()
}

var errStateUnavailable = errors.New("auth state unavailable")

func nonceKey(nonce []byte) string { return "nonce:" + hex.EncodeToString(nonce) }
