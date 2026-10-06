package api

import (
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSubtitleFormatsAndFailure(t *testing.T) {
	for _, tc := range []struct {
		name, format, script, contentType, body string
		status                                  int
	}{
		{"vtt default", "", "printf 'WEBVTT\\n\\n'", "text/vtt", "WEBVTT", 200},
		{"ass", "ass", "printf '[Script Info]\\n'", "text/x-ssa", "[Script Info]", 200},
		{"invalid format", "png", "exit 0", "text/plain", "unsupported", 400},
		{"extraction failure", "ass", "printf 'partial output'; exit 1", "text/plain", "Impossible", 502},
	} {
		t.Run(tc.name, func(t *testing.T) {
			executable := filepath.Join(t.TempDir(), "ffmpeg")
			if err := os.WriteFile(executable, []byte("#!/bin/sh\n"+tc.script+"\n"), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("FFMPEG_PATH", executable)
			server := &Server{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
			recorder := httptest.NewRecorder()
			server.HandleSubtitles(recorder, httptest.NewRequest("GET", "/subtitles?ih=0123456789abcdef0123456789abcdef01234567&format="+tc.format, nil))
			if recorder.Code != tc.status || !strings.HasPrefix(recorder.Header().Get("Content-Type"), tc.contentType) || !strings.Contains(recorder.Body.String(), tc.body) {
				t.Fatalf("unexpected subtitle response: %d %v %s", recorder.Code, recorder.Header(), recorder.Body.String())
			}
			if tc.status != 200 && strings.Contains(recorder.Body.String(), "partial output") {
				t.Fatal("partial extraction leaked")
			}
		})
	}
}

func TestSubtitlesRejectInvalidTarget(t *testing.T) {
	const ih = "0123456789abcdef0123456789abcdef01234567"
	for _, query := range []string{
		"ih=test",
		"ih=" + ih + "%26file_idx%3D9",
		"ih=" + ih + "&file_idx=-1",
		"ih=" + ih + "&file_idx=abc",
		"ih=" + ih + "&file_idx=100001",
		"ih=" + ih + "&track_idx=-1",
		"ih=" + ih + "&track_idx=1001",
	} {
		executable := filepath.Join(t.TempDir(), "ffmpeg")
		if err := os.WriteFile(executable, []byte("#!/bin/sh\necho ran >&2; exit 1\n"), 0700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("FFMPEG_PATH", executable)
		server := &Server{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
		recorder := httptest.NewRecorder()
		server.HandleSubtitles(recorder, httptest.NewRequest("GET", "/subtitles?"+query, nil))
		if recorder.Code != 400 {
			t.Fatalf("%q: status %d, want 400", query, recorder.Code)
		}
	}
}

func TestSubtitleJobsCapConcurrency(t *testing.T) {
	var jobs subtitleJobs
	script := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexec sleep 30\n"), 0700); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var started []*subtitleJob
	for i := 0; i < subtitleMaxConcurrent; i++ {
		job := jobs.join(fmt.Sprintf("key-%d", i), logger, nil, script)
		if job == nil {
			t.Fatalf("job %d refused below the cap", i)
		}
		started = append(started, job)
	}
	if jobs.join("one-too-many", logger, nil, script) != nil {
		t.Fatal("job beyond the cap was started")
	}
	if again := jobs.join("key-0", logger, nil, script); again != started[0] {
		t.Fatal("identical request must join the running job even at the cap")
	}
	for _, job := range started {
		job.cancel()
		<-job.done
	}
	if jobs.join("after-release", logger, nil, script) == nil {
		t.Fatal("slot was not released")
	}
}
