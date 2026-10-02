package api

import (
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
			server.HandleSubtitles(recorder, httptest.NewRequest("GET", "/subtitles?ih=test&format="+tc.format, nil))
			if recorder.Code != tc.status || !strings.HasPrefix(recorder.Header().Get("Content-Type"), tc.contentType) || !strings.Contains(recorder.Body.String(), tc.body) {
				t.Fatalf("unexpected subtitle response: %d %v %s", recorder.Code, recorder.Header(), recorder.Body.String())
			}
			if tc.status != 200 && strings.Contains(recorder.Body.String(), "partial output") {
				t.Fatal("partial extraction leaked")
			}
		})
	}
}
