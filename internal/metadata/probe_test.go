package metadata_test

import (
	"testing"

	"github.com/gazes/gazes/internal/metadata"
)

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		seconds  float64
		expected string
	}{
		{0, "00:00"},
		{-10, "00:00"},
		{45.2, "00:45"},
		{1442.8, "24:02"},
		{3600, "01:00:00"},
		{3665.4, "01:01:05"},
		{7322.0, "02:02:02"},
	}

	for _, tc := range tests {
		got := metadata.FormatDuration(tc.seconds)
		if got != tc.expected {
			t.Errorf("FormatDuration(%f) = %s, expected %s", tc.seconds, got, tc.expected)
		}
	}
}

func TestGetResolutionLabel(t *testing.T) {
	tests := []struct {
		width    int
		height   int
		expected string
	}{
		{3840, 2160, "4K"},
		{2560, 1440, "1440p"},
		{1920, 1080, "1080p"},
		{1280, 720, "720p"},
		{854, 480, "480p"},
		{640, 360, "360p"},
		{0, 0, ""},
	}

	for _, tc := range tests {
		got := metadata.GetResolutionLabel(tc.width, tc.height)
		if got != tc.expected {
			t.Errorf("GetResolutionLabel(%d, %d) = %s, expected %s", tc.width, tc.height, got, tc.expected)
		}
	}
}
