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
}

type window struct {
	count int
	reset time.Time
}

func newLimiter() *limiter { return &limiter{entries: map[string]*window{}, now: time.Now} }

// hit records one event and reports whether the key is still within limit.
func (l *limiter) hit(key string, limit int, span time.Duration) bool {
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
	return w.count <= limit
}

func (l *limiter) reset(key string) {
	l.mu.Lock()
	delete(l.entries, key)
	l.mu.Unlock()
}
