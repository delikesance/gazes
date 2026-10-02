package indexer

import (
	"context"
	"fmt"
	"github.com/gazes/gazes/internal/diagnostics"
	"log/slog"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// LanguageTag represents the audio/subtitle language classification.
type LanguageTag string

const (
	LangVF     LanguageTag = "VF"     // French Audio (Doublage Français)
	LangVOSTFR LanguageTag = "VOSTFR" // French Subtitles (Sous-titres Français)
	LangMULTI  LanguageTag = "MULTI"  // Multi-Audio / Multi-Subs (often includes French)
	LangVOSTEN LanguageTag = "VOSTEN" // English Subtitles
	LangRAW    LanguageTag = "RAW"    // Japanese Audio without subs
	LangOther  LanguageTag = "OTHER"
)

// EpisodeSource represents a resolved torrent swarm for a specific episode.
type EpisodeSource struct {
	AnimeTitle     string   `json:"anime_title"`
	AnimeAliases   []string `json:"anime_aliases"`
	ExcludedTitles []string `json:"excluded_titles,omitempty"`
	TorrentItem
	EpisodeNumber   int            `json:"episode_number"`
	LanguageTag     LanguageTag    `json:"language_tag"`
	LanguageLabel   string         `json:"language_label"`
	ReleaseGroup    string         `json:"release_group"`
	Quality         string         `json:"quality"`
	IsFrench        bool           `json:"is_french"`
	LanguageFlags   []LanguageTag  `json:"language_flags"`
	FrenchEvidence  string         `json:"french_evidence"`
	ScoreBreakdown  ScoreBreakdown `json:"score_breakdown"`
	IsBatch         bool           `json:"is_batch"`
	TaggedEpisode   int            `json:"tagged_episode,omitempty"`
	AbsoluteEpisode int            `json:"absolute_episode,omitempty"`
	SeasonNumber    int            `json:"season_number"`
	ScoreRank       int            `json:"score_rank"`
}

type EpisodeSourcesResponse struct {
	Partial           bool            `json:"partial"`
	Warning           string          `json:"warning,omitempty"`
	AnimeTitle        string          `json:"anime_title"`
	SeasonNumber      int             `json:"season_number"`
	EpisodeNumber     int             `json:"episode_number"`
	RequestID         string          `json:"request_id,omitempty"`
	PlaybackSessionID string          `json:"playback_session_id,omitempty"`
	TotalSources      int             `json:"total_sources"`
	FrenchSources     int             `json:"french_sources"`
	Sources           []EpisodeSource `json:"sources"`
	DebugLog          []string        `json:"debug_log,omitempty"`
}

var (
	vfRegex          = regexp.MustCompile(`(?i)\b(VF|FRENCH|TRUEFRENCH|DOUBLAGE\s*FR)\b`)
	vostfrRegex      = regexp.MustCompile(`(?i)\b(VOSTFR|SUBFRENCH|STFR|SOUS-TITRES\s*FR)\b`)
	multiRegex       = regexp.MustCompile(`(?i)\b(MULTI|MULTi-AUDIO|MULTISUB|MULTI-SUB|DUAL\s*AUDIO)\b`)
	vostenRegex      = regexp.MustCompile(`(?i)\b(VOSTEN|SUBBED|ENG\s*SUB|ENGLISH)\b`)
	rawRegex         = regexp.MustCompile(`(?i)\b(RAW|RAW-HD|JAP\s*RAW)\b`)
	seasonRegex      = regexp.MustCompile(`(?i)\b(?:season|saison|part|cour)\s*0*(\d+)\b|\bS0*(\d+)\b|\b(\d+)(?:st|nd|rd|th)\s*Season\b`)
	ovaTag           = regexp.MustCompile(`(?i)\b(OAD|OVA|OAV|SP|Specials?|Extra|Extras|Bonus)\b`)
	cleanSpacesRegex = regexp.MustCompile(`\s+`)
)

func languageTitle(title string) string {
	return strings.ReplaceAll(normalize(title), "vfvostfr", "vf vostfr")
}

// ClassifyLanguage detects the language profile from the release title.
func ClassifyLanguage(title string) (LanguageTag, string, bool) {
	titleUpper := strings.ToUpper(languageTitle(title))

	// Check VF (French Dub)
	if vfRegex.MatchString(titleUpper) {
		return LangVF, "🇫🇷 VF (Audio Français)", true
	}

	// Check VOSTFR (French Subtitles)
	if vostfrRegex.MatchString(titleUpper) {
		return LangVOSTFR, "🇫🇷 VOSTFR (Sous-titres FR)", true
	}

	if multiRegex.MatchString(titleUpper) || strings.Contains(titleUpper, "MULTIPLE SUBTITLE") {
		return LangMULTI, "MULTI (français non confirmé)", false
	}

	if vostenRegex.MatchString(titleUpper) {
		return LangVOSTEN, "🇬🇧 VOSTEN (English)", false
	}

	if rawRegex.MatchString(titleUpper) {
		return LangRAW, "🇯🇵 RAW", false
	}

	return LangOther, "Langue non précisée", false
}

// EpisodeResolver resolves torrent swarms for specific anime episodes.
type EpisodeResolver struct {
	indexer Provider
}

// NewEpisodeResolver creates a new episode resolver.
func NewEpisodeResolver(idx Provider) *EpisodeResolver {
	return &EpisodeResolver{
		indexer: idx,
	}
}

// ExtractSeasonNumber extracts season number from anime titles like "Re:ZERO Season 4".
func ExtractSeasonNumber(title string) int {
	matches := seasonRegex.FindStringSubmatch(title)
	if len(matches) > 1 {
		for i := 1; i < len(matches); i++ {
			if matches[i] != "" {
				var s int
				if _, err := fmt.Sscanf(matches[i], "%d", &s); err == nil && s > 0 {
					return s
				}
			}
		}
	}
	return 1
}

// CleanTitleForSearch cleans special chars like colons, dashes, and extra subtitles.
var emptyBracketsRegex = regexp.MustCompile(`[(\[{]\s*[)\]}]`)

func CleanTitleForSearch(title string) string {
	t := strings.ReplaceAll(title, ":", " ")
	t = strings.ReplaceAll(t, "-", " ")
	t = strings.ReplaceAll(t, "'", "")
	t = strings.ReplaceAll(t, "\"", "")
	t = seasonRegex.ReplaceAllString(t, " ")
	t = emptyBracketsRegex.ReplaceAllString(t, " ") // "Part 6 (Part 2)" must not leave "( )" behind
	t = cleanSpacesRegex.ReplaceAllString(t, " ")
	return strings.TrimSpace(t)
}

type ScoreBreakdown struct {
	French   int `json:"french"`
	Multi    int `json:"multi"`
	Swarm    int `json:"swarm"`
	Leechers int `json:"leechers"` // Kept for API compatibility; leechers do not contribute to the score.
	Quality  int `json:"quality"`
}

// EpisodeIdentity keeps catalog identity separate from release numbering.
type EpisodeIdentity struct {
	ExcludedTitles    []string
	UnqualifiedTitles []string
	Titles            []string
	SeasonNumber      int
	PartNumber        int
	EpisodeNumber     int
	AbsoluteEpisode   int
	TaggedEpisode     int
	Standalone        bool
	IsOVA             bool
	AllowUnqualified  bool
}

var releaseEpisode = regexp.MustCompile(`(?i)\bS0*(\d+)E0*(\d+)(?:v\d+)?\b|\b(?:EP?|episode)\s*0*(\d+)(?:v\d+)?\b|(?:^|\s)-\s*0*(\d+)(?:v\d+)?(?:\s|\[|\(|\.|$)`)
var releaseRange = regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{N}])0*(\d+)[\s_]*[-~][\s_]*0*(\d+)(?:$|[^\p{L}\p{N}])`)
var extraVideoTag = regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{N}])(?:OP|ED|OST|NCOP|NCED|opening|ending|soundtrack|trailer|sample)(?:$|[^\p{L}\p{N}])`)
var movieTag = regexp.MustCompile(`(?i)\b(?:movies?|films?|gekijouban)\b`)
var seasonSetTag = regexp.MustCompile(`(?i)\bS0*(\d+)((?:\+0*\d+)+)\b`)

