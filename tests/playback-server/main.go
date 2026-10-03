// A local, synthetic-media server for browser decoding tests. Never uses public swarms.
package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gazes/gazes/internal/api"
	"github.com/gazes/gazes/internal/config"
	"github.com/gazes/gazes/internal/stream"
	"github.com/gazes/gazes/internal/torrent"
)

type engine struct{ files map[string]string }

func (e *engine) AddTorrent(ctx context.Context, magnet string) (string, []torrent.FileInfo, error) {
	hash := strings.TrimPrefix(magnet, "fixture:")
	path, ok := e.files[hash]
	if !ok {
		return "", nil, fmt.Errorf("fixture missing")
	}
	stat, err := os.Stat(path)
	if err != nil {
		return "", nil, err
	}
	return hash, []torrent.FileInfo{{Index: 0, Path: path, Length: stat.Size(), IsVideo: true}}, nil
}
func (e *engine) GetFileStream(ctx context.Context, hash string, index int) (io.ReadSeekCloser, *torrent.FileInfo, error) {
	path := e.files[hash]
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	stat, _ := f.Stat()
	return f, &torrent.FileInfo{Index: 0, Path: path, Length: stat.Size(), IsVideo: true}, nil
}
func (e *engine) GetStats(string) (*torrent.SwarmStats, error) {
	return &torrent.SwarmStats{ActiveSeeders: 1}, nil
}
func (e *engine) Close() error { return nil }
func main() {
	dir, err := os.MkdirTemp("", "gazes-playback-browser-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	ass := `[Script Info]
ScriptType: v4.00+
PlayResX: 160
PlayResY: 90
[V4+ Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding
Style: Default,Liberation Sans,12,&H00FFFFFF,&H00FFFFFF,&H00000000,&H00000000,0,0,0,0,100,100,0,0,1,1,0,2,2,2,2,1
[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
Dialogue: 0,0:00:00.00,0:01:15.00,Default,,0,0,0,,Long overlapping cue
`
	for n := 0; n < 80; n++ {
		ass += fmt.Sprintf("Dialogue: 0,0:%02d:%02d.00,0:%02d:%02d.80,Default,,0,0,0,,SECOND %d\n", n/60, n%60, n/60, n%60, n)
	}
	sub := filepath.Join(dir, "sub.ass")
	os.WriteFile(sub, []byte(ass), 0600)
	filter := "color=black:size=160x90:rate=25"
	for bit := 0; bit < 7; bit++ {
		filter += fmt.Sprintf(",drawbox=x=%d:y=0:w=20:h=20:color=white:t=fill:enable='gte(mod(floor(t*25)/%d,2),1)'", bit*20, 1<<bit)
	}
	files := map[string]string{}
	items := []map[string]string{}
	for _, tc := range []struct {
		name, ext string
		origin    int
	}{{"mkv", "mkv", 0}, {"shifted", "mkv", 5}, {"mp4", "mp4", 0}} {
		path := filepath.Join(dir, tc.name+"."+tc.ext)
		args := []string{"-v", "error", "-f", "lavfi", "-i", filter, "-f", "lavfi", "-i", "aevalsrc='sin(2*PI*880*t)*lt(mod(t,1),0.1)':s=48000", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-i", sub, "-map", "0:v", "-map", "1:a", "-map", "2:a", "-map", "3:s", "-t", "80", "-c:v", "libx264", "-g", "50", "-bf", "2", "-threads", "1", "-c:a:0", "aac", "-c:a:1", "ac3", "-c:s", "ass", "-metadata:s:s:0", "language=fre", "-output_ts_offset", fmt.Sprint(tc.origin), "-y", path}
		if tc.ext == "mp4" {
			for n := range args {
				if args[n] == "ass" {
					args[n] = "mov_text"
				}
			}
		}
		if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
			panic(fmt.Sprintf("fixture %v: %s", err, out))
		}
		hash := sha1.Sum([]byte(tc.name))
		id := hex.EncodeToString(hash[:])
		files[id] = path
		items = append(items, map[string]string{"name": tc.name, "info_hash": id, "magnet_uri": "fixture:" + id})
	}
	pgs := "/src/internal/api/testdata/subtitle-window.mkv"
	if _, err := os.Stat(pgs); err == nil {
		id := strings.Repeat("b", 40)
		files[id] = pgs
		items = append(items, map[string]string{"name": "pgs", "info_hash": id, "magnet_uri": "fixture:" + id})
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	server := api.NewServer(&config.Config{Port: 8099, CacheDir: dir, PlaybackEngine: "hls"}, logger, nil, &engine{files}, stream.NewPipelineManager(logger))
	defer server.ClosePlayback()
	mux := http.NewServeMux()
	mux.HandleFunc("/fixtures", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(items)
	})
	mux.Handle("/", server.Router())
	fmt.Println("READY :8099")
	if err := http.ListenAndServe(":8099", mux); err != nil {
		panic(err)
	}
}
