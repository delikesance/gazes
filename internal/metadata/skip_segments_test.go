package metadata

import (
	"reflect"
	"testing"
)

func TestDetectSkipSegments(t *testing.T) {
	ch := func(start, end float64, title string) Chapter { return Chapter{Start: start, End: end, Title: title} }
	op := func(s, e float64) SkipSegment { return SkipSegment{"opening", s, e, "chapters"} }
	ed := func(s, e float64) SkipSegment { return SkipSegment{"ending", s, e, "chapters"} }
	titled := func(o, e string) []Chapter {
		return []Chapter{ch(0, 120, "Prologue"), ch(120, 210, o), ch(210, 700, "Part A"), ch(700, 1290, "Part B"), ch(1290, 1380, e), ch(1380, 1420, "Preview")}
	}
	both := []SkipSegment{op(120, 210), ed(1290, 1380)}

	tests := []struct {
		name     string
		chapters []Chapter
		duration float64
		want     []SkipSegment
	}{
		{"english titles", titled("Opening", "Ending"), 1420, both},
		{"OP1/ED1", titled("OP1", "ED1"), 1420, both},
		{"french", titled("Générique de début", "Générique de fin"), 1420, both},
		{"japanese", titled("オープニング", "エンディング"), 1420, both},
		// 150 s is in the titled range but outside the heuristic one: a false keyword match would yield a segment.
		{"Opera is not op", []Chapter{ch(0, 120, "A"), ch(120, 270, "Opera"), ch(270, 1420, "B")}, 1420, nil},
		{"Episode is not ed", []Chapter{ch(0, 1000, "A"), ch(1000, 1130, "B"), ch(1130, 1270, "Episode"), ch(1270, 1420, "Edited")}, 1420, nil},
		{"Credited is not credits", []Chapter{ch(0, 1000, "A"), ch(1000, 1130, "B"), ch(1130, 1270, "C"), ch(1270, 1420, "Credited")}, 1420, nil},
		{"generic both", []Chapter{ch(0, 150, "Chapter 01"), ch(150, 240, "Chapter 02"), ch(240, 1290, "Chapter 03"), ch(1290, 1380, "Chapter 04"), ch(1380, 1420, "Chapter 05")}, 1420, both2()},
		{"generic opening only", []Chapter{ch(0, 89, "Chapter 01"), ch(89, 1420, "Chapter 02")}, 1420, []SkipSegment{op(0, 89)}},
		{"generic no ending candidate", []Chapter{ch(0, 150, "Chapter 01"), ch(150, 240, "Chapter 02"), ch(240, 1290, "Chapter 03"), ch(1290, 1420, "Chapter 04")}, 1420, []SkipSegment{op(150, 240)}},
		{"parts", []Chapter{ch(0, 700, "Part A"), ch(700, 1420, "Part B")}, 1420, nil},
		{"mid-file 90s", []Chapter{ch(0, 600, "A"), ch(600, 690, "B"), ch(690, 1420, "C")}, 1420, nil},
		{"movie", []Chapter{ch(0, 100, "Opening"), ch(100, 3500, "A"), ch(3500, 3590, "B"), ch(3590, 7200, "C")}, 7200, []SkipSegment{op(0, 100)}},
		{"short file", []Chapter{ch(0, 90, "Opening"), ch(90, 180, "Main")}, 180, nil},
		{"single opening chapter", []Chapter{ch(0, 1420, "Opening")}, 1420, nil},
		{"OP / ED ambiguous", []Chapter{ch(0, 100, "OP / ED"), ch(100, 1420, "Main")}, 1420, nil},
		{"titled 15s", []Chapter{ch(0, 15, "Opening"), ch(15, 700, "A"), ch(700, 1420, "B")}, 1420, nil},
		{"titled 200s", []Chapter{ch(0, 200, "Opening"), ch(200, 700, "A"), ch(700, 1420, "B")}, 1420, nil},
		{"opening after ending", []Chapter{ch(0, 100, "A"), ch(100, 190, "Ending"), ch(190, 1200, "B"), ch(1200, 1290, "Opening"), ch(1290, 1420, "C")}, 1420, nil},
		{"last chapter without end", []Chapter{ch(0, 1330, "Main"), ch(1330, 0, "Ending")}, 1420, []SkipSegment{ed(1330, 1420)}},
		{"zero duration", titled("Opening", "Ending"), 0, nil},
		{"titled opening + generic ending", []Chapter{ch(0, 100, "Opening"), ch(100, 1290, "A"), ch(1290, 1380, "Chapter 9"), ch(1380, 1420, "B")}, 1420, both3()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DetectSkipSegments(tt.chapters, tt.duration); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func both2() []SkipSegment {
	return []SkipSegment{{"opening", 150, 240, "chapters"}, {"ending", 1290, 1380, "chapters"}}
}

func both3() []SkipSegment {
	return []SkipSegment{{"opening", 0, 100, "chapters"}, {"ending", 1290, 1380, "chapters"}}
}

func TestKeywordMatching(t *testing.T) {
	const (
		none = ""
		o    = "opening"
		e    = "ending"
	)
	tests := []struct{ title, want string }{
		{"Episode", none}, {"Opera", none}, {"Edited", none}, {"Credited", none},
		{"Introspection", none}, {"Operation", none}, {"Main", none},
		{"OP1", o}, {"op-1", o}, {"Opening", o}, {"Opening Song", o}, {"OP", o}, {"Intro", o},
		{"Générique de début", o}, {"オープニング", o},
		{"(ED)", e}, {"ED", e}, {"ED2", e}, {"ed 1", e}, {"Credits", e}, {"Outro", e},
		{"Générique de fin", e}, {"エンディング", e},
	}
	for _, tt := range tests {
		got := none
		if matchesKeyword(tt.title, openingKeywords) {
			got = o
		}
		if matchesKeyword(tt.title, endingKeywords) {
			if got != none {
				t.Fatalf("%q matched both lists", tt.title)
			}
			got = e
		}
		if got != tt.want {
			t.Errorf("%q: got %q, want %q", tt.title, got, tt.want)
		}
	}
}