// A season explicitly bundled with extras is a pack even without "Batch" or
// "Complete". A separately numbered OVA remains an extra, not a TV episode.
var seasonExtrasPackTag = regexp.MustCompile(`(?i)\bS0*\d+\s*\+\s*(?:OADs?|OVAs?|OAVs?|Extras?|Bonus)\b`)
var batchTag = regexp.MustCompile(`(?i)\b(batch|complete|integrale|intégrale|collection)\b`)
var partTag = regexp.MustCompile(`(?i)\b(?:part|cour|partie)\s*0*(\d+)\b`)
var releaseSeason = regexp.MustCompile(`(?i)\bS0*(\d+)(?:E\d+)?\b|\b(?:season|saison)\s*0*(\d+)\b|\b(\d+)(?:st|nd|rd|th)(?:\s*season|\s*[-_ ]|\b)`)
var normalizedTitle = regexp.MustCompile(`[^\p{L}\p{N}]+`)

func number(s string) int { var n int; fmt.Sscanf(s, "%d", &n); return n }
func normalize(s string) string {
	return strings.TrimSpace(normalizedTitle.ReplaceAllString(strings.ToLower(s), " "))
}

// UniqueSeasonAliases excludes generic franchise aliases from unqualified episode matches.
func UniqueSeasonAliases(selected, canonical []string) []string {
	generic := map[string]bool{}
	for _, alias := range canonical {
		generic[normalize(CleanTitleForSearch(alias))] = true
	}
	unique := []string{}
	for _, alias := range selected {
		key := normalize(CleanTitleForSearch(alias))
		if key != "" && !generic[key] {
			unique = append(unique, alias)
		}
	}
	return unique
}

