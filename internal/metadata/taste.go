package metadata

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

const (
	// Under this much playback a session is an accidental click, not a signal.
	minSignalSeconds = 120
	// Below this share of an episode, a session that did play is read as "tried and dropped".
	abandonRatio = 0.2
	// Without a known duration, a regular episode is assumed to last this long.
	defaultEpisodeSeconds = 1400
	recencyHalfLifeDays   = 60
)

// TasteSession is the part of a stored watch session the recommender needs.
type TasteSession struct {
	AnimeID        int
	SeasonID       int
	Genres         []string
	WatchedSeconds float64
	Duration       float64
	Completed      bool
	UpdatedAt      int64
}

// Taste is a viewer profile derived from their watch history.
type Taste struct {
	// Genres holds signed weights scaled so the strongest is 1: positive for what is
	// watched through, negative for what was tried and dropped.
	Genres map[string]float64
	// Seeds are the anime watched for real, most recent first.
	Seeds []int
	// Dropped are the anime tried and abandoned: never suggested again.
	Dropped map[int]bool
}

func (s TasteSession) animeKey() int {
	if s.AnimeID > 0 {
		return s.AnimeID
	}
	return s.SeasonID
}

// BuildTaste turns watch sessions into a taste profile. Time spent, completion, recency
// (60-day half-life) and abandonment all shape the weight of each session's genres.
func BuildTaste(sessions []TasteSession, now time.Time) Taste {
	taste := Taste{Genres: map[string]float64{}, Dropped: map[int]bool{}}
	lastSeen := map[int]int64{}
	engaged := map[int]bool{}
	tried := map[int]bool{}
	for _, s := range sessions {
		if s.WatchedSeconds < minSignalSeconds {
			continue
		}
		key := s.animeKey()
		if key <= 0 {
			continue
		}
		duration := s.Duration
		if duration <= 0 {
			duration = defaultEpisodeSeconds
		}
		ratio := math.Min(s.WatchedSeconds/duration, 1)
		weight := ratio
		switch {
		case s.Completed:
			weight *= 1.5
			engaged[key] = true
		case ratio < abandonRatio:
			weight = -0.5
			tried[key] = true
		default:
			engaged[key] = true
		}
		ageDays := math.Max(0, now.Sub(time.Unix(s.UpdatedAt, 0)).Hours()/24)
		weight *= math.Pow(0.5, ageDays/recencyHalfLifeDays)
		if len(s.Genres) > 0 {
			share := weight / math.Sqrt(float64(len(s.Genres)))
			for _, genre := range s.Genres {
				taste.Genres[strings.ToLower(strings.TrimSpace(genre))] += share
			}
		}
		if s.UpdatedAt > lastSeen[key] {
			lastSeen[key] = s.UpdatedAt
		}
	}
	// An anime only counts as dropped when no session ever engaged with it.
	for key := range tried {
		if !engaged[key] {
			taste.Dropped[key] = true
			delete(lastSeen, key)
		}
	}
	peak := 0.0
	for _, w := range taste.Genres {
		peak = math.Max(peak, math.Abs(w))
	}
	if peak > 0 {
		for genre, w := range taste.Genres {
			taste.Genres[genre] = w / peak
		}
	}
	for key := range lastSeen {
		taste.Seeds = append(taste.Seeds, key)
	}
	sort.Slice(taste.Seeds, func(a, b int) bool {
		if lastSeen[taste.Seeds[a]] != lastSeen[taste.Seeds[b]] {
			return lastSeen[taste.Seeds[a]] > lastSeen[taste.Seeds[b]]
		}
		return taste.Seeds[a] < taste.Seeds[b]
	})
	return taste
}

// Empty reports whether the profile carries any signal.
func (t Taste) Empty() bool { return len(t.Genres) == 0 && len(t.Dropped) == 0 }

// Score rates a candidate's genres against the profile, in [-1, 1].
func (t Taste) Score(genres []string) float64 {
	if len(genres) == 0 || len(t.Genres) == 0 {
		return 0
	}
	sum := 0.0
	for _, genre := range genres {
		sum += t.Genres[strings.ToLower(strings.TrimSpace(genre))]
	}
	return math.Max(-1, math.Min(1, sum/math.Sqrt(float64(len(genres)))))
}

// Fingerprint is a stable short digest, so two viewers with the same profile share cached feeds.
func (t Taste) Fingerprint() string {
	if t.Empty() {
		return "none"
	}
	genres := make([]string, 0, len(t.Genres))
	for genre, w := range t.Genres {
		genres = append(genres, fmt.Sprintf("%s:%.1f", genre, w))
	}
	sort.Strings(genres)
	dropped := make([]int, 0, len(t.Dropped))
	for id := range t.Dropped {
		dropped = append(dropped, id)
	}
	sort.Ints(dropped)
	sum := sha1.Sum([]byte(strings.Join(genres, ",") + "|" + fmt.Sprint(dropped)))
	return hex.EncodeToString(sum[:6])
}
