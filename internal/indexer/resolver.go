package indexer

import (
	"context"
	"fmt"
	"github.com/gazes/gazes/internal/diagnostics"
	"golang.org/x/text/unicode/norm"
	"log/slog"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
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
	vfRegex          = regexp.MustCompile(`(?i)\b(VF|VFF|VFQ|VF2|FRENCH|TRUEFRENCH|DOUBLAGE\s*FR|AUDIO\s*FR)\b`)
	vostfrRegex      = regexp.MustCompile(`(?i)\b(VOSTFR|SUBFRENCH|STFR|SOUS-TITRES\s*FR)\b`)
	multiRegex       = regexp.MustCompile(`(?i)\b(MULTI|MULTi-AUDIO|MULTISUB|MULTI-SUB|DUAL\s*AUDIO)\b`)
	vostenRegex      = regexp.MustCompile(`(?i)\b(VOSTEN|SUBBED|ENG\s*SUB|ENGLISH)\b`)
	rawRegex         = regexp.MustCompile(`(?i)\b(RAW|RAW-HD|JAP\s*RAW)\b`)
	seasonRegex      = regexp.MustCompile(`(?i)\b(?:season|saison|part|cour)\s*0*(\d+)\b|\bS0*(\d+)\b|\b(\d+)(?:st|nd|rd|th)\s*Season\b`)
	ovaTag           = regexp.MustCompile(`(?i)\b(OADs?|OVAs?|OAVs?|SP|Specials?|Extra|Extras|Bonus)\b`)
	cleanSpacesRegex = regexp.MustCompile(`\s+`)
)

var frenchSubtitleWords = regexp.MustCompile(`(?i)\b(?:(?:french|francais)\s+(?:subs?|subtitles?)|(?:subs?|subtitles?)\s+in\s+(?:french|francais))\b`)

func languageTitle(title string) string {
	title = frenchSubtitleWords.ReplaceAllString(normalize(title), "vostfr")
	return strings.ReplaceAll(title, "vfvostfr", "vf vostfr")
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
	// fastPhase bounds the first playback discovery round (see DefaultFastPhaseTimeout).
	fastPhase time.Duration
}

// DefaultFastPhaseTimeout is how long the shared season-pack queries of a playback resolve may run
// before the exhaustive discovery starts.
const DefaultFastPhaseTimeout = 3 * time.Second

// SetFastPhaseTimeout overrides the fast-phase budget; non-positive values keep the default.
func (r *EpisodeResolver) SetFastPhaseTimeout(d time.Duration) {
	if d > 0 {
		r.fastPhase = d
	}
}

// EpisodeSourceResolver allows a maintained external resolver to be used while
// retaining the local resolver as a fallback.
type EpisodeSourceResolver interface {
	ResolveSeasonSources(context.Context, EpisodeIdentity) (*EpisodeSourcesResponse, error)
	ResolvePlaybackSources(context.Context, EpisodeIdentity) (*EpisodeSourcesResponse, error)
}