// MoreSpecificTitle compares aliases across punctuation and spacing differences.
func MoreSpecificTitle(candidate, base string) bool {
	candidate = normalize(CleanTitleForSearch(candidate))
	base = normalize(CleanTitleForSearch(base))
	return base != "" && strings.HasPrefix(candidate, base+" ")
}

// ExtractPartNumber retains split-season identity separately from season numbering.
func ExtractPartNumber(title string) int {
	if m := partTag.FindStringSubmatch(title); len(m) > 1 {
		return number(m[1])
	}
	return 0
}

// MatchEpisodeDebug accepts contextual episode identities and provides a debug decision string.
func MatchEpisodeDebug(title string, identity EpisodeIdentity) (bool, bool, string) {
	title = strings.NewReplacer("_", " ", ".", " ").Replace(title)
	matchTitle := regexp.MustCompile(`^\s*\[[^\]]+\]\s*`).ReplaceAllString(title, "")
	normalized := " " + normalize(matchTitle) + " "

	multiSeasonPack := false
	if set := seasonSetTag.FindStringSubmatch(title); len(set) > 0 {
		for _, entry := range strings.Split(set[1]+set[2], "+") {
			if number(entry) == identity.SeasonNumber {
				multiSeasonPack = true
			}
		}
		if !multiSeasonPack {
			return false, true, "rejected: requested season absent from multi-season pack"
		}
	}
	// Extract season number
	season := 0
	if m := releaseSeason.FindStringSubmatch(title); len(m) > 0 {
		for _, v := range m[1:] {
			if v != "" {
				season = number(v)
				break
			}
		}
	}
	if multiSeasonPack {
		season = identity.SeasonNumber
	}
	if season > 0 && season != identity.SeasonNumber {
		return false, false, fmt.Sprintf("rejected: season mismatch (got S%02d, want S%02d)", season, identity.SeasonNumber)
	}
	seasonExtrasPack := season == identity.SeasonNumber && season > 0 &&
		seasonExtrasPackTag.MatchString(title) && !releaseEpisode.MatchString(title)

	// 1. Excluded titles rejection
	for _, other := range identity.ExcludedTitles {
		base := normalize(CleanTitleForSearch(other))
		if base != "" && strings.Contains(normalized, " "+base+" ") {
			if multiSeasonPack || (season == identity.SeasonNumber && season > 0 && ovaTag.MatchString(other)) {
				continue
			}
			return false, false, fmt.Sprintf("rejected: matches excluded title '%s'", other)
		}
	}

	// 2. Extra media and OVA/movie rejections
	rangeMatch := releaseRange.FindStringSubmatch(title)
	if extraVideoTag.MatchString(title) && len(rangeMatch) == 0 {
		return false, false, "rejected: extra/OST/OP/ED video tag"
	}
	if !identity.Standalone && movieTag.MatchString(title) && len(rangeMatch) == 0 {
		return false, false, "rejected: movie tag on standard TV target"
	}
	if !identity.Standalone && !identity.IsOVA && ovaTag.MatchString(title) {
		isTVBatchWithOAD := multiSeasonPack || seasonExtrasPack || (season == identity.SeasonNumber && batchTag.MatchString(title)) ||
			(len(rangeMatch) > 2 && number(rangeMatch[2])-number(rangeMatch[1]) >= 5)
		if !isTVBatchWithOAD {
			return false, false, "rejected: OVA/OAD tag on standard TV target"
		}
	}

	// 3. Title match against allowed aliases
	titleMatch := false
	matchedAlias := ""
	for _, alias := range identity.Titles {
		base := normalize(CleanTitleForSearch(alias))
		if base != "" && strings.Contains(normalized, " "+base+" ") {
			titleMatch = true
			matchedAlias = alias
			break
		}
	}
	if !titleMatch {
		return false, false, "rejected: no alias match in title"
	}

	// 5. Part number check
	part := ExtractPartNumber(title)
	if identity.PartNumber > 0 && part > 0 && part != identity.PartNumber {
		return false, false, fmt.Sprintf("rejected: part mismatch (got Part %d, want Part %d)", part, identity.PartNumber)
	}
	if identity.PartNumber > 1 && part == 0 && identity.AbsoluteEpisode == 0 {
		return false, false, "rejected: missing required part number for split season"
	}

	if season == 0 && !identity.AllowUnqualified {
		exact := false
		for _, alias := range identity.Titles {
			if normalize(alias) != normalize(CleanTitleForSearch(alias)) && strings.Contains(normalized, " "+normalize(alias)+" ") {
				exact = true
			}
		}
		for _, alias := range identity.UnqualifiedTitles {
			if normalize(alias) != "" && strings.Contains(normalized, " "+normalize(alias)+" ") {
				exact = true
			}
		}
		if !exact && identity.AbsoluteEpisode == 0 {
			return false, false, "rejected: unqualified title not permitted for season > 1 without distinct subtitle"
		}
	}

	target := identity.EpisodeNumber
	if season > 0 && identity.TaggedEpisode > 0 {
		target = identity.TaggedEpisode
	}
	if season == 0 && identity.AbsoluteEpisode > 0 {
		target = identity.AbsoluteEpisode
	}

	// 6. Episode number check
	if m := releaseEpisode.FindStringSubmatch(title); len(m) > 0 {
		ep := 0
		if m[2] != "" {
			ep = number(m[2])
		} else {
			for _, v := range m[3:] {
				if v != "" {
					ep = number(v)
					break
				}
			}
		}
		if tail := regexp.MustCompile(`(?i)^\s*[-~]\s*(?:E|EP)?0*(\d+)(?:\s|\[|\(|\.|$)`).FindStringSubmatch(title[strings.Index(title, m[0])+len(m[0]):]); len(tail) > 1 {
			end := number(tail[1])
			if ep > 0 && end >= ep && target >= ep && target <= end {
				return true, true, fmt.Sprintf("matched: tagged range %02d-%02d (contains ep %d)", ep, end, target)
			}
			return false, true, fmt.Sprintf("rejected: tagged range %02d-%02d does not contain ep %d", ep, end, target)
		}
		if ep == target {
			return true, false, fmt.Sprintf("matched: explicit episode %02d", ep)
		}
		return false, false, fmt.Sprintf("rejected: episode mismatch (got ep %d, want ep %d)", ep, target)
	}

	if m := rangeMatch; len(m) > 2 {
		lo, hi := number(m[1]), number(m[2])
		if lo >= 1900 && hi <= 2099 {
			// Ignore year ranges like 2019-2021
		} else if lo > 0 && hi >= lo && target >= lo && target <= hi {
			return true, true, fmt.Sprintf("matched: range %02d-%02d (contains ep %d)", lo, hi, target)
		} else if lo > 0 && hi >= lo {
			return false, true, fmt.Sprintf("rejected: range %02d-%02d does not contain ep %d", lo, hi, target)
		}
	}

	// Bare episode numbers
	for _, alias := range identity.Titles {
		base := strings.TrimSpace(CleanTitleForSearch(alias))
		if base == "" {
			continue
		}
		pattern := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(base) + `\s+0*(\d{1,4})(?:v\d+)?(?:\s|\[|\(|\.|$)`)
		if m := pattern.FindStringSubmatch(title); len(m) > 1 {
			n := number(m[1])
			if n != 480 && n != 720 && n != 1080 && n != 2160 && (n < 1900 || n > 2099) {
				if n == target {
					return true, false, fmt.Sprintf("matched: bare episode %02d after title", n)
				}
				return false, false, fmt.Sprintf("rejected: bare episode %02d does not match target %d", n, target)
			}
		}
	}

	if identity.Standalone {
		return true, false, "matched: standalone movie"
	}

	if (multiSeasonPack || seasonExtrasPack || batchTag.MatchString(title)) && (season > 0 || identity.AllowUnqualified) {
		return true, true, fmt.Sprintf("matched: batch pack for season %d", identity.SeasonNumber)
	}

	if season > 0 && len(rangeMatch) == 0 {
		return false, true, fmt.Sprintf("rejected: season %d release without episode number", season)
	}

	return false, false, fmt.Sprintf("rejected: no episode number matching %d found in '%s' (alias '%s')", target, title, matchedAlias)
}

