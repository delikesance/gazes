package library

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildArgsCopiesAACAndOpusTranscodesOthers(t *testing.T) {
	info := MediaInfo{AudioCodecs: []string{"aac", "ac3", "flac"}, AudioChannels: []int{2, 6, 2}}
	args := BuildArgs("in.mkv", "out.mkv", info, EncodeSettings{Preset: 8, CRF: 30, Threads: 8})
	line := " " + strings.Join(args, " ") + " "
	for _, want := range []string{
		" -i in.mkv ", " -c:v libsvtav1 ", " -preset 8 ", " -crf 30 ", " -pix_fmt yuv420p10le ", " -svtav1-params lp=8 ",
		" -map 0:V ", " -map 0:a? ", " -map 0:s? ", " -map 0:t? ", " -map -0:d? ", " -c:s copy ", " -c:t copy ", " -map_chapters 0 ", " -map_metadata 0 ",
		" -c:a:0 copy ", " -c:a:1 libopus -b:a:1 256k ", " -c:a:2 libopus -b:a:2 128k ", " -progress pipe:1 ",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("missing %q in %s", want, line)
		}
	}
	if strings.Contains(line, " -map 0 ") {
		t.Errorf("must not map every stream (attached pictures would be encoded): %s", line)
	}
	if strings.Contains(line, "-b:a:0") {
		t.Errorf("copied track must not get a bitrate: %s", line)
	}
	if args[len(args)-1] != "out.mkv" {
		t.Errorf("output must be last, got %v", args[len(args)-1])
	}
	if args[0] == "nice" || args[0] == "ffmpeg" {
		t.Errorf("BuildArgs must not include the binary or nice: %v", args[:2])
	}
}

func TestBuildArgsOpusIsCopied(t *testing.T) {
	args := BuildArgs("a", "b", MediaInfo{AudioCodecs: []string{"opus"}, AudioChannels: []int{6}}, EncodeSettings{})
	line := strings.Join(args, " ")
	if !strings.Contains(line, "-c:a:0 copy") || !strings.Contains(line, "-preset 8 -crf 30") || !strings.Contains(line, "lp=8") {
		t.Fatalf("%s", line)
	}
}

func TestInWindow(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 1, 1, h, m, 0, 0, time.UTC) }
	cases := []struct {
		win  string
		now  time.Time
		want bool
	}{
		{"", at(12, 0), true},
		{"01:00-09:00", at(0, 59), false},
		{"01:00-09:00", at(1, 0), true},
		{"01:00-09:00", at(8, 59), true},
		{"01:00-09:00", at(9, 0), false},
		{"22:00-06:00", at(23, 0), true},
		{"22:00-06:00", at(5, 0), true},
		{"22:00-06:00", at(12, 0), false},
	}
	for _, c := range cases {
		got, err := InWindow(c.win, c.now)
		if err != nil || got != c.want {
			t.Errorf("InWindow(%q, %s) = %v, %v; want %v", c.win, c.now.Format("15:04"), got, err, c.want)
		}
	}
	for _, bad := range []string{"nonsense", "25:00-06:00", "01:00", "aa:bb-cc:dd"} {
		if _, err := InWindow(bad, at(1, 0)); err == nil {
			t.Errorf("InWindow(%q) accepted", bad)
		}
	}
}

type mapProber map[string]MediaInfo

func (m mapProber) Probe(ctx context.Context, path string) (MediaInfo, error) {
	return m[filepath.Base(path)], nil
}

func writeFile(t *testing.T, dir, name string, size int) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestVerifyRejectsDurationAndTrackMismatch(t *testing.T) {
	dir := t.TempDir()
	orig, enc := writeFile(t, dir, "o.mkv", 10), writeFile(t, dir, "e.mkv", 5)
	ff := fakeFFmpeg(t, "exit 0\n")
	base := MediaInfo{DurationMS: 100000, VideoStreams: 1, AudioCodecs: []string{"aac", "opus"}, AudioChannels: []int{2, 2}, SubtitleTracks: 2}
	check := func(e MediaInfo) error {
		return Verify(context.Background(), mapProber{"o.mkv": base, "e.mkv": e}, ff, orig, enc)
	}
	if err := check(base); err != nil {
		t.Fatalf("identical info rejected: %v", err)
	}
	near := base
	near.DurationMS += 1000
	if err := check(near); err != nil {
		t.Fatalf("1 s drift rejected: %v", err)
	}
	long := base
	long.DurationMS += 2000
	if err := check(long); err == nil {
		t.Fatal("2 s drift accepted")
	}
	fewAudio := base
	fewAudio.AudioCodecs, fewAudio.AudioChannels = []string{"aac"}, []int{2}
	if err := check(fewAudio); err == nil {
		t.Fatal("missing audio track accepted")
	}
	twoVideo := base
	twoVideo.VideoStreams = 2
	if err := check(twoVideo); err == nil {
		t.Fatal("2 video streams accepted")
	}
	noVideo := base
	noVideo.VideoStreams = 0
	if err := check(noVideo); err == nil {
		t.Fatal("no video stream accepted")
	}
	fewSubs := base
	fewSubs.SubtitleTracks = 1
	if err := check(fewSubs); err == nil {
		t.Fatal("missing subtitle track accepted")
	}
}

func TestVerifyRejectsEmptyFileAndDecodeFailure(t *testing.T) {
	dir := t.TempDir()
	orig := writeFile(t, dir, "o.mkv", 10)
	info := MediaInfo{DurationMS: 60000, VideoStreams: 1}
	p := mapProber{"o.mkv": info, "e.mkv": info}
	empty := writeFile(t, dir, "e.mkv", 0)
	if err := Verify(context.Background(), p, fakeFFmpeg(t, "exit 0\n"), orig, empty); err == nil {
		t.Fatal("empty file accepted")
	}
	enc := writeFile(t, dir, "e.mkv", 5)
	if err := Verify(context.Background(), p, failingFFmpeg(t), orig, enc); err == nil {
		t.Fatal("decode failure accepted")
	}
}

func failingFFmpeg(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(p, []byte("#!/bin/sh\necho corrupt >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}
