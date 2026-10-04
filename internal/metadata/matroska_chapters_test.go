package metadata

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"testing"
)

var (
	idSeekHead        = []byte{0x11, 0x4D, 0x9B, 0x74}
	idSeek            = []byte{0x4D, 0xBB}
	idSeekID          = []byte{0x53, 0xAB}
	idSeekPosition    = []byte{0x53, 0xAC}
	idInfo            = []byte{0x15, 0x49, 0xA9, 0x66}
	idChapters        = []byte{0x10, 0x43, 0xA7, 0x70}
	idEditionEntry    = []byte{0x45, 0xB9}
	idEditionDefault  = []byte{0x45, 0xDB}
	idEditionOrdered  = []byte{0x45, 0xDD}
	idChapterAtom     = []byte{0xB6}
	idChapterStart    = []byte{0x91}
	idChapterEnd      = []byte{0x92}
	idChapterHidden   = []byte{0x98}
	idChapterEnabled  = []byte{0x45, 0x98}
	idChapterSegUID   = []byte{0x6E, 0x67}
	idChapterDisplay  = []byte{0x80}
	idChapString      = []byte{0x85}
	idChapLanguage    = []byte{0x43, 0x7C}
	chaptersIDAsBytes = []byte{0x10, 0x43, 0xA7, 0x70}
)

func ebmlUint(id []byte, n uint64) []byte {
	payload := make([]byte, 8)
	for i := 7; i >= 0; i-- {
		payload[i] = byte(n)
		n >>= 8
	}
	return ebmlElement(id, payload)
}

func cat(parts ...[]byte) []byte { return bytes.Join(parts, nil) }

const sec = 1_000_000_000

func atom(startSec, endSec int64, title string, extra ...[]byte) []byte {
	parts := [][]byte{ebmlUint(idChapterStart, uint64(startSec*sec))}
	if endSec >= 0 {
		parts = append(parts, ebmlUint(idChapterEnd, uint64(endSec*sec)))
	}
	if title != "" {
		parts = append(parts, display("eng", title))
	}
	parts = append(parts, extra...)
	return ebmlElement(idChapterAtom, cat(parts...))
}

func display(lang, title string) []byte {
	return ebmlElement(idChapterDisplay, cat(
		ebmlElement(idChapString, []byte(title)),
		ebmlElement(idChapLanguage, []byte(lang)),
	))
}

func edition(flags []byte, atoms ...[]byte) []byte {
	return ebmlElement(idEditionEntry, cat(flags, cat(atoms...)))
}

// mkv wraps segment children in an EBML header and a known-size Segment.
func mkv(children ...[]byte) []byte {
	return cat(ebmlElement(idEBML, []byte("webm")), ebmlElement(idSegment, cat(children...)))
}

