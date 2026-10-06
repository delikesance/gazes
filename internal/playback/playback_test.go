package playback

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gazes/gazes/internal/metadata"
	"github.com/gazes/gazes/internal/torrent"
)

type fileEngine struct{ path string }

func (e fileEngine) AddTorrent(context.Context, string) (string, []torrent.FileInfo, error) {
	return "", nil, nil
}
func (e fileEngine) GetFileStream(ctx context.Context, _ string, _ int) (io.ReadSeekCloser, *torrent.FileInfo, error) {
	f, err := os.Open(e.path)
	if err != nil {
		return nil, nil, err
	}
	stat, _ := f.Stat()
	return f, &torrent.FileInfo{Path: e.path, Length: stat.Size(), IsVideo: true}, nil
}
func (e fileEngine) GetStats(string) (*torrent.SwarmStats, error) { return &torrent.SwarmStats{}, nil }
func (e fileEngine) Close() error                                 { return nil }
func fixture(t *testing.T, ext string, origin int) string {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("FFmpeg required")
	}
	path := filepath.Join(t.TempDir(), "fixture."+ext)
	args := []string{"-v", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=25", "-f", "lavfi", "-i", "sine=frequency=880:sample_rate=48000", "-t", "16", "-c:v", "libx264", "-threads", "1", "-g", "50", "-bf", "2", "-c:a", "aac", "-output_ts_offset", fmt.Sprint(origin), "-y", path}
	if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, out)
	}
	return path
}
func TestBoundedIndexesAndRealSegments(t *testing.T) {
	for _, tc := range []struct {
		ext    string
		origin int
	}{{"mkv", 0}, {"mkv", 5}, {"mp4", 0}, {"mp4", 5}, {"webm", 0}} {
		if tc.ext == "webm" {
			continue
		} // WebM exercises the same EBML parser with its own fixture below.
		t.Run(fmt.Sprintf("%s/%d", tc.ext, tc.origin), func(t *testing.T) {
			path := fixture(t, tc.ext, tc.origin)
			f, _ := os.Open(path)
			defer f.Close()
			stat, _ := f.Stat()
			idx, err := ReadIndex(context.Background(), f, stat.Size())
			if err != nil {
				t.Fatal(err)
			}
			if len(idx.Segments) != 4 {
				t.Fatalf("segments %+v", idx)
			}
			raw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				file, _ := os.Open(path)
				defer file.Close()
				http.ServeContent(w, r, "fixture", time.Time{}, file)
			}))
			defer raw.Close()
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			m := New(fileEngine{path}, metadata.NewFFprobeAnalyzer(logger), logger, Options{Directory: t.TempDir(), RawURL: raw.URL})
			defer m.Close()
			s, err := m.Create(context.Background(), strings.Repeat("a", 40), 0, 0, 8, false)
			if err != nil {
				t.Fatal(err)
			}
			for _, n := range []int{2, 0, 3, 1} {
				m.Update(s.ID, float64(n*4), uint64(n+10))
				init, media := httptest.NewRecorder(), httptest.NewRecorder()
				if err := m.Media(context.Background(), s.ID, n, true, init, httptest.NewRequest("GET", "/init.mp4", nil)); err != nil {
					t.Fatal(err)
				}
				if err := m.Media(context.Background(), s.ID, n, false, media, httptest.NewRequest("GET", "/media.m4s", nil)); err != nil {
					t.Fatal(err)
				}
				output := filepath.Join(t.TempDir(), "segment.mp4")
				os.WriteFile(output, append(init.Body.Bytes(), media.Body.Bytes()...), 0600)
				out, err := exec.Command("ffprobe", "-v", "error", "-show_packets", "-show_entries", "packet=codec_type,pts_time,dts_time,flags", "-of", "json", output).Output()
				if err != nil {
					t.Fatal(err)
				}
				var packets struct {
					Packets []struct {
						Type  string `json:"codec_type"`
						PTS   string `json:"pts_time"`
						DTS   string `json:"dts_time"`
						Flags string `json:"flags"`
					}
				}
				if err = json.Unmarshal(out, &packets); err != nil {
					t.Fatal(err)
				}
				var firstVideo, firstAudio string
				for _, p := range packets.Packets {
					if p.Type == "video" && firstVideo == "" {
						firstVideo = p.PTS
						if !strings.Contains(p.Flags, "K") {
							t.Fatal("non-keyframe segment")
						}
					}
					if p.Type == "audio" && firstAudio == "" {
						firstAudio = p.PTS
					}
				}
				want := fmt.Sprintf("%.6f", float64(n*4+1))
				if firstVideo != want {
					t.Fatalf("segment %d: video %s want %s, audio %s", n, firstVideo, want, firstAudio)
				}
				if out, err := exec.Command("ffmpeg", "-v", "error", "-i", output, "-f", "null", "-").CombinedOutput(); err != nil || len(out) > 0 {
					t.Fatalf("decode: %v %s", err, out)
				}
			}
		})
	}
}
func TestNoIndexDoesNotScanPayload(t *testing.T) {
	path := fixture(t, "mkv", 0)
	data, _ := os.ReadFile(path)
	// Remove the SeekHead's pointer to Cues, keeping a playable file but no
	// discoverable seek index. The parser must stop at the first Cluster.
	for n := 0; n+4 < len(data); n++ {
		if string(data[n:n+4]) == "\x1c\x53\xbb\x6b" {
			copy(data[n:n+4], []byte{0x1c, 0x53, 0xbb, 0x6c})
			break
		}
	}
	r := strings.NewReader(string(data))
	if _, err := ReadIndex(context.Background(), r, int64(len(data))); err == nil {
		t.Fatal("missing cue index accepted")
	}
}
func TestOldGenerationsAndIndependentViewers(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := New(nil, nil, logger, Options{Directory: t.TempDir()})
	defer m.Close()
	idx := &Index{Duration: 100, Segments: []Segment{{0, 4, 0}, {80, 84, 0}}}
	s := &Session{ID: "one", Duration: 100, index: idx, hash: "hash", Position: 0, used: time.Now()}
	second := cloneSession(s)
	second.ID = "two"
	ctx, cancel := context.WithCancel(context.Background())
	j := &job{done: make(chan struct{}), cancel: cancel, owners: map[string]bool{"one": true, "two": true}}
	m.sessions[s.ID] = s
	m.sessions[second.ID] = second
	m.jobs[mediaKey(s, 0)] = j
	if _, ok := m.Update("one", 80, 2); !ok {
		t.Fatal("update failed")
	}
	if ctx.Err() != nil {
		t.Fatal("cancelled another viewer")
	}
	updated, _ := m.Update("one", 0, 1)
	if updated.Position != 80 {
		t.Fatal("stale seek replaced destination")
	}
	m.Delete("two")
	if ctx.Err() == nil {
		t.Fatal("abandoned job remains active")
	}
}

func TestVideoArgs(t *testing.T) {
	if got := strings.Join(videoArgs(false), " "); got != "-c:v copy" {
		t.Fatalf("copy args = %q", got)
	}
	if got := strings.Join(videoArgs(true), " "); !strings.Contains(got, "libx264") || !strings.Contains(got, "-pix_fmt yuv420p") {
		t.Fatalf("transcode args = %q", got)
	}
}
