package stream

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRemuxVideoCodecs(t *testing.T) {
	ffmpeg := FFmpegPath()
	if _, err := exec.LookPath(ffmpeg); err != nil {
		t.Skip("ffmpeg unavailable")
	}
	ffprobe := filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
	if _, err := exec.LookPath(ffprobe); err != nil {
		t.Skip("ffprobe unavailable")
	}
	t.Setenv("FFPROBE_PATH", ffprobe)
	for _, tc := range []struct{ codec, encoder, tag, pixelFormat string }{
		{"hevc", "libx265", "hvc1", "yuv420p"},
		{"hevc", "libx265", "hvc1", "yuv420p10le"},
		{"h264", "libx264", "avc1", "yuv420p"},
	} {
		t.Run(tc.codec+"/"+tc.pixelFormat, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			input := filepath.Join(t.TempDir(), "input.mkv")
			args := []string{"-v", "error", "-f", "lavfi", "-i", "testsrc2=size=64x64:rate=10", "-t", "3", "-c:v", tc.encoder, "-pix_fmt", tc.pixelFormat, "-threads", "1", "-g", "5"}
			if tc.codec == "hevc" {
				args = append(args, "-x265-params", "pools=none:frame-threads=1")
			}
			args = append(args, input)
			if out, err := exec.CommandContext(ctx, ffmpeg, args...).CombinedOutput(); err != nil {
				t.Fatalf("fixture: %v: %s", err, out)
			}
			src, err := os.Open(input)
			if err != nil {
				t.Fatal(err)
			}
			stat, err := src.Stat()
			if err != nil {
				t.Fatal(err)
			}
			rec := httptest.NewRecorder()
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			req := httptest.NewRequest("GET", "/stream", nil).WithContext(ctx)
			if err := NewPipelineManager(logger).ServeHTTP(rec, req, src, input, stat.Size(), PipelineOptions{}); err != nil {
				t.Fatal(err)
			}
			output := filepath.Join(t.TempDir(), "output.mp4")
			if err := os.WriteFile(output, rec.Body.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			probe, err := exec.CommandContext(ctx, ffprobe, "-v", "error", "-select_streams", "v:0", "-count_packets", "-show_entries", "stream=codec_name,codec_tag_string,nb_read_packets", "-of", "json", output).Output()
			if err != nil {
				t.Fatal(err)
			}
			var result struct {
				Streams []struct {
					Codec   string `json:"codec_name"`
					Tag     string `json:"codec_tag_string"`
					Packets string `json:"nb_read_packets"`
				} `json:"streams"`
			}
			if err := json.Unmarshal(probe, &result); err != nil {
				t.Fatal(err)
			}
			if len(result.Streams) != 1 {
				t.Fatalf("missing video: %s", probe)
			}
			video := result.Streams[0]
			if video.Codec != tc.codec || video.Tag != tc.tag || video.Packets == "0" || video.Packets == "" {
				t.Fatalf("invalid remux: %s", probe)
			}
			if out, err := exec.CommandContext(ctx, ffmpeg, "-v", "error", "-i", output, "-f", "null", "-").CombinedOutput(); err != nil || len(out) > 0 {
				t.Fatalf("decode: %v: %s", err, out)
			}
		})
	}
}

