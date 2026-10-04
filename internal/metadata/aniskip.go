package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gazes/gazes/internal/kv"
)

const (
	defaultAniskipBaseURL = "https://api.aniskip.com"

	aniskipLengthTolerance = 3.0 // max |episodeLength - file duration|, seconds
	aniskipMinSec          = 20.0
	aniskipMaxSec          = 180.0
	aniskipFoundTTL        = 7 * 24 * time.Hour
	aniskipEmptyTTL        = 24 * time.Hour
)

// aniskipTimeout bounds one AniSkip request; a var so tests can shorten it.
var aniskipTimeout = 4 * time.Second

type aniskipResponse struct {
	Found   bool `json:"found"`
	Results []struct {
		Interval struct {
			StartTime float64 `json:"startTime"`
			EndTime   float64 `json:"endTime"`
		} `json:"interval"`
		SkipType      string  `json:"skipType"`
		EpisodeLength float64 `json:"episodeLength"`
	} `json:"results"`
}

// SkipTimes returns the community opening/ending segments AniSkip knows for a MAL id and episode,
// keeping only those that match this file's duration. Invalid arguments yield an empty result.
func (s *AnimeCatalogService) SkipTimes(ctx context.Context, malID, episode int, duration float64) ([]SkipSegment, error) {
	if malID <= 0 || episode <= 0 || !(duration > 0) || math.IsInf(duration, 0) {
		return []SkipSegment{}, nil
	}
	key := fmt.Sprintf("%d:%d:%d", malID, episode, int(math.Round(duration)))
	policy := kv.Policy[[]SkipSegment]{TTLFor: func(v *[]SkipSegment) time.Duration {
		if v != nil && len(*v) > 0 {
			return aniskipFoundTTL
		}
		return aniskipEmptyTTL
	}}
	res, err := s.aniskipC.Get(ctx, key, policy, func(ctx context.Context) (*[]SkipSegment, error) {
		segs, err := s.fetchSkipTimes(ctx, malID, episode, duration)
		if err != nil {
			return nil, err
		}
		return &segs, nil
	})
	if err != nil {
		return nil, err
	}
	if res == nil {
		return []SkipSegment{}, nil
	}
	return *res, nil
}

func (s *AnimeCatalogService) fetchSkipTimes(ctx context.Context, malID, episode int, duration float64) ([]SkipSegment, error) {
	ctx, cancel := context.WithTimeout(ctx, aniskipTimeout)
	defer cancel()

	base := s.aniskipBaseURL
	if base == "" {
		base = defaultAniskipBaseURL
	}
	q := url.Values{}
	q.Add("types", "op")
	q.Add("types", "ed")
	q.Set("episodeLength", strconv.FormatFloat(duration, 'f', -1, 64))
	u := fmt.Sprintf("%s/v2/skip-times/%d/%d?%s", base, malID, episode, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	client := http.DefaultClient
	if s.anilist != nil && s.anilist.http != nil {
		client = s.anilist.http
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return nil, fmt.Errorf("aniskip: status %d", resp.StatusCode)
	}
	var parsed aniskipResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("aniskip: decode: %w", err)
	}

	out := []SkipSegment{}
	if resp.StatusCode == http.StatusNotFound || !parsed.Found {
		return out, nil
	}
	seen := map[string]bool{}
	for _, r := range parsed.Results {
		var kind string
		switch r.SkipType {
		case "op":
			kind = SkipKindOpening
		case "ed":
			kind = SkipKindEnding
		default:
			continue
		}
		if seen[kind] {
			continue
		}
		start, end := r.Interval.StartTime, r.Interval.EndTime
		length := end - start
		if math.Abs(r.EpisodeLength-duration) > aniskipLengthTolerance ||
			start < 0 || !(start < end) || end > duration+1 ||
			length < aniskipMinSec || length > aniskipMaxSec {
			continue
		}
		seen[kind] = true
		out = append(out, SkipSegment{Kind: kind, Start: start, End: end, Source: "aniskip"})
	}
	return out, nil
}
