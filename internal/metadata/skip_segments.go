package metadata

import (
	"sort"
	"strings"
	"unicode"
)

// SkipSegment is a span of an episode the player can offer to skip.
type SkipSegment struct {
	Kind   string  `json:"kind"`   // "opening" | "ending"
	Start  float64 `json:"start"`  // seconds
	End    float64 `json:"end"`    // seconds
	Source string  `json:"source"` // "chapters" | "aniskip"
}

const (
	SkipKindOpening = "opening"
	SkipKindEnding  = "ending"

	skipSourceChapters = "chapters"

	skipMinFileSec      = 300
	skipTitledMinSec    = 20
	skipTitledMaxSec    = 180
	skipHeuristicMinSec = 75
	skipHeuristicMaxSec = 110
)

var (
	openingKeywords = []string{
		"op", "op1", "op2", "op3", "op4", "op5", "op6", "op7", "op8", "op9",
		"opening", "intro", "introduction", "ouverture",
		"générique de début", "generique de debut", "opening song", "オープニング",
	}
	endingKeywords = []string{
		"ed", "ed1", "ed2", "ed3", "ed4", "ed5", "ed6", "ed7", "ed8", "ed9",
		"ending", "outro", "credits",
		"générique de fin", "generique de fin", "ending song", "エンディング",
	}
)

// DetectSkipSegments finds the opening and ending of an episode from its
// chapters. When in doubt it returns nothing: a wrong button is worse than none.
func DetectSkipSegments(chapters []Chapter, duration float64) []SkipSegment {
	if duration < skipMinFileSec {
		return nil
	}

	type chap struct {
		Chapter
		isOP, isED bool
	}
	var list []chap
	for _, c := range chapters {
		if c.End <= 0 {
			c.End = duration
		}
		if c.End-c.Start <= 0 {
			continue
		}
		// A chapter covering over half the file is never a skip target.
		if c.End-c.Start > duration/2 {
			continue
		}
		list = append(list, chap{c, matchesKeyword(c.Title, openingKeywords), matchesKeyword(c.Title, endingKeywords)})
	}

	var opening, ending *SkipSegment
	var openingTitled, endingTitled bool

	// Step 1: titles.
	for _, c := range list {
		if c.isOP == c.isED {
			continue
		}
		d := c.End - c.Start
		if d < skipTitledMinSec || d > skipTitledMaxSec {
			continue
		}
		seg := SkipSegment{Start: c.Start, End: c.End, Source: skipSourceChapters}
		if c.isOP {
			if opening == nil {
				seg.Kind = SkipKindOpening
				opening = &seg
				openingTitled = true
			}
		} else {
			seg.Kind = SkipKindEnding
			ending = &seg
			endingTitled = true
		}
	}

	// Step 2: length and position, on chapters whose title matches no list.
	for _, c := range list {
		if c.isOP || c.isED {
			continue
		}
		d := c.End - c.Start
		if d < skipHeuristicMinSec || d > skipHeuristicMaxSec {
			continue
		}
		if !openingTitled && opening == nil && c.Start < duration/3 {
			opening = &SkipSegment{Kind: SkipKindOpening, Start: c.Start, End: c.End, Source: skipSourceChapters}
		}
		if c.Start > duration*0.75 {
			// Last candidate wins, unless a titled ending was found.
			if !endingTitled {
				ending = &SkipSegment{Kind: SkipKindEnding, Start: c.Start, End: c.End, Source: skipSourceChapters}
			}
		}
	}

	if opening != nil && ending != nil && opening.End > ending.Start {
		return nil
	}
	var out []SkipSegment
	if opening != nil {
		out = append(out, *opening)
	}
	if ending != nil {
		out = append(out, *ending)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out
}

// matchesKeyword reports whether title contains one of the keywords as a whole
// word, case-insensitively. CJK keywords have no word boundaries and match as
// substrings.
func matchesKeyword(title string, keywords []string) bool {
	t := strings.ToLower(title)
	for _, k := range keywords {
		if strings.IndexFunc(k, func(r rune) bool { return r > 0x3000 }) >= 0 {
			if strings.Contains(t, k) {
				return true
			}
			continue
		}
		for from := 0; ; {
			i := strings.Index(t[from:], k)
			if i < 0 {
				break
			}
			i += from
			end := i + len(k)
			if boundaryBefore(t, i) && boundaryAfter(t, end) {
				return true
			}
			from = i + 1
		}
	}
	return false
}

func boundaryBefore(s string, i int) bool {
	if i == 0 {
		return true
	}
	r := []rune(s[:i])
	return !isWordRune(r[len(r)-1])
}

func boundaryAfter(s string, i int) bool {
	if i >= len(s) {
		return true
	}
	for _, r := range s[i:] {
		return !isWordRune(r)
	}
	return true
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }
