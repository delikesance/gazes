package torrent_test

import (
	"testing"

	"github.com/gazes/gazes/internal/torrent"
)

func TestIsVideoFile(t *testing.T) {
	tests := []struct {
		filename string
		expected bool
	}{
		{"anime_ep01.mkv", true},
		{"movie.mp4", true},
		{"stream.webm", true},
		{"subtitles.ass", false},
		{"cover.jpg", false},
		{"README.txt", false},
	}

	for _, tc := range tests {
		got := torrent.IsVideoFile(tc.filename)
		if got != tc.expected {
			t.Errorf("IsVideoFile(%q) = %v, expected %v", tc.filename, got, tc.expected)
		}
	}
}

func TestDetectMimeType(t *testing.T) {
	tests := []struct {
		filename string
		expected string
	}{
		{"video.mkv", "video/x-matroska"},
		{"video.mp4", "video/mp4"},
		{"video.webm", "video/webm"},
		{"unknown.xyz", "application/octet-stream"},
	}

	for _, tc := range tests {
		got := torrent.DetectMimeType(tc.filename)
		if got != tc.expected {
			t.Errorf("DetectMimeType(%q) = %s, expected %s", tc.filename, got, tc.expected)
		}
	}
}

func TestFindMainVideoFile(t *testing.T) {
	files := []torrent.FileInfo{
		{Index: 0, Path: "sample.mkv", Length: 10 * 1024 * 1024, IsVideo: true},
		{Index: 1, Path: "subs/eng.ass", Length: 100 * 1024, IsVideo: false},
		{Index: 2, Path: "Full_Episode_1080p.mkv", Length: 1400 * 1024 * 1024, IsVideo: true},
		{Index: 3, Path: "cover.png", Length: 500 * 1024, IsVideo: false},
	}

	mainVideo := torrent.FindMainVideoFile(files)
	if mainVideo == nil {
		t.Fatalf("expected non-nil main video")
	}

	if mainVideo.Index != 2 {
		t.Errorf("expected index 2, got %d", mainVideo.Index)
	}
	if mainVideo.Path != "Full_Episode_1080p.mkv" {
		t.Errorf("unexpected path: %s", mainVideo.Path)
	}
}