// NewEpisodeResolver creates a new episode resolver.
func NewEpisodeResolver(idx Provider) *EpisodeResolver {
	return &EpisodeResolver{
		indexer:   idx,
		fastPhase: DefaultFastPhaseTimeout,
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

// LanguageTier orders sources VF > VOSTFR > unconfirmed MULTI > other, whatever their swarm.
// MULTI is not proof of a French track, but it is a better French-audience fallback than an
// English-only release, so it sits below confirmed VOSTFR/VF and above everything else.
func LanguageTier(b ScoreBreakdown) int {
	if b.French > 0 {
		return b.French
	}
	if b.Multi > 0 {
		return 75
	}
	return 0
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
	// MediaID is the immutable catalog identifier (AniList today). Authoritative
	// resolvers use it to look up an explicit *Arr binding; they never infer the
	// work from a release title.
	MediaID           int
	Format            string
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

var leadingReleaseGroup = regexp.MustCompile(`^\s*\[[^\]]+\]\s*`)
var taggedRangeTail = regexp.MustCompile(`(?i)^\s*[-~]\s*(?:E|EP)?0*(\d+)(?:\s|\[|\(|\.|$)`)
var latinSearchTitle = regexp.MustCompile(`[A-Za-z]`)

var releaseEpisode = regexp.MustCompile(`(?i)\bS0*(\d+)E0*(\d+)(?:v\d+)?\b|\b(?:EP?|episode)\s*0*(\d+)(?:v\d+)?\b|(?:^|\s)-\s*0*(\d+)(?:v\d+)?(?:\s|\[|\(|\.|$)`)

// cjkTotalEpisode matches "[18 - 总第84]" / "总第85": season-local episode (optional) plus absolute number.
var cjkTotalEpisode = regexp.MustCompile(`(?:(\d+)\s*-\s*)?[总總]第\s*0*(\d+)`)

// seasonBareEpisode matches a bare episode right after an ordinal/S/Season marker ("4th 15 [", "S2 05 ").
// Underscores are already spaces by the time Match runs. Resolutions, years and "10bit"-style tokens
// are excluded by the terminator and by the value check at the call site.
var seasonBareEpisode = regexp.MustCompile(`(?i)(?:\b\d+(?:st|nd|rd|th)|\bS0*\d+|\b(?:season|saison)\s*0*\d+)\s+0*(\d+)(?:v\d+)?(?:\s|\[|\(|$)`)
var releaseRange = regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{N}])0*(\d+)[\s_]*[-~][\s_]*0*(\d+)(?:$|[^\p{L}\p{N}])`)
var extraVideoTag = regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{N}])(?:OP|ED|OST|NCOP|NCED|opening|ending|soundtrack|trailer|sample)(?:$|[^\p{L}\p{N}])`)

