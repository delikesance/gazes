package indexer

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// HTTPError is a non-200 answer from a provider. MultiProvider uses Status and RetryAfter to
// decide whether the provider must cool down (429, 5xx, 401/403) and for how long.
type HTTPError struct {
	Provider string
	Status   int
	// RetryAfter is the upstream's Retry-After (seconds or HTTP-date), 0 when absent.
	RetryAfter time.Duration
	// Detail is an optional redacted upstream description (e.g. a Torznab error body).
	Detail string
}

func (e *HTTPError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("%s: HTTP %d: %s", e.Provider, e.Status, e.Detail)
	}
	return fmt.Sprintf("%s: HTTP %d", e.Provider, e.Status)
}

// NewHTTPError builds the error for res, reading its Retry-After header.
func NewHTTPError(provider string, res *http.Response) *HTTPError {
	return &HTTPError{Provider: provider, Status: res.StatusCode, RetryAfter: ParseRetryAfter(res.Header.Get("Retry-After"), time.Now())}
}

// ParseRetryAfter reads a Retry-After value: a number of seconds or an HTTP-date. Unparsable,
// negative or past values yield 0.
func ParseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if secs, err := strconv.Atoi(value); err == nil {
		return time.Duration(max(0, secs)) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil {
		return max(0, at.Sub(now))
	}
	return 0
}

// Pacing bounds how a provider may be called. The zero value means "no special limits".
// Use it for upstreams that enforce their own request delay (Prowlarr's C411 definition allows
// one request per 4.1 s and queues the rest, which would blow every caller's deadline).
type Pacing struct {
	// MaxConcurrent is the number of in-flight requests; 0 means 1 when MinInterval is set, else the default (2).
	MaxConcurrent int
	// MinInterval is the minimum time between the START of two requests to this provider.
	MinInterval time.Duration
}

// IsZero reports that no pacing is configured.
func (p Pacing) IsZero() bool { return p.MaxConcurrent <= 0 && p.MinInterval <= 0 }

// PacedProvider is implemented by providers that declare their pacing. NewMultiProvider enforces it.
// Pacing is enforced per process: instances sharing a Redis share caches and the circuit breaker
// but not the pacing clock, so run one backend instance per paced upstream or lower the interval's
// share accordingly.
type PacedProvider interface {
	Pacing() Pacing
}