// MatchEpisode accepts contextual episode identities and marked packs, rejecting unrelated numeric tags.
func MatchEpisode(title string, identity EpisodeIdentity) (bool, bool) {
	match, batch, _ := MatchEpisodeDebug(title, identity)
	return match, batch
}

func RankSource(item TorrentItem, identity EpisodeIdentity, batch bool) EpisodeSource {
	tag, label, fr := ClassifyLanguage(item.Title)
	flags := make([]LanguageTag, 0, 3)
	if vfRegex.MatchString(languageTitle(item.Title)) {
		flags = append(flags, LangVF)
	}
	if vostfrRegex.MatchString(languageTitle(item.Title)) {
		flags = append(flags, LangVOSTFR)
	}
	multi := multiRegex.MatchString(languageTitle(item.Title)) || strings.Contains(strings.ToUpper(item.Title), "MULTIPLE SUBTITLE")
	if multi {
		flags = append(flags, LangMULTI)
	}
	quality := ExtractQuality(item.Title)
	points := 0
	switch quality {
	case "480p":
		points = 1
	case "720p":
		points = 2
	case "1080p":
		points = 3
	case "4K UHD":
		points = 4
	}
	breakdown := ScoreBreakdown{Swarm: min(60, int(math.Round(8*math.Log2(1+float64(max(0, item.Seeders)))))), Quality: points}
	evidence := "unconfirmed"
	if fr {
		// Exceeds the maximum combined swarm, MULTI and quality bonuses (66).
		// The resolver still places all unseeded sources last.
		breakdown.French = 100
		if vfRegex.MatchString(languageTitle(item.Title)) {
			// A confirmed dub outranks even the healthiest VOSTFR source.
			breakdown.French = 200
		}
		evidence = "release_title"
	}
	if multi {
		breakdown.Multi = 2
	}
	animeTitle := ""
	if len(identity.Titles) > 0 {
		animeTitle = identity.Titles[0]
	}
	return EpisodeSource{AnimeTitle: animeTitle, AnimeAliases: identity.Titles, ExcludedTitles: identity.ExcludedTitles, TorrentItem: item, EpisodeNumber: identity.EpisodeNumber, SeasonNumber: identity.SeasonNumber, AbsoluteEpisode: identity.AbsoluteEpisode, TaggedEpisode: identity.TaggedEpisode, LanguageTag: tag, LanguageLabel: label, LanguageFlags: flags, IsFrench: fr, FrenchEvidence: evidence, Quality: quality, ReleaseGroup: ExtractReleaseGroup(item.Title), ScoreRank: breakdown.French + breakdown.Multi + breakdown.Swarm + breakdown.Leechers + breakdown.Quality, ScoreBreakdown: breakdown, IsBatch: batch}
}