// fanEditTag matches French fan recuts ("Fan-Kaï", "Henshū" chapters) and generic fan edits.
// Standalone "Kai" is deliberately absent: Dragon Ball Kai and friends are official series.
var fanEditTag = regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{N}])(?:fan[\s_-]*ka[iï]|hensh[uūû]|fan[\s_-]*edit|re[\s_-]?cut)(?:$|[^\p{L}\p{N}])`)
var narutoVariantTag = regexp.MustCompile(`(?i)\bnaruto\s+(?:shippu?den\s+)?(?:yaba[iï]|kai|full\s*edit|sd|spin\s*off)(?:\s|$)`)
var movieTag = regexp.MustCompile(`(?i)\b(?:movies?|films?|gekijouban)\b`)
var seasonSetTag = regexp.MustCompile(`(?i)\bS0*(\d+)((?:\+(?:S)?0*\d+)+)\b`)

// A season explicitly bundled with extras is a pack even without "Batch" or
// "Complete". A separately numbered OVA remains an extra, not a TV episode.
var seasonExtrasPackTag = regexp.MustCompile(`(?i)\b(?:S0*\d+|(?:season|saison)\s*(?:\d+|one|two|three|four|un|une|deux|trois|quatre))\s*\+\s*(?:\d+\s+)?(?:OADs?|OVAs?|OAVs?|Extras?|Bonus)\b`)
var batchTag = regexp.MustCompile(`(?i)\b(batch|complete|integrale|intégrale|collection)\b`)
var partTag = regexp.MustCompile(`(?i)\b(?:part|cour|partie)\s*0*(\d+)\b`)
var wordSeason = regexp.MustCompile(`(?i)\b(?:season|saison)\s+(one|two|three|four|un|une|deux|trois|quatre)\b`)
var wordSeasonNumbers = map[string]int{"one": 1, "un": 1, "une": 1, "two": 2, "deux": 2, "three": 3, "trois": 3, "four": 4, "quatre": 4}

// cjkSeason matches "第四季", "第4期", "第二部" style season markers; 話/话 (episode) is deliberately excluded.
var cjkSeason = regexp.MustCompile(`第\s*([一二三四五六七八九十0-9]{1,3})\s*[季期部]`)

var cjkDigits = map[rune]int{'一': 1, '二': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9}

// cjkSeasonNumber converts the number inside a 第N季 marker (digits or 一..九十九) to an int.
func cjkSeasonNumber(s string) int {
	if n := number(s); n > 0 {
		return n
	}
	runes := []rune(s)
	total := 0
	for i, r := range runes {
		switch {
		case r == '十':
			tens := 1
			if i > 0 {
				tens = cjkDigits[runes[i-1]]
			}
			total += tens * 10
		case i+1 < len(runes) && runes[i+1] == '十':
			// consumed by the following 十
		default:
			total += cjkDigits[r]
		}
	}
	return total
}

var releaseSeason = regexp.MustCompile(`(?i)\bS0*(\d+)(?:E\d+)?\b|\b(?:season|saison)\s*0*(\d+)\b|\b(\d+)(?:st|nd|rd|th)(?:\s*season|\s*[-_ ]|\b)`)
var normalizedTitle = regexp.MustCompile(`[^\p{L}\p{N}]+`)

func number(s string) int { var n int; fmt.Sscanf(s, "%d", &n); return n }
func normalize(s string) string {
	latin := false
	s = strings.Map(func(r rune) rune {
		if unicode.IsMark(r) {
			if latin {
				return -1
			}
			return r
		}
		latin = unicode.Is(unicode.Latin, r)
		return r
	}, norm.NFKD.String(s))
	return strings.TrimSpace(normalizedTitle.ReplaceAllString(strings.ToLower(norm.NFC.String(s)), " "))
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
// completePackTag marks a release as a bundle of many episodes rather than a single unnumbered one.
var completePackTag = regexp.MustCompile(`(?i)\b(?:complete|complet|compl[eè]te|int[eé]grale|bd|bdrip|blu-?ray|bdmv|dual[ -]audio|multi[ -]?(?:audio|subs?)|vf|vostfr)\b`)

type preparedAlias struct {
	original, clean, key string
	bare                 *regexp.Regexp
}

// EpisodeMatcher prepares identity-dependent work once for a whole candidate set.
type EpisodeMatcher struct {
	identity    EpisodeIdentity
	aliases     []preparedAlias
	excluded    []string
	unqualified []string
}

func NewEpisodeMatcher(identity EpisodeIdentity) *EpisodeMatcher {
	m := &EpisodeMatcher{identity: identity}
	for _, alias := range identity.Titles {
		clean := CleanTitleForSearch(alias)
		a := preparedAlias{original: normalize(alias), clean: normalize(clean), key: alias}
		if clean != "" {
			a.bare = regexp.MustCompile(`(?i)` + regexp.QuoteMeta(clean) + `\s+0*(\d{1,4})(?:v\d+)?(?:\s|\[|\(|\.|$)`)
		}
		m.aliases = append(m.aliases, a)
	}
	for _, alias := range identity.ExcludedTitles {
		m.excluded = append(m.excluded, normalize(CleanTitleForSearch(alias)))
	}
	for _, alias := range identity.UnqualifiedTitles {
		m.unqualified = append(m.unqualified, normalize(alias))
	}
	return m
}

func MatchEpisodeDebug(title string, identity EpisodeIdentity) (bool, bool, string) {
	return NewEpisodeMatcher(identity).Match(title)
}

func (matcher *EpisodeMatcher) Match(title string) (bool, bool, string) {
	identity := matcher.identity
	title = strings.NewReplacer("_", " ", ".", " ").Replace(title)
	if fanEditTag.MatchString(title) {
		return false, false, "rejected: fan edit"
	}
	matchTitle := leadingReleaseGroup.ReplaceAllString(title, "")
	normalized := " " + normalize(matchTitle) + " "
	// Recuts and spin-offs have numbering distinct from standard Naruto TV episodes.
	if narutoVariantTag.MatchString(normalized) {
		requestedRecut := false
		for _, alias := range identity.Titles {
			requestedRecut = requestedRecut || narutoVariantTag.MatchString(normalize(alias))
		}
		if !requestedRecut {
			return false, false, "rejected: Naruto variant numbering differs from TV episodes"
		}
	}

	multiSeasonPack := false
	if set := seasonSetTag.FindStringSubmatch(title); len(set) > 0 {
		for _, entry := range strings.Split(set[1]+set[2], "+") {
			if number(strings.TrimPrefix(strings.ToUpper(entry), "S")) == identity.SeasonNumber {
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
	if season == 0 {
		if m := wordSeason.FindStringSubmatch(title); len(m) > 1 {
			season = wordSeasonNumbers[strings.ToLower(m[1])]
		}
	}
	if season == 0 {
		if m := cjkSeason.FindStringSubmatch(title); len(m) > 1 {
			season = cjkSeasonNumber(m[1])
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
	for i, other := range identity.ExcludedTitles {
		base := matcher.excluded[i]
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
	if !identity.Standalone && movieTag.MatchString(title) &&
		(len(rangeMatch) == 0 || movieTag.FindStringIndex(title)[0] < releaseRange.FindStringIndex(title)[0]) {
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
	for _, alias := range matcher.aliases {
		base := alias.clean
		if base != "" && strings.Contains(normalized, " "+base+" ") {
			titleMatch = true
			matchedAlias = alias.key
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
		for _, alias := range matcher.aliases {
			if alias.original != alias.clean && strings.Contains(normalized, " "+alias.original+" ") {
				exact = true
			}
		}
		for _, alias := range matcher.unqualified {
			if alias != "" && strings.Contains(normalized, " "+alias+" ") {
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
	if m := cjkTotalEpisode.FindStringSubmatch(title); len(m) > 0 {
		abs := number(m[2])
		if m[1] != "" {
			if ep := number(m[1]); ep == target || (identity.AbsoluteEpisode > 0 && abs == identity.AbsoluteEpisode) {
				return true, false, fmt.Sprintf("matched: explicit episode %02d", ep)
			} else {
				return false, false, fmt.Sprintf("rejected: episode mismatch (got ep %d, want ep %d)", ep, target)
			}
		}
		if identity.AbsoluteEpisode > 0 && abs == identity.AbsoluteEpisode {
			return true, false, fmt.Sprintf("matched: explicit absolute episode %02d", abs)
		}
		return false, false, fmt.Sprintf("rejected: episode mismatch (got absolute %d, want ep %d)", abs, target)
	}
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
		if tail := taggedRangeTail.FindStringSubmatch(title[strings.Index(title, m[0])+len(m[0]):]); len(tail) > 1 {
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

	if season > 0 && len(rangeMatch) == 0 {
		if m := seasonBareEpisode.FindStringSubmatch(title); len(m) > 1 {
			if ep := number(m[1]); ep > 0 && ep != 480 && ep != 720 && ep != 1080 && ep != 2160 && (ep < 1900 || ep > 2099) {
				if ep == target {
					return true, false, fmt.Sprintf("matched: explicit episode %02d", ep)
				}
				return false, false, fmt.Sprintf("rejected: episode mismatch (got ep %d, want ep %d)", ep, target)
			}
		}
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
	for _, alias := range matcher.aliases {
		if alias.bare == nil {
			continue
		}
		if m := alias.bare.FindStringSubmatch(title); len(m) > 1 {
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

	// A numberless pack of the whole series (e.g. "Death Note - BDRIP - VF VOSTFR") is valid for
	// a first-season target: the player picks the episode file by name inside the pack.
	if season == 0 && identity.SeasonNumber <= 1 && len(rangeMatch) == 0 && completePackTag.MatchString(title) {
		return true, true, "matched: complete series pack (episode file chosen by name)"
	}

	if season > 0 && len(rangeMatch) == 0 {
		return true, true, fmt.Sprintf("candidate: season %d pack; episode file must be verified", season)
	}
	// Absence of numbering or language tags is not evidence of absence. Keep an
	// unqualified first-season release for the player's exact file/track checks.
	if season == 0 && identity.SeasonNumber <= 1 && identity.AllowUnqualified {
		return true, true, "candidate: unnumbered series release; episode file must be verified"
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
		// A MULTI release is not proof of a French track: it never earns French
		// points. LanguageTier still sorts it above English-only releases.
		breakdown.Multi = 2
	}
	animeTitle := ""
	if len(identity.Titles) > 0 {
		animeTitle = identity.Titles[0]
	}
	if item.IndexerOnly {
		// Also rewrites results cached before the provider was marked indexer-only.
		item.MagnetURI = PrivateMagnet(item)
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

// EpisodeSearchQueries searches season packs before individually tagged episodes.
func EpisodeSearchQueries(identity EpisodeIdentity) []string {
	episode := identity.EpisodeNumber
	if identity.TaggedEpisode > 0 {
		episode = identity.TaggedEpisode
	}
	queries := []string{}
	seen := map[string]bool{}
	add := func(q string) {
		if !seen[q] {
			seen[q] = true
			queries = append(queries, q)
		}
	}
	for i, base := range searchAliases(identity, 6) {
		if i >= 2 {
			// A single broad query covers alternate localized/acronym titles; avoid
			// multiplying every season and episode spelling across all aliases.
			add(base)
			continue
		}
		add(fmt.Sprintf("%s S%02d", base, identity.SeasonNumber))
		add(fmt.Sprintf("%s Season %d", base, identity.SeasonNumber))
		add(base)
		add(base + " batch")
		add(fmt.Sprintf("%s S%02dE%02d", base, identity.SeasonNumber, episode))
		add(fmt.Sprintf("%s %02d", base, episode))
		if identity.AbsoluteEpisode > 0 {
			add(fmt.Sprintf("%s %02d", base, identity.AbsoluteEpisode))
		}
	}
	return queries
}

// SeasonPlaybackOptions are shared by all episodes in a season, so provider
// caches can reuse discovery. Exact episode queries are reserved for fallback.
func SeasonPlaybackOptions(identity EpisodeIdentity) []SearchOptions {
	aliases := searchAliases(identity, 2)
	if len(aliases) == 0 {
		return nil
	}
	base := aliases[0]
	options := []SearchOptions{
		{Query: fmt.Sprintf("%s S%02d VF", base, identity.SeasonNumber), Category: "1_0"},
		{Query: base + " VF", Category: "1_0"},
		{Query: base + " MULTI", Category: "1_0"},
		{Query: fmt.Sprintf("%s S%02d", base, identity.SeasonNumber), Category: "1_0"},
		{Query: base + " VOSTFR", Category: "1_0"},
		{Query: base + " batch", Category: "1_0"},
	}
	if len(aliases) > 1 {
		// Romanised releases can omit their English catalog title entirely.
		options = append(options, SearchOptions{Query: aliases[1] + " VF", Category: "1_0"}, SearchOptions{Query: fmt.Sprintf("%s S%02d", aliases[1], identity.SeasonNumber), Category: "1_0"})
	}
	return options
}

// FrenchSearchOptions run independently so broad English packs cannot consume their budget.
func FrenchSearchOptions(identity EpisodeIdentity) []SearchOptions {
	aliases := searchAliases(identity, 6)
	options := []SearchOptions{}
	// All-anime queries include both translated categories and miscategorised packs.
	// Suffix-first ordering lets every alias reach VF before subtitle fallbacks.
	for _, suffix := range []string{"VF", "FRENCH", "MULTI", "VOSTFR"} {
		for i, base := range aliases {
			if suffix != "VF" && i >= 2 {
				continue
			}
			options = append(options, SearchOptions{Query: base + " " + suffix, Category: "1_0"})
		}
	}
	for _, base := range aliases {
		// Numberless releases without language tags remain candidates for track inspection.
		options = append(options, SearchOptions{Query: base, Category: "1_3"})
	}
	return options
}

// maxPacedQueries caps the queries one resolve sends to a rate-paced provider.
const maxPacedQueries = 3

// pacedBudget is the number of queries a resolve may send to rate-paced providers: what the slowest one
// can serve before the deadline (one call per MinInterval), capped at maxPacedQueries. ok is false
// when the indexer has no paced provider.
func pacedBudget(idx Provider, remaining time.Duration) (budget int, ok bool) {
	src, isSrc := idx.(interface{ PacedLimits() []Pacing })
	if !isSrc {
		return 0, false
	}
	limits := src.PacedLimits()
	if len(limits) == 0 {
		return 0, false
	}
	budget = maxPacedQueries
	for _, l := range limits {
		if l.MinInterval > 0 {
			budget = min(budget, int(remaining/l.MinInterval))
		}
	}
	return max(budget, 0), true
}

// PacedPriorityOptions are the queries worth a rate-paced provider's few requests, most valuable
// first. They mirror the French VF fallback role of the private tracker and reuse the existing query
// shapes: base title + season + VF, base title + VF, base title + MULTI (the first entries of
// SeasonPlaybackOptions), then the bare base title (as in FrenchSearchOptions).
func PacedPriorityOptions(identity EpisodeIdentity) []SearchOptions {
	options := SeasonPlaybackOptions(identity)
	if len(options) < 3 {
		return nil
	}
	out := append([]SearchOptions(nil), options[:3]...)
	for _, o := range FrenchSearchOptions(identity) {
		if o.Category == "1_3" {
			return append(out, o)
		}
	}
	return out
}

func searchAliases(identity EpisodeIdentity, limit int) []string {
	aliases := []string{}
	seen := map[string]bool{}
	for _, alias := range identity.Titles {
		base := CleanTitleForSearch(alias)
		key := normalize(base)
		if key != "" && latinSearchTitle.MatchString(base) && !seen[key] {
			seen[key] = true
			aliases = append(aliases, base)
			if len(aliases) == limit {
				break
			}
		}
	}
	if len(aliases) == 0 && len(identity.Titles) > 0 {
		aliases = append(aliases, CleanTitleForSearch(identity.Titles[0]))
	}
	return aliases
}

func (r *EpisodeResolver) ResolveSeasonSources(ctx context.Context, identity EpisodeIdentity) (*EpisodeSourcesResponse, error) {
	return r.resolveSeasonSources(ctx, identity, false)
}

// ResolvePlaybackSources starts with shared season-pack queries.
// A seeded VF or multilingual pack can be inspected before exhaustive discovery.
func (r *EpisodeResolver) ResolvePlaybackSources(ctx context.Context, identity EpisodeIdentity) (*EpisodeSourcesResponse, error) {
	return r.resolveSeasonSources(ctx, identity, true)
}

const (
	// A handful of seeded matches is enough for playback and failover; the rest of the discovery is skipped.
	enoughSources = 3
	enoughSeeders = 3
)

func (r *EpisodeResolver) resolveSeasonSources(ctx context.Context, identity EpisodeIdentity, playback bool) (*EpisodeSourcesResponse, error) {
	if len(identity.Titles) == 0 || identity.EpisodeNumber <= 0 {
		return nil, fmt.Errorf("invalid episode search arguments")
	}
	start := time.Now()
	matcher := NewEpisodeMatcher(identity)
	diagnostics.Log(ctx, slog.LevelInfo, "resolver.started", "titles", identity.Titles, "excluded_titles", identity.ExcludedTitles, "season_number", identity.SeasonNumber, "episode_number", identity.EpisodeNumber)
	queries := EpisodeSearchQueries(identity)
	var mu sync.Mutex
	items := map[string]TorrentItem{}
	failures, successes := 0, 0
	truncated := false
	searchParent, cancelSearch := context.WithTimeout(ctx, 20*time.Second)
	defer cancelSearch()
	var wg sync.WaitGroup
	phaseParent := searchParent
	searched := map[string]bool{}
	// stopWhenEnough ends the exhaustive generic phase as soon as a few seeded matches
	// exist: most of its ~20 query variants return nothing new and each costs a provider slot.
	stopWhenEnough := false
	enough := func() bool {
		mu.Lock()
		defer mu.Unlock()
		good := 0
		for _, item := range items {
			if item.Seeders < enoughSeeders {
				continue
			}
			if matched, _, _ := matcher.Match(item.Title); matched {
				good++
				if good >= enoughSources {
					return true
				}
			}
		}
		return false
	}
	runSearch := func(parent context.Context, opts SearchOptions, maxPages int) {
		searchCtx, cancel := context.WithTimeout(parent, 8*time.Second)
		defer cancel()
		querySeen := map[string]bool{}
		for page := 1; page <= maxPages; page++ {
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
			if page == maxPages && len(results) >= 75 {
				truncated = true
			}
			mu.Unlock()
			if len(results) < 75 {
				return
			}
		}
	}
	// pacing is set when a provider is rate-paced: fast, French and generic queries then skip it
	// (ScopeUnpaced) and it only gets the pacedPhase subset below.
	pacing := false
	if _, ok := pacedBudget(r.indexer, 0); ok {
		pacing = true
	}
	scoped := func(o SearchOptions) SearchOptions {
		if pacing {
			o.Scope = ScopeUnpaced
		}
		return o
	}
	runGroup := func(options []SearchOptions) {
		jobs := make(chan SearchOptions, len(options))
		for _, option := range options {
			option = scoped(option)
			key := option.Category + "|" + normalize(option.Query)
			if searched[key] {
				continue
			}
			searched[key] = true
			jobs <- option
		}
		close(jobs)
		for worker := 0; worker < 4; worker++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for option := range jobs {
					if phaseParent.Err() != nil {
						mu.Lock()
						truncated = true
						mu.Unlock()
						return
					}
					if stopWhenEnough && enough() {
						mu.Lock()
						truncated = true
						mu.Unlock()
						return
					}
					runSearch(phaseParent, option, 2)
				}
			}()
		}
	}
	// hasFrenchPack reports a seeded VF/MULTI batch match, which makes further discovery unnecessary.
	hasFrenchPack := func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, item := range items {
			if matched, batch, _ := matcher.Match(item.Title); matched && batch && item.Seeders > 0 && !item.IndexerOnly {
				tag, _, _ := ClassifyLanguage(item.Title)
				// MULTI remains unconfirmed, but its tracks can be checked while
				// exhaustive discovery is deferred until the player's fallback.
				if tag == LangVF || tag == LangMULTI {
					return true
				}
			}
		}
		return false
	}
	// pacedPhase sends a rate-paced provider its few most valuable queries, one at a time (a queue
	// would only time out), first page only, concurrently with the other providers' phases. It stops
	// as soon as a seeded French pack is known.
	pacedPhase := func() {
		if !pacing {
			return
		}
		remaining := 20 * time.Second
		if deadline, ok := searchParent.Deadline(); ok {
			remaining = time.Until(deadline)
		}
		budget, _ := pacedBudget(r.indexer, remaining)
		queue := PacedPriorityOptions(identity)
		if budget < len(queue) {
			queue = queue[:budget]
		}
		diagnostics.Log(ctx, slog.LevelDebug, "resolver.paced_budget", "budget", budget, "queries", len(queue))
		if len(queue) == 0 {
			return
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, option := range queue {
				if searchParent.Err() != nil || hasFrenchPack() {
					return
				}
				option.Scope = ScopePaced
				runSearch(searchParent, option, 1)
			}
		}()
	}
	generic := make([]SearchOptions, 0, len(queries))
	for _, query := range queries {
		generic = append(generic, SearchOptions{Query: query, Category: "1_0"})
	}
	frenchOptions := FrenchSearchOptions(identity)
	ready := false
	if playback {
		primary := SeasonPlaybackOptions(identity)
		fastCtx, cancelFast := context.WithTimeout(searchParent, r.fastPhase)
		phaseParent = fastCtx
		runGroup(primary)
		wg.Wait()
		cancelFast()
		searched = map[string]bool{}
		phaseParent = searchParent
		ready = hasFrenchPack()
		// We intentionally skipped discovery; do not claim exhaustive results.
		truncated = truncated || ready
	}
	if !ready {
		pacedPhase()
		runGroup(frenchOptions)
		wg.Wait()
		stopWhenEnough = true
		runGroup(generic)
	}

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
	// Indexer-only results come from a private tracker whose ratio playback consumes:
	// they are a last resort, kept only for a seeded VF when no public one exists.
	publicVF := false
	for _, item := range items {
		if matched, _, _ := matcher.Match(item.Title); matched && !item.IndexerOnly && item.Seeders > 0 {
			if tag, _, _ := ClassifyLanguage(item.Title); tag == LangVF {
				publicVF = true
				break
			}
		}
	}
	for _, item := range items {
		matches, batch, reason := matcher.Match(item.Title)
		if matches && item.IndexerOnly {
			if tag, _, _ := ClassifyLanguage(item.Title); tag != LangVF || item.Seeders == 0 {
				matches, reason = false, "rejected: private tracker kept only as a seeded VF fallback"
			} else if publicVF {
				matches, reason = false, "rejected: private VF unnecessary, a public VF exists"
			}
		}
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
	SortEpisodeSources(sources)
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

// SortEpisodeSources keeps offline audits and live playback ranking identical.
func SortEpisodeSources(sources []EpisodeSource) {
	sort.Slice(sources, func(i, j int) bool {
		a, b := sources[i], sources[j]
		if (a.Seeders > 0) != (b.Seeders > 0) {
			return a.Seeders > 0
		}
		if at, bt := LanguageTier(a.ScoreBreakdown), LanguageTier(b.ScoreBreakdown); at != bt {
			return at > bt
		}
		if a.IsBatch != b.IsBatch {
			return a.IsBatch
		}
		if a.ScoreBreakdown.Quality != b.ScoreBreakdown.Quality {
			return a.ScoreBreakdown.Quality > b.ScoreBreakdown.Quality
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
}
