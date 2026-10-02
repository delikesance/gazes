package auth

import (
	"sync"
	"time"
)

// limiter is a fixed-window counter keyed by an arbitrary string (IP, email index).
type limiter struct {
	mu      sync.Mutex
	entries map[string]*window
	now     func() time.Time
	shared  State // when set, counters live in Redis and an error denies
}

type window struct {
	count int
	reset time.Time
}

func newLimiter() *limiter { return &limiter{entries: map[string]*window{}, now: time.Now} }

// hit records one event and reports whether the key is still within limit.
func (l *limiter) hit(key string, limit int, span time.Duration) bool {
	ok, _ := l.hitChecked(key, limit, span)
	return ok
}

// hitChecked is hit that also reports a shared store failure (the call is then denied, fail closed).
func (l *limiter) hitChecked(key string, limit int, span time.Duration) (bool, error) {
	if l.shared != nil {
		ok, err := l.shared.Hit(key, limit, span)
		return err == nil && ok, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if len(l.entries) > 50000 {
		for k, w := range l.entries {
			if now.After(w.reset) {
				delete(l.entries, k)
			}
		}
	}
	w := l.entries[key]
	if w == nil || now.After(w.reset) {
		w = &window{reset: now.Add(span)}
		l.entries[key] = w
	}
	w.count++
	return w.count <= limit, nil
}

func (l *limiter) reset(key string) {
	if l.shared != nil {
		_ = l.shared.Reset(key)
		return
	}
	l.mu.Lock()
	delete(l.entries, key)
	l.mu.Unlock()
}