func (r *EpisodeResolver) ResolveEpisodeSources(ctx context.Context, titles []string, ep int) (*EpisodeSourcesResponse, error) {
	season := 1
	if len(titles) > 0 {
		season = ExtractSeasonNumber(titles[0])
	}
	return r.ResolveSeasonSources(ctx, EpisodeIdentity{Titles: titles, SeasonNumber: season, EpisodeNumber: ep, AllowUnqualified: season == 1})
}

// EpisodeSearchQueries prioritizes exact title/SxxExx identity for every alias.
func EpisodeSearchQueries(identity EpisodeIdentity) []string {
	queries := []string{}
	episode := identity.EpisodeNumber
	if identity.TaggedEpisode > 0 {
		episode = identity.TaggedEpisode
	}
	seen := map[string]bool{}
	add := func(q string) {
		q = strings.TrimSpace(q)
		if q != "" && !seen[q] {
			seen[q] = true
			queries = append(queries, q)
		}
	}

	relevantTitles := []string{}
	for _, alias := range identity.Titles {
		clean := CleanTitleForSearch(strings.TrimSpace(alias))
		if clean != "" && regexp.MustCompile(`[A-Za-z]`).MatchString(clean) {
			norm := strings.ToLower(clean)
			if !seen[norm] {
				seen[norm] = true
				relevantTitles = append(relevantTitles, clean)
			}
		}
		if len(relevantTitles) >= 4 {
			break
		}
	}
	if len(relevantTitles) == 0 && len(identity.Titles) > 0 {
		relevantTitles = append(relevantTitles, CleanTitleForSearch(identity.Titles[0]))
	}

	for _, base := range relevantTitles {
		add(fmt.Sprintf("%s S%02dE%02d", base, identity.SeasonNumber, episode))
		add(fmt.Sprintf("%s %02d", base, episode))
		add(fmt.Sprintf("%s S%02d", base, identity.SeasonNumber))
		add(fmt.Sprintf("%s Season %d", base, identity.SeasonNumber))
		add(fmt.Sprintf("%s Season %d batch", base, identity.SeasonNumber))
		add(fmt.Sprintf("%s Judas", base))
		add(fmt.Sprintf("%s Dual Audio", base))
		add(fmt.Sprintf("%s Multi-Subs", base))
		add(base + " batch")
		if identity.AbsoluteEpisode > 0 {
			add(fmt.Sprintf("%s %02d", base, identity.AbsoluteEpisode))
		}
	}
	return queries
}

