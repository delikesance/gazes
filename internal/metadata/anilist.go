package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/gazes/gazes/internal/kv"
)

// How long a stored answer counts as current for data that moves (older ones are still served
// when AniList cannot answer). Everything else is served from the store with no expiry.
const (
	scheduleStoreMaxAge = 3 * time.Hour
	listingStoreMaxAge  = 24 * time.Hour
	detailStoreMaxAge   = 24 * time.Hour
)

const (
	// anilistDefaultBackoff applies when AniList answers 429 without a Retry-After header.
	anilistDefaultBackoff = 30 * time.Second
	anilistMaxBackoff     = 2 * time.Minute
	anilistMaxWait        = 8 * time.Second
)

// anilistRate is the fleet-wide budget in requests per minute (ANILIST_PER_MINUTE, default 24).
// AniList allows 90/min nominally but has announced X-RateLimit-Limit: 30 for a long time. A 429
// freezes every AniList call for up to a minute (searches included), so the budget stays under
// that limit even when a full burst is spent on top of a minute of sustained calls.
func anilistRate() (perMinute, burst int) {
	perMinute = 24
	if v, err := strconv.Atoi(os.Getenv("ANILIST_PER_MINUTE")); err == nil && v > 0 {
		perMinute = v
	}
	return perMinute, max(4, perMinute/4)
}

// RateLimitError means AniList is throttling us; RetryAfter says when calls may resume.
type RateLimitError struct{ RetryAfter time.Duration }

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("anilist rate limited, retry in %s", e.RetryAfter.Round(time.Second))
}

// anilistClient is the only path to graphql.anilist.co. Every call spends a token from the
// fleet-wide governor and any 429 starts a cooldown shared by all instances.
type anilistClient struct {
	http *http.Client
	gov  *kv.Governor
}

func newAnilistClient(hc *http.Client, c *kv.Client) *anilistClient {
	perMinute, burst := anilistRate()
	return &anilistClient{http: hc, gov: c.NewGovernor("anilist", perMinute, burst, anilistMaxWait)}
}

// post runs one GraphQL request and decodes the answer into out. Every successful answer is kept
// forever in the durable store and served from it on the next identical request.
func (a *anilistClient) post(ctx context.Context, query string, variables any, out any) error {
	return a.postFresh(ctx, query, variables, out, 0)
}

// postFresh is post for answers that go out of date (airing schedules, rankings): a stored answer
// older than maxAge is fetched again, and served anyway when AniList is throttling or failing.
func (a *anilistClient) postFresh(ctx context.Context, query string, variables any, out any, maxAge time.Duration) error {
	store := sharedAnilistStore.Load()
	var key string
	var stale []byte
	if store != nil {
		if k, err := anilistKey(query, variables); err == nil {
			key = k
			if body, at, ok := store.get(ctx, key); ok {
				if maxAge <= 0 || time.Since(at) < maxAge {
					if json.Unmarshal(body, out) == nil {
						return nil
					}
				}
				stale = body
			}
		}
	}
	body, err := a.fetch(ctx, query, variables)
	if err != nil {
		if stale != nil && json.Unmarshal(stale, out) == nil {
			return nil
		}
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return err
	}
	if key != "" && storable(body) {
		_ = store.put(ctx, key, body)
	}
	return nil
}

// anilistRotator moves the process to another exit IP (VPN). Nil without a VPN.
var anilistRotator atomic.Pointer[func(context.Context) error]

// SetAnilistRotator installs the function that changes the exit IP; nil removes it. It must return
// an error when it did not rotate (cooldown, failure).
func SetAnilistRotator(fn func(context.Context) error) {
	if fn == nil {
		anilistRotator.Store(nil)
		return
	}
	anilistRotator.Store(&fn)
}

// anilistStatusError is a non-200, non-429 answer from AniList.
type anilistStatusError struct{ Code int }

func (e *anilistStatusError) Error() string { return fmt.Sprintf("anilist returned status %d", e.Code) }

// throttled reports whether err is AniList refusing this exit IP (429 or a gateway error), as
// opposed to a problem a new IP would not fix.
func throttled(err error) bool {
	var limited *RateLimitError
	if errors.As(err, &limited) {
		return true
	}
	var st *anilistStatusError
	return errors.As(err, &st) && (st.Code == http.StatusBadGateway || st.Code == http.StatusServiceUnavailable || st.Code == http.StatusGatewayTimeout)
}

// fetch performs one upstream request and returns the raw body. When AniList throttles this exit
// IP and a VPN rotator is installed, it moves to a fresh IP, lifts the shared cooldown (it was
// earned by the old IP) and retries once.
func (a *anilistClient) fetch(ctx context.Context, query string, variables any) ([]byte, error) {
	body, err := a.fetchOnce(ctx, query, variables)
	if err == nil || !throttled(err) {
		return body, err
	}
	rotate := anilistRotator.Load()
	if rotate == nil || (*rotate)(ctx) != nil {
		return body, err
	}
	a.gov.ClearCooldown(ctx)
	return a.fetchOnce(ctx, query, variables)
}

func (a *anilistClient) fetchOnce(ctx context.Context, query string, variables any) ([]byte, error) {
	if err := a.gov.Acquire(ctx); err != nil {
		return nil, asRateLimit(err)
	}
	body, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://graphql.anilist.co", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Gazes/1.0 (AnimeStreamingEngine)")
	resp, err := a.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		wait := anilistDefaultBackoff
		if secs, convErr := strconv.Atoi(resp.Header.Get("Retry-After")); convErr == nil && secs > 0 {
			wait = time.Duration(secs) * time.Second
		}
		wait = min(wait, anilistMaxBackoff)
		a.gov.Penalize(ctx, wait)
		return nil, &RateLimitError{RetryAfter: wait}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &anilistStatusError{Code: resp.StatusCode}
	}
	return io.ReadAll(resp.Body)
}

func asRateLimit(err error) error {
	var limited *kv.LimitedError
	if errors.As(err, &limited) {
		return &RateLimitError{RetryAfter: limited.RetryAfter}
	}
	return err
}

// UpstreamCooldown returns how long the fleet-wide AniList cooldown still lasts (0 when none).
func (s *AnimeCatalogService) UpstreamCooldown(ctx context.Context) time.Duration {
	return s.anilist.gov.Cooldown(ctx)
}