// Two distinguishable tones ensure that a successful HTTP response also contains
// the requested, decodable audio after a non-keyframe HEVC seek.
func TestHEVCAudioSwitch(t *testing.T) {
	ffmpeg := FFmpegPath()
	if _, err := exec.LookPath(ffmpeg); err != nil {
		t.Skip("ffmpeg unavailable")
	}
	ffprobe := filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
	for _, tc := range []struct{ pixelFormat, audioCodec string }{
		{"yuv420p", "aac"}, {"yuv420p10le", "aac"},
		{"yuv420p", "ac3"}, {"yuv420p10le", "ac3"},
	} {
		t.Run(tc.pixelFormat+"/"+tc.audioCodec, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			input := filepath.Join(t.TempDir(), "dual.mkv")
			args := []string{"-v", "error", "-f", "lavfi", "-i", "testsrc2=size=64x64:rate=10", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-f", "lavfi", "-i", "sine=frequency=880:sample_rate=48000", "-map", "0:v", "-map", "1:a", "-map", "2:a", "-t", "6", "-c:v", "libx265", "-pix_fmt", tc.pixelFormat, "-x265-params", "pools=none:frame-threads=1", "-g", "20", "-c:a:0", tc.audioCodec, "-c:a:1", "aac", input}
			if out, err := exec.CommandContext(ctx, ffmpeg, args...).CombinedOutput(); err != nil {
				t.Fatalf("fixture: %v: %s", err, out)
			}
			for _, track := range []int{0, 1, 0} {
				output := filepath.Join(t.TempDir(), "output.mp4")
				rec := httptest.NewRecorder()
				audioCodec := "aac"
				if track == 0 {
					audioCodec = tc.audioCodec
				}
				err := RemuxStream(ctx, rec, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), PipelineOptions{InputURL: input, VideoCodec: "hevc", AudioCodec: audioCodec, AudioTrackIndex: track, TimeOffset: 2.35})
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(output, rec.Body.Bytes(), 0600); err != nil {
					t.Fatal(err)
				}
				probe, err := exec.CommandContext(ctx, ffprobe, "-v", "error", "-count_packets", "-show_entries", "stream=codec_name,codec_tag_string,start_time,nb_read_packets", "-of", "json", output).Output()
				if err != nil {
					t.Fatal(err)
				}
				var result struct {
					Streams []struct {
						Codec   string `json:"codec_name"`
						Tag     string `json:"codec_tag_string"`
						Start   string `json:"start_time"`
						Packets string `json:"nb_read_packets"`
					}
				}
				if err := json.Unmarshal(probe, &result); err != nil {
					t.Fatal(err)
				}
				if len(result.Streams) != 2 || result.Streams[0].Codec != "hevc" || result.Streams[0].Tag != "hvc1" || result.Streams[1].Codec != "aac" || result.Streams[1].Packets == "0" {
					t.Fatalf("bad streams: %s", probe)
				}
				// Match copied packets to the source instead of comparing stream
				// start times, which legitimately differ due to keyframe preroll.
				packetProbe := func(path string) map[string]float64 {
					raw, err := exec.CommandContext(ctx, ffprobe, "-v", "error", "-show_packets", "-show_data_hash", "sha256", "-show_entries", "packet=pts_time,data_hash", "-of", "json", path).Output()
					if err != nil {
						t.Fatal(err)
					}
					var packets struct {
						Packets []struct {
							PTS  string `json:"pts_time"`
							Hash string `json:"data_hash"`
						}
					}
					if err := json.Unmarshal(raw, &packets); err != nil {
						t.Fatal(err)
					}
					times := map[string]float64{}
					for _, packet := range packets.Packets {
						times[packet.Hash], _ = strconv.ParseFloat(packet.PTS, 64)
					}
					return times
				}
				sourceTimes := packetProbe(input)
				matched := 0
				for hash, timestamp := range packetProbe(output) {
					if _, ok := sourceTimes[hash]; ok {
						matched++
					}
					// FFmpeg 6.x shifts the remuxed timeline by up to one AAC frame
					// (1024 samples at 48 kHz) of priming; newer releases do not.
					if original, ok := sourceTimes[hash]; ok && math.Abs(timestamp-original+2.35) > 1024.0/48000+.002 {
						t.Fatalf("packet timeline shifted: output %.6f, source %.6f", timestamp, original)
					}
				}
				if audioCodec != "aac" {
					raw, err := exec.CommandContext(ctx, ffprobe, "-v", "error", "-select_streams", "a:0", "-read_intervals", "%+#1", "-show_packets", "-show_entries", "packet=pts_time", "-of", "csv=p=0", output).Output()
					if err != nil {
						t.Fatal(err)
					}
					firstPTS, err := strconv.ParseFloat(strings.Split(strings.TrimSpace(string(raw)), ",")[0], 64)
					if err != nil || firstPTS < -.025 {
						t.Fatalf("encoded audio starts with silent preroll: %s", raw)
					}
				}
				if matched == 0 {
					t.Fatal("no copied packets matched the original video")
				}
				pcm, err := exec.CommandContext(ctx, ffmpeg, "-v", "error", "-i", output, "-map", "0:a:0", "-t", "1", "-ac", "1", "-ar", "48000", "-f", "s16le", "pipe:1").Output()
				if err != nil || len(pcm) < 48000 {
					t.Fatalf("missing decoded audio: %v, %d bytes", err, len(pcm))
				}
				crossings := 0
				previous := int16(0)
				for i := 0; i+1 < len(pcm); i += 2 {
					sample := int16(binary.LittleEndian.Uint16(pcm[i : i+2]))
					if previous < 0 && sample >= 0 {
						crossings++
					}
					previous = sample
				}
				frequency := float64(crossings) / (float64(len(pcm)/2) / 48000)
				expected := 440.0 * float64(track+1)
				if math.Abs(frequency-expected) > 15 {
					t.Fatalf("wrong or silent audio: %.1f Hz, want %.1f", frequency, expected)
				}
			}
			rec := httptest.NewRecorder()
			if err := RemuxStream(ctx, rec, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), PipelineOptions{InputURL: input, VideoCodec: "hevc", AudioCodec: "aac", AudioTrackIndex: 99}); err == nil {
				t.Fatal("missing audio track silently accepted")
			}
		})
	}
}
