package admin

import (
	"net/http"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// ResponseCacheTTL is how long the heavy aggregate endpoints (views, catalog, users, growth, costs,
// overview) reuse a computed answer. These figures move slowly (the rollup runs every 10 minutes) and a
// dashboard refresh or a polling Claude would otherwise rescan the whole history each time. Anything
// that must be fresh (approvals, issues, watch, kill switch, audit) is not cached.
const ResponseCacheTTL = time.Minute

const responseCacheMax = 128

type cachedResponse struct {
	at     time.Time
	header http.Header
	body   []byte
}

type responseCache struct {
	mu    sync.Mutex
	items map[string]cachedResponse
	order []string
	group singleflight.Group
}

// recorder captures a handler's answer so it can be stored and replayed.
type recorder struct {
	header http.Header
	status int
	body   []byte
}

func (r *recorder) Header() http.Header { return r.header }
func (r *recorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
}
func (r *recorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	r.body = append(r.body, b...)
	return len(b), nil
}

// AuthCached is Auth(scope) followed by a short response cache. Authentication always runs first, so a
// cached answer is only ever served to a caller who passed it. The key holds the kind of caller
// (session or token) because a session sees pseudos and a token does not, and the whole request URI.
// Only 200 answers are cached; concurrent identical requests share one computation.
func (s *Service) AuthCached(scope string, ttl time.Duration) func(http.Handler) http.Handler {
	auth := s.Auth(scope)
	return func(next http.Handler) http.Handler {
		return auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				next.ServeHTTP(w, r)
				return
			}
			info, _ := r.Context().Value(authKey{}).(authInfo)
			key := info.via + "|" + r.URL.RequestURI()
			c := &s.respCache
			c.mu.Lock()
			if hit, ok := c.items[key]; ok && s.now().Sub(hit.at) < ttl {
				c.mu.Unlock()
				replay(w, hit, "hit")
				return
			}
			c.mu.Unlock()
			v, _, _ := c.group.Do(key, func() (any, error) {
				rec := &recorder{header: http.Header{}}
				next.ServeHTTP(rec, r)
				out := cachedResponse{at: s.now(), header: rec.header, body: rec.body}
				if rec.status == 0 || rec.status == http.StatusOK {
					c.store(key, out)
					return out, nil
				}
				// not cacheable (an error): replay it once without storing
				return errResponse{status: rec.status, cachedResponse: out}, nil
			})
			switch x := v.(type) {
			case cachedResponse:
				replay(w, x, "miss")
			case errResponse:
				for k, vals := range x.header {
					w.Header()[k] = vals
				}
				w.WriteHeader(x.status)
				_, _ = w.Write(x.body)
			}
		}))
	}
}

type errResponse struct {
	status int
	cachedResponse
}

func replay(w http.ResponseWriter, c cachedResponse, state string) {
	for k, vals := range c.header {
		w.Header()[k] = vals
	}
	w.Header().Set("X-Gazes-Cache", state)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(c.body)
}

func (c *responseCache) store(key string, v cachedResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.items == nil {
		c.items = map[string]cachedResponse{}
	}
	if _, ok := c.items[key]; !ok {
		c.order = append(c.order, key)
		if len(c.order) > responseCacheMax {
			delete(c.items, c.order[0])
			c.order = c.order[1:]
		}
	}
	c.items[key] = v
}

// clear drops every cached answer (an input they depend on changed).
func (c *responseCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items, c.order = nil, nil
}
