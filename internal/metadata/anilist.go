package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gazes/gazes/internal/kv"
)

const (
	// anilistDefaultBackoff applies when AniList answers 429 without a Retry-After header.
	anilistDefaultBackoff = 30 * time.Second
	anilistMaxBackoff     = 2 * time.Minute
	// AniList allows 90 requests/min nominally but is currently degraded to 30: stay under it.
	anilistPerMinute = 25
	anilistBurst     = 10
	anilistMaxWait   = 8 * time.Second
)

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
	return &anilistClient{http: hc, gov: c.NewGovernor("anilist", anilistPerMinute, anilistBurst, anilistMaxWait)}
}

// post runs one GraphQL request and decodes the answer into out.
func (a *anilistClient) post(ctx context.Context, query string, variables any, out any) error {
	if err := a.gov.Acquire(ctx); err != nil {
		return asRateLimit(err)
	}
	body, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://graphql.anilist.co", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Gazes/1.0 (AnimeStreamingEngine)")
	resp, err := a.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		wait := anilistDefaultBackoff
		if secs, convErr := strconv.Atoi(resp.Header.Get("Retry-After")); convErr == nil && secs > 0 {
			wait = time.Duration(secs) * time.Second
		}
		wait = min(wait, anilistMaxBackoff)
		a.gov.Penalize(ctx, wait)
		return &RateLimitError{RetryAfter: wait}
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("anilist returned status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func asRateLimit(err error) error {
	var limited *kv.LimitedError
	if errors.As(err, &limited) {
		return &RateLimitError{RetryAfter: limited.RetryAfter}
	}
	return err
}