// FrenchSearchOptions run independently so broad English packs cannot consume their budget.
func FrenchSearchOptions(identity EpisodeIdentity) []SearchOptions {
	aliases := []string{}
	seen := map[string]bool{}
	for _, alias := range identity.Titles {
		base := CleanTitleForSearch(alias)
		key := normalize(base)
		if key != "" && regexp.MustCompile(`[A-Za-z]`).MatchString(base) && !seen[key] && len(aliases) < 6 {
			seen[key] = true
			aliases = append(aliases, base)
		}

	}
	options := []SearchOptions{}
	for _, suffix := range []string{"VF", "VOSTFR", "FRENCH", "MULTI", fmt.Sprintf("S%02d VOSTFR", identity.SeasonNumber), fmt.Sprintf("S%02d VF", identity.SeasonNumber), fmt.Sprintf("S%02dE%02d VOSTFR", identity.SeasonNumber, identity.EpisodeNumber), fmt.Sprintf("S%02dE%02d VF", identity.SeasonNumber, identity.EpisodeNumber), fmt.Sprintf("%d VOSTFR", identity.EpisodeNumber), fmt.Sprintf("%d VF", identity.EpisodeNumber)} {
		for _, base := range aliases {
			// French subtitles also appear in the English-translated category
			// on multilingual packs; search it separately from French releases.
			for _, category := range []string{"1_2", "1_3"} {
				options = append(options, SearchOptions{Query: base + " " + suffix, Category: category})
			}
		}
	}
	for _, base := range aliases {
		options = append(options, SearchOptions{Query: fmt.Sprintf("%s %d", base, identity.EpisodeNumber), Category: "1_3"})
	}
	return options
}

