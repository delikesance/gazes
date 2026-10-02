package api

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gazes/gazes/internal/config"
)

func TestSubtitleWindowRejectsInvalidBounds(t *testing.T) {
	s := &Server{}
	for _, query := range []string{"start=-1", "start=NaN", "start=Inf", "start=604801", "duration=0", "duration=121", "duration=NaN", "duration=invalid"} {
		w := httptest.NewRecorder()
		s.HandleSubtitles(w, httptest.NewRequest("GET", "/subtitles?ih=test&"+query, nil))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s: status %d", query, w.Code)
		}
	}
}

func TestSubtitleExtractionStopsOnDisconnect(t *testing.T) {
	temporary := t.TempDir()
	executable := filepath.Join(temporary, "ffmpeg")
	started := filepath.Join(temporary, "started")
	t.Setenv("GAZES_SUBTITLE_TEST_STARTED", started)
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf started > \"$GAZES_SUBTITLE_TEST_STARTED\"\nexec sleep 30\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FFMPEG_PATH", executable)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &Server{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		s.HandleSubtitles(w, httptest.NewRequest("GET", "/subtitles?ih=test&start=0&duration=120", nil).WithContext(ctx))
		close(done)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(started); err == nil {
			break
		}
		select {
		case <-done:
			t.Fatal("subtitle process exited before cancellation")
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("subtitle process did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
		if w.Body.Len() != 0 {
			t.Fatal("canceled extraction wrote a response")
		}
	case <-time.After(time.Second):
		t.Fatal("subtitle process did not stop on disconnect")
	}
}

func TestSubtitleWindowFFmpegPreservesTimestamps(t *testing.T) {
	if _, err := exec.LookPath(findFFmpegBin()); err != nil {
		t.Skip("FFmpeg required for media integration")
	}
	// Synthetic three-minute video with PGS and text cues at 70s and 150s.
	data, err := os.ReadFile("testdata/subtitle-window.mkv")
	if err != nil {
		t.Fatal(err)
	}
	raw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "fixture.mkv", time.Time{}, bytes.NewReader(data))
	}))
	defer raw.Close()
	u, _ := url.Parse(raw.URL)
	port, _ := strconv.Atoi(u.Port())
	s := &Server{cfg: &config.Config{Port: port}, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	for _, format := range []string{"sup", "ass"} {
		t.Run(format, func(t *testing.T) {
			track := "0"
			if format == "ass" {
				track = "1"
			}
			w := httptest.NewRecorder()
			s.HandleSubtitles(w, httptest.NewRequest("GET", "/subtitles?ih=test&start=60&duration=40&track_idx="+track+"&format="+format, nil))
			if w.Code != 200 {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			if format == "ass" {
				if !strings.Contains(w.Body.String(), "0:01:10.00") || !strings.Contains(w.Body.String(), "Inside window") || strings.Contains(w.Body.String(), "Outside window") {
					t.Fatalf("subtitle timings/window changed: %s", w.Body.String())
				}
				return
			}
			data := w.Body.Bytes()
			if len(data) == 0 {
				t.Fatal("missing PGS subtitles")
			}
			for len(data) > 0 {
				if len(data) < 13 || string(data[:2]) != "PG" {
					t.Fatal("invalid PGS packet")
				}
				pts := float64(binary.BigEndian.Uint32(data[2:6])) / 90000
				if pts < 60 || pts >= 100 {
					t.Fatalf("PGS cue outside requested window: %f", pts)
				}
				length := 13 + int(binary.BigEndian.Uint16(data[11:13]))
				if length > len(data) {
					t.Fatal("truncated PGS packet")
				}
				data = data[length:]
			}
		})
	}
}
