package library

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Defaults applied by BuildArgs when the matching EncodeSettings field is zero.
const (
	defaultPreset  = 8
	defaultCRF     = 30
	defaultThreads = 8
)

// EncodeSettings configures the AV1 encoder worker.
type EncodeSettings struct {
	FFmpeg               string // ffmpeg binary, "ffmpeg" when empty
	Preset, CRF, Threads int    // SVT-AV1 preset, CRF and logical processors (lp)
	PauseStreams         int    // pause while at least this many streams are active (0 = never)
	Window               string // "HH:MM-HH:MM" local-time window, "" = always
	PeakWindow           string // "HH:MM-HH:MM" local-time peak hours during which encoding is paused, "" = none
}

func (s EncodeSettings) ffmpeg() string {
	if s.FFmpeg == "" {
		return "ffmpeg"
	}
	return s.FFmpeg
}

// BuildArgs returns the ffmpeg arguments (without the binary and without `nice`) that re-encode
// in to out as AV1. AAC and Opus audio is copied, everything else becomes Opus.
func BuildArgs(in, out string, info MediaInfo, s EncodeSettings) []string {
	preset, crf, lp := s.Preset, s.CRF, s.Threads
	if preset == 0 {
		preset = defaultPreset
	}
	if crf == 0 {
		crf = defaultCRF
	}
	if lp <= 0 {
		lp = defaultThreads
	}
	args := []string{
		"-y", "-nostdin", "-hide_banner", "-nostats", "-progress", "pipe:1",
		"-i", in,
		// Real video only (no attached pictures), then audio, subtitles and attachments; never data streams.
		"-map", "0:V", "-map", "0:a?", "-map", "0:s?", "-map", "0:t?", "-map", "-0:d?",
		"-c:v", "libsvtav1", "-preset", strconv.Itoa(preset), "-crf", strconv.Itoa(crf),
		"-pix_fmt", "yuv420p10le", "-svtav1-params", "lp=" + strconv.Itoa(lp),
	}
	for i, codec := range info.AudioCodecs {
		idx := strconv.Itoa(i)
		if codec == "aac" || codec == "opus" {
			args = append(args, "-c:a:"+idx, "copy")
			continue
		}
		bitrate := "128k"
		if i < len(info.AudioChannels) && info.AudioChannels[i] > 2 {
			bitrate = "256k"
			// libopus rejects layouts such as 5.1(side) (what AC3/DTS decode to); aformat remaps
			// them to the nearest layout it accepts.
			args = append(args, "-filter:a:"+idx, "aformat=channel_layouts=mono|stereo|quad|5.1|7.1")
		}
		args = append(args, "-c:a:"+idx, "libopus", "-b:a:"+idx, bitrate)
	}
	args = append(args,
		"-c:s", "copy", "-c:t", "copy",
		"-map_chapters", "0", "-map_metadata", "0",
		out)
	return args
}

func parseClock(s string) (int, error) {
	h, m, ok := strings.Cut(strings.TrimSpace(s), ":")
	if !ok {
		return 0, fmt.Errorf("library: bad time %q", s)
	}
	hh, err1 := strconv.Atoi(h)
	mm, err2 := strconv.Atoi(m)
	if err1 != nil || err2 != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return 0, fmt.Errorf("library: bad time %q", s)
	}
	return hh*60 + mm, nil
}

// InWindow reports whether now falls in the "HH:MM-HH:MM" window (start inclusive, end exclusive).
// An empty window is always open; a window whose start is after its end wraps past midnight.
func InWindow(window string, now time.Time) (bool, error) {
	if strings.TrimSpace(window) == "" {
		return true, nil
	}
	a, b, ok := strings.Cut(window, "-")
	if !ok {
		return false, fmt.Errorf("library: bad window %q", window)
	}
	start, err := parseClock(a)
	if err != nil {
		return false, err
	}
	end, err := parseClock(b)
	if err != nil {
		return false, err
	}
	cur := now.Hour()*60 + now.Minute()
	switch {
	case start == end:
		return true, nil
	case start < end:
		return cur >= start && cur < end, nil
	default:
		return cur >= start || cur < end, nil
	}
}