func (r *EpisodeResolver) ResolveSeasonSources(ctx context.Context, identity EpisodeIdentity) (*EpisodeSourcesResponse, error) {
	if len(identity.Titles) == 0 || identity.EpisodeNumber <= 0 {
		return nil, fmt.Errorf("invalid episode search arguments")
	}
	start := time.Now()
	diagnostics.Log(ctx, slog.LevelInfo, "resolver.started", "titles", identity.Titles, "excluded_titles", identity.ExcludedTitles, "season_number", identity.SeasonNumber, "episode_number", identity.EpisodeNumber)
	queries := EpisodeSearchQueries(identity)
	var mu sync.Mutex
	items := map[string]TorrentItem{}
	failures, successes := 0, 0
	truncated := false
	searchParent, cancelSearch := context.WithTimeout(ctx, 20*time.Second)
	defer cancelSearch()
	var wg sync.WaitGroup
	runSearch := func(opts SearchOptions) {
		searchCtx, cancel := context.WithTimeout(searchParent, 8*time.Second)
		defer cancel()
		querySeen := map[string]bool{}
		for page := 1; page <= 2; page++ {
			if searchCtx.Err() != nil {
				mu.Lock()
				failures++
				mu.Unlock()
				return
			}
			opts.Page = page
			opts.SortBy = "seeders"
			opts.Order = "desc"
			results, err := r.indexer.Search(searchCtx, opts)
			mu.Lock()
			if err != nil {
				failures++
				partial, isPartial := err.(*PartialError)
				if len(results) == 0 && (!isPartial || partial.AllFailed) {
					mu.Unlock()
					return
				}
			}
			successes++
			newToQuery := 0
			for _, item := range results {
				hash := strings.ToLower(strings.TrimSpace(item.InfoHash))
				if hash == "" {
					continue
				}
				if !querySeen[hash] {
					querySeen[hash] = true
					newToQuery++
				}
				previous, exists := items[hash]
				if exists {
					diagnostics.Log(ctx, slog.LevelDebug, "resolver.deduplicated", "infohash", hash, "provider", item.Provider)
				}
				if !exists || item.Seeders > previous.Seeders {
					item.InfoHash = hash
					items[hash] = item
				}
			}
			if len(results) > 0 && page > 1 && newToQuery == 0 {
				truncated = true
				mu.Unlock()
				return
			}
			if page == 2 && len(results) >= 75 {
				truncated = true
			}
			mu.Unlock()
			if len(results) < 75 {
				return
			}
		}
	}
	runGroup := func(options []SearchOptions) {
		jobs := make(chan SearchOptions, len(options))
		for _, option := range options {
			jobs <- option
		}
		close(jobs)
		for worker := 0; worker < 4; worker++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for option := range jobs {
					runSearch(option)
				}
			}()
		}
	}
	generic := make([]SearchOptions, 0, len(queries))
	for _, query := range queries {
		generic = append(generic, SearchOptions{Query: query, Category: "1_0"})
	}
	runGroup(FrenchSearchOptions(identity))
	runGroup(generic)

	wg.Wait()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if successes == 0 {
		return nil, fmt.Errorf("all torrent searches failed")
	}
	sources := make([]EpisodeSource, 0)
	french := 0
	debugLog := make([]string, 0, 50)
	debugLog = append(debugLog, fmt.Sprintf("[Resolver] Target: Season %d Episode %d (Part %d, Absolute %d, Tagged %d)", identity.SeasonNumber, identity.EpisodeNumber, identity.PartNumber, identity.AbsoluteEpisode, identity.TaggedEpisode))
	debugLog = append(debugLog, fmt.Sprintf("[Resolver] Titles (%d): %v", len(identity.Titles), identity.Titles))
	if len(identity.ExcludedTitles) > 0 {
		debugLog = append(debugLog, fmt.Sprintf("[Resolver] Excluded titles (%d): %v", len(identity.ExcludedTitles), identity.ExcludedTitles))
	}
	for _, item := range items {
		matches, batch, reason := MatchEpisodeDebug(item.Title, identity)
		if !matches {
			diagnostics.Log(ctx, slog.LevelDebug, "resolver.rejected", "infohash", item.InfoHash, "provider", item.Provider, "title", item.Title, "reason", reason)
			if len(debugLog) < 60 {
				debugLog = append(debugLog, fmt.Sprintf("  [-] %s => %s", item.Title, reason))
			}
			continue
		}
		debugLog = append(debugLog, fmt.Sprintf("  [+] %s => %s (Seeders: %d)", item.Title, reason, item.Seeders))
		source := RankSource(item, identity, batch)
		diagnostics.Log(ctx, slog.LevelDebug, "resolver.accepted", "infohash", item.InfoHash, "provider", item.Provider, "title", item.Title, "reason", reason, "score", source.ScoreRank, "language", source.LanguageTag, "seeders", item.Seeders, "batch", batch)
		sources = append(sources, source)
		if source.IsFrench {
			french++
		}
	}
	sort.Slice(sources, func(i, j int) bool {
		a, b := sources[i], sources[j]
		if (a.Seeders > 0) != (b.Seeders > 0) {
			return a.Seeders > 0
		}
		if a.ScoreRank != b.ScoreRank {
			return a.ScoreRank > b.ScoreRank
		}
		if a.Seeders != b.Seeders {
			return a.Seeders > b.Seeders
		}
		if a.IsFrench != b.IsFrench {
			return a.IsFrench
		}
		if a.ScoreBreakdown.Quality != b.ScoreBreakdown.Quality {
			return a.ScoreBreakdown.Quality > b.ScoreBreakdown.Quality
		}
		return a.InfoHash < b.InfoHash
	})
	diagnostics.Log(ctx, slog.LevelInfo, "resolver.completed", "sources", len(sources), "french_sources", french, "results", len(items), "search_failures", failures, "search_successes", successes, "duration_ms", time.Since(start).Milliseconds())
	response := &EpisodeSourcesResponse{RequestID: diagnostics.Get(ctx).RequestID, PlaybackSessionID: diagnostics.Get(ctx).SessionID,
		AnimeTitle:    identity.Titles[0],
		SeasonNumber:  identity.SeasonNumber,
		EpisodeNumber: identity.EpisodeNumber,
		TotalSources:  len(sources),
		FrenchSources: french,
		Sources:       sources,
		DebugLog:      debugLog,
		Partial:       failures > 0 || truncated,
	}
	if response.Partial {
		response.Warning = "Recherche partielle : certains résultats du fournisseur sont indisponibles ou limités."
	}
	return response, nil
}
