package admin

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// UserLookup resolves a user's role from their ID (implemented by *auth.Store.UserRole).
type UserLookup interface {
	UserRole(ctx context.Context, id int64) (string, error)
}

// TokenVerifier resolves a plain bearer token (implemented by *Store).
type TokenVerifier interface {
	VerifyToken(ctx context.Context, plain string) (*Token, error)
}

const roleAdmin = "admin"

// RequireAdminSession lets a request through only when identify reports a logged-in user whose
// role is admin: 401 when not logged in, 403 otherwise. Bearer tokens are never consulted.
func RequireAdminSession(identify func(*http.Request) (int64, bool), users UserLookup) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, ok := identify(r)
			if !ok {
				writeErr(w, http.StatusUnauthorized, "authentication required")
				return
			}
			role, err := users.UserRole(r.Context(), id)
			if err != nil || role != roleAdmin {
				writeErr(w, http.StatusForbidden, "forbidden")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// TokenOptions tunes RequireTokenWith.
type TokenOptions struct {
	RatePerMinute int              // requests per minute per token; <= 0 means 60
	Now           func() time.Time // test hook
}

type ctxKey struct{}

// TokenFromContext returns the token authenticated by RequireToken.
func TokenFromContext(ctx context.Context) (*Token, bool) {
	t, ok := ctx.Value(ctxKey{}).(*Token)
	return t, ok && t != nil
}

// RequireToken authenticates ONLY through "Authorization: Bearer gzs_..." (cookies are ignored),
// requires every scope, and rate-limits per token at 60 requests per minute.
func RequireToken(store TokenVerifier, scopes ...string) func(http.Handler) http.Handler {
	return RequireTokenWith(store, TokenOptions{}, scopes...)
}

// RequireTokenWith is RequireToken with explicit options. The limiter is shared by every
// handler wrapped by the returned middleware.
func RequireTokenWith(store TokenVerifier, opts TokenOptions, scopes ...string) func(http.Handler) http.Handler {
	if len(scopes) == 0 {
		// A guard with no required scope would accept any valid token: a wiring mistake, caught at startup.
		panic("admin: RequireToken needs at least one scope")
	}
	if opts.RatePerMinute <= 0 {
		opts.RatePerMinute = 60
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	lim := &limiter{rate: float64(opts.RatePerMinute) / 60, burst: float64(opts.RatePerMinute), now: opts.Now, buckets: map[int64]*bucket{}}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			plain, ok := bearer(r)
			if !ok {
				unauthorized(w)
				return
			}
			tok, err := store.VerifyToken(r.Context(), plain)
			if err != nil {
				if errors.Is(err, ErrInvalidToken) {
					unauthorized(w)
				} else {
					writeErr(w, http.StatusInternalServerError, "internal error")
				}
				return
			}
			for _, sc := range scopes {
				if !tok.HasScope(sc) {
					writeErr(w, http.StatusForbidden, "insufficient scope")
					return
				}
			}
			if wait, ok := lim.take(tok.ID); !ok {
				secs := int(wait.Seconds()) + 1
				w.Header().Set("Retry-After", strconv.Itoa(secs))
				writeErr(w, http.StatusTooManyRequests, "rate limit exceeded")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, tok)))
		})
	}
}

func bearer(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	const p = "Bearer "
	if len(h) <= len(p) || !strings.EqualFold(h[:len(p)], p) {
		return "", false
	}
	return strings.TrimSpace(h[len(p):]), true
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	writeErr(w, http.StatusUnauthorized, "invalid or missing token")
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(`{"error":"` + msg + `"}`))
}

type bucket struct {
	tokens float64
	last   time.Time
}

type limiter struct {
	mu      sync.Mutex
	rate    float64 // tokens per second
	burst   float64
	now     func() time.Time
	buckets map[int64]*bucket
}

// take consumes one token; when empty it returns the wait until the next one.
func (l *limiter) take(id int64) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	b := l.buckets[id]
	if b == nil {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[id] = b
	}
	if el := now.Sub(b.last).Seconds(); el > 0 {
		b.tokens = min(l.burst, b.tokens+el*l.rate)
		b.last = now
	}
	if b.tokens < 1 {
		return time.Duration((1 - b.tokens) / l.rate * float64(time.Second)), false
	}
	b.tokens--
	return 0, true
}