func chaptersOf(t *testing.T, file []byte) []Chapter {
	t.Helper()
	got, err := readMatroskaChapters(context.Background(), bytes.NewReader(file), int64(len(file)))
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestReadChaptersFromHeader(t *testing.T) {
	file := mkv(
		ebmlElement(idInfo, []byte("info")),
		ebmlElement(idChapters, edition(nil,
			atom(1290, 1400, "Ending"),
			atom(0, 90, "Opening"),
			atom(90, 1290, "Part A"),
		)),
		ebmlElement(idCluster, bytes.Repeat([]byte{7}, 64)),
	)
	want := []Chapter{{0, 90, "Opening"}, {90, 1290, "Part A"}, {1290, 1400, "Ending"}}
	if got := chaptersOf(t, file); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

// guardReader fails any read that touches [lo, hi).
type guardReader struct {
	*bytes.Reader
	lo, hi int64
}

func (g *guardReader) Read(p []byte) (int, error) {
	pos := g.Reader.Size() - int64(g.Reader.Len())
	if pos < g.hi && pos+int64(len(p)) > g.lo {
		return 0, errors.New("read inside the cluster payload")
	}
	return g.Reader.Read(p)
}

func TestReadChaptersThroughSeekHead(t *testing.T) {
	info := ebmlElement(idInfo, []byte("info"))
	clusterPayload := bytes.Repeat([]byte{7}, 4096)
	cluster := ebmlElement(idCluster, clusterPayload)
	chapters := ebmlElement(idChapters, edition(nil, atom(0, 90, "Opening"), atom(90, 200, "Main")))
	seekHead := func(position uint64) []byte {
		return ebmlElement(idSeekHead, ebmlElement(idSeek, cat(
			ebmlElement(idSeekID, chaptersIDAsBytes),
			ebmlUint(idSeekPosition, position),
		)))
	}
	position := uint64(len(seekHead(0)) + len(info) + len(cluster))
	head := cat(ebmlElement(idEBML, []byte("webm")), ebmlElement(idSegment, cat(seekHead(position), info, cluster, chapters)))
	segmentStart := len(head) - len(seekHead(0)) - len(info) - len(cluster) - len(chapters)
	clusterStart := int64(segmentStart + len(seekHead(0)) + len(info))
	guard := &guardReader{Reader: bytes.NewReader(head), lo: clusterStart + int64(len(cluster)-len(clusterPayload)), hi: clusterStart + int64(len(cluster))}

	got, err := readMatroskaChapters(context.Background(), guard, int64(len(head)))
	if err != nil {
		t.Fatal(err)
	}
	want := []Chapter{{0, 90, "Opening"}, {90, 200, "Main"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestReadChaptersNone(t *testing.T) {
	file := mkv(ebmlElement(idInfo, []byte("info")), ebmlElement(idCluster, []byte("data")))
	if got := chaptersOf(t, file); got != nil {
		t.Fatalf("got %+v, want nil", got)
	}
}

func TestReadChaptersSkipsHiddenDisabledAndLinked(t *testing.T) {
	file := mkv(ebmlElement(idChapters, edition(nil,
		atom(0, 10, "Keep"),
		atom(10, 20, "Hidden", ebmlUint(idChapterHidden, 1)),
		atom(20, 30, "Disabled", ebmlUint(idChapterEnabled, 0)),
		atom(30, 40, "Linked", ebmlElement(idChapterSegUID, bytes.Repeat([]byte{1}, 16))),
		atom(40, 50, "Enabled", ebmlUint(idChapterEnabled, 1)),
	)))
	want := []Chapter{{0, 10, "Keep"}, {40, 50, "Enabled"}}
	if got := chaptersOf(t, file); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestReadChaptersEditions(t *testing.T) {
	def := ebmlUint(idEditionDefault, 1)
	ordered := ebmlUint(idEditionOrdered, 1)
	cases := map[string]struct {
		editions [][]byte
		want     []Chapter
	}{
		"default wins": {
			[][]byte{edition(nil, atom(0, 5, "First")), edition(def, atom(0, 7, "Second"))},
			[]Chapter{{0, 7, "Second"}},
		},
		"first when no default": {
			[][]byte{edition(nil, atom(0, 5, "First")), edition(nil, atom(0, 7, "Second"))},
			[]Chapter{{0, 5, "First"}},
		},
		"ordered ignored": {
			[][]byte{edition(ordered, atom(0, 5, "Ordered")), edition(nil, atom(0, 7, "Plain"))},
			[]Chapter{{0, 7, "Plain"}},
		},
		"all ordered": {
			[][]byte{edition(ordered, atom(0, 5, "A")), edition(cat(def, ordered), atom(0, 7, "B"))},
			nil,
		},
	}
	for name, tc := range cases {
		file := mkv(ebmlElement(idChapters, cat(tc.editions...)))
		if got := chaptersOf(t, file); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %+v, want %+v", name, got, tc.want)
		}
	}
}

func TestReadChaptersPrefersEnglishTitle(t *testing.T) {
	both := ebmlElement(idChapterAtom, cat(ebmlUint(idChapterStart, 0), display("jpn", "オープニング"), display("eng", "Opening")))
	only := ebmlElement(idChapterAtom, cat(ebmlUint(idChapterStart, 5*sec), display("jpn", "第一"), display("fre", "Premier")))
	file := mkv(ebmlElement(idChapters, edition(nil, both, only)))
	got := chaptersOf(t, file)
	if len(got) != 2 || got[0].Title != "Opening" || got[1].Title != "第一" {
		t.Fatalf("got %+v", got)
	}
}

func TestReadChaptersMissingEnd(t *testing.T) {
	file := mkv(ebmlElement(idChapters, edition(nil, atom(0, -1, "A"), atom(60, -1, "B"))))
	want := []Chapter{{0, 60, "A"}, {60, 0, "B"}}
	if got := chaptersOf(t, file); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestReadChaptersNestedAtomsIgnored(t *testing.T) {
	nested := atom(10, 20, "Nested")
	parent := ebmlElement(idChapterAtom, cat(ebmlUint(idChapterStart, 0), ebmlUint(idChapterEnd, 30*sec), display("eng", "Parent"), nested))
	file := mkv(ebmlElement(idChapters, edition(nil, parent)))
	want := []Chapter{{0, 30, "Parent"}}
	if got := chaptersOf(t, file); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestReadChaptersTruncated(t *testing.T) {
	file := mkv(ebmlElement(idChapters, edition(nil, atom(0, 90, "Opening"))))
	file = file[:len(file)-10]
	if _, err := readMatroskaChapters(context.Background(), bytes.NewReader(file), int64(len(file))); err == nil {
		t.Fatal("expected an error for a Chapters element past the end of the file")
	}
}

func TestReadChaptersTooLarge(t *testing.T) {
	file := mkv(ebmlElement(idChapters, bytes.Repeat([]byte{0}, 600<<10)))
	if _, err := readMatroskaChapters(context.Background(), bytes.NewReader(file), int64(len(file))); err == nil {
		t.Fatal("expected an error for an oversized Chapters element")
	}
}

func TestReadChaptersNotMatroska(t *testing.T) {
	file := []byte("\x00\x00\x00\x18ftypmp42 plain mp4 bytes")
	got, err := readMatroskaChapters(context.Background(), bytes.NewReader(file), int64(len(file)))
	if got != nil || err != nil {
		t.Fatalf("got %+v, %v; want nil, nil", got, err)
	}
}

type contextualReader struct{ *bytes.Reader }

func (c contextualReader) ReadContext(ctx context.Context, p []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return c.Reader.Read(p)
}

func TestReadChaptersCancelled(t *testing.T) {
	file := mkv(ebmlElement(idChapters, edition(nil, atom(0, 90, "Opening"))))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := readMatroskaChapters(ctx, contextualReader{bytes.NewReader(file)}, int64(len(file)))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

var _ io.ReadSeeker = contextualReader{}
