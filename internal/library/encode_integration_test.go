//go:build ffmpeg

package library

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gazes/gazes/internal/playback"
)

const assFixture = `[Script Info]
ScriptType: v4.00+
PlayResX: 1280
PlayResY: 720

[V4+ Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding
Style: Default,DejaVu Sans,36,&H00FFFFFF,&H000000FF,&H00000000,&H00000000,0,0,0,0,100,100,0,0,1,2,0,2,10,10,10,1

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
Dialogue: 0,0:00:00.50,0:00:04.00,Default,,0,0,0,,Hello library
`

func run(t *testing.T, name string, args ...string) []byte {
	t.Helper()
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return out
}

func TestRealAV1EncodeKeepsTracksAndIsHLSIndexable(t *testing.T) {
	for _, bin := range []string{"ffmpeg", "ffprobe", "fc-match"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not installed", bin)
		}
	}
	font := strings.TrimSpace(string(run(t, "fc-match", "-f", "%{file}", "DejaVu Sans")))
	if font == "" || !strings.HasSuffix(strings.ToLower(font), ".ttf") {
		t.Skipf("no TTF system font (fc-match gave %q)", font)
	}

	pool, store, _ := newPoolTest(t, fakeFS{"d1": {1 << 40, 1 << 39}}, 0, 0, "d1")
	if _, _, err := pool.Scan(); err != nil {
		t.Fatal(err)
	}
	disk := pool.Disks()[0]
	rel := "7/1-vostfr.mkv"
	src := filepath.Join(disk.Path, rel)
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	ass := filepath.Join(t.TempDir(), "subs.ass")
	if err := os.WriteFile(ass, []byte(assFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, "ffmpeg", "-y", "-v", "error",
		"-f", "lavfi", "-i", "testsrc2=size=1280x720:rate=24:duration=5",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=5",
		"-f", "lavfi", "-i", "sine=frequency=880:duration=5",
		"-i", ass,
		"-map", "0:v", "-map", "1:a", "-map", "2:a", "-map", "3:s",
		"-c:v", "libx264", "-crf", "18", "-g", "24", "-pix_fmt", "yuv420p",
		"-c:a:0", "aac", "-ac:a:0", "2", "-c:a:1", "ac3", "-ac:a:1", "6",
		"-c:s", "ass",
		"-attach", font, "-metadata:s:t:0", "mimetype=application/x-truetype-font",
		src)

	k := Key{SeasonID: 7, Episode: 1, Lang: "vostfr"}
	if err := store.Create(Entry{Key: k, AnimeID: 1, Title: "T", State: StateOriginal, DiskID: disk.ID, RelPath: rel}); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(src)
	if _, err := store.Update(k, func(e *Entry) error { e.SizeBytes, e.OriginalSizeBytes = st.Size(), st.Size(); return nil }); err != nil {
		t.Fatal(err)
	}

	enc := NewEncoder(store, pool, FFprobe("ffprobe"), nil, EncodeSettings{Preset: 8, CRF: 30, Threads: 4}, nil)
	ctx := context.Background()
	if !enc.encodeNext(ctx) {
		t.Fatal("encoder did no work")
	}
	e, err := store.Get(k)
	if err != nil {
		t.Fatal(err)
	}
	if e.State != StateAV1 {
		t.Fatalf("state %s (skipped %q, err %q)", e.State, e.EncodeSkipped, e.LastError)
	}
	if err := Verify(ctx, FFprobe("ffprobe"), "ffmpeg", src, src); err != nil {
		t.Fatalf("Verify of the final file: %v", err)
	}

	var doc struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
			CodecName string `json:"codec_name"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(run(t, "ffprobe", "-v", "error", "-print_format", "json", "-show_streams", src), &doc); err != nil {
		t.Fatal(err)
	}
	var video, audio, subs, attach []string
	for _, s := range doc.Streams {
		switch s.CodecType {
		case "video":
			video = append(video, s.CodecName)
		case "audio":
			audio = append(audio, s.CodecName)
		case "subtitle":
			subs = append(subs, s.CodecName)
		case "attachment":
			attach = append(attach, s.CodecName)
		}
	}
	eq := func(got []string, want ...string) bool { return strings.Join(got, ",") == strings.Join(want, ",") }
	if !eq(video, "av1") || !eq(audio, "aac", "opus") || !eq(subs, "ass") || len(attach) != 1 {
		t.Fatalf("streams: video=%v audio=%v subs=%v attach=%v", video, audio, subs, attach)
	}

	f, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	fi, _ := f.Stat()
	idx, err := playback.ReadIndex(ctx, f, fi.Size())
	if err != nil {
		t.Fatalf("ReadIndex: %v", err)
	}
	if len(idx.Segments) < 1 {
		t.Fatalf("index has no segments: %+v", idx)
	}
}

// An MP4 (stored under the library's .mkv name) can carry cover art as an attached-picture video
// stream; it must neither be encoded as a second AV1 video stream nor make Verify fail.
func TestRealAV1EncodeDropsAttachedPicture(t *testing.T) {
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not installed", bin)
		}
	}
	pool, store, _ := newPoolTest(t, fakeFS{"d1": {1 << 40, 1 << 39}}, 0, 0, "d1")
	if _, _, err := pool.Scan(); err != nil {
		t.Fatal(err)
	}
	disk := pool.Disks()[0]
	rel := "7/1-vostfr.mkv"
	src := filepath.Join(disk.Path, rel)
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	cover := filepath.Join(t.TempDir(), "cover.png")
	run(t, "ffmpeg", "-y", "-v", "error", "-f", "lavfi", "-i", "color=c=red:s=64x64", "-frames:v", "1", cover)
	run(t, "ffmpeg", "-y", "-v", "error",
		"-f", "lavfi", "-i", "testsrc2=size=1280x720:rate=24:duration=3",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=3",
		"-i", cover,
		"-map", "0:v", "-map", "1:a", "-map", "2:v",
		"-c:v:0", "libx264", "-crf", "18", "-pix_fmt", "yuv420p", "-c:a", "aac",
		"-c:v:1", "copy", "-disposition:v:1", "attached_pic",
		"-f", "mp4", src)
	probe := func(path string) string {
		return string(run(t, "ffprobe", "-v", "error", "-select_streams", "v", "-show_entries", "stream=codec_name:stream_disposition=attached_pic", "-of", "csv=p=0", path))
	}
	if got := probe(src); got != "h264,0\npng,1\n" {
		t.Fatalf("fixture lacks an attached picture: %q", got)
	}

	k := Key{SeasonID: 7, Episode: 1, Lang: "vostfr"}
	st, _ := os.Stat(src)
	if err := store.Create(Entry{Key: k, AnimeID: 1, Title: "T", State: StateOriginal, DiskID: disk.ID, RelPath: rel,
		SizeBytes: st.Size(), OriginalSizeBytes: st.Size()}); err != nil {
		t.Fatal(err)
	}
	enc := NewEncoder(store, pool, FFprobe("ffprobe"), nil, EncodeSettings{Preset: 8, CRF: 30, Threads: 4}, nil)
	if !enc.encodeNext(context.Background()) {
		t.Fatal("encoder did no work")
	}
	e, err := store.Get(k)
	if err != nil {
		t.Fatal(err)
	}
	if e.State != StateAV1 {
		t.Fatalf("state %s (skipped %q, err %q)", e.State, e.EncodeSkipped, e.LastError)
	}
	if got := probe(src); got != "av1,0\n" {
		t.Fatalf("video streams after encode: %q, want only the real one as av1", got)
	}
}
