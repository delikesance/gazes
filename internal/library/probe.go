package library

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// MediaInfo is what the library needs to know about a media file.
type MediaInfo struct {
	DurationMS                       int64
	VideoCodec                       string
	AudioCodecs                      []string
	AudioChannels                    []int
	SubtitleTracks, AttachmentTracks int
	VideoStreams                     int     // real video streams, attached pictures excluded
	FrameRate                        float64 // avg_frame_rate of the first real video stream, 0 if unknown
}

// Prober inspects a media file.
type Prober interface {
	Probe(ctx context.Context, path string) (MediaInfo, error)
}

type ffprobe struct{ bin string }

// FFprobe returns a Prober that runs the ffprobe binary at bin.
func FFprobe(bin string) Prober { return ffprobe{bin: bin} }

func (f ffprobe) Probe(ctx context.Context, path string) (MediaInfo, error) {
	cmd := exec.CommandContext(ctx, f.bin, "-v", "error", "-print_format", "json", "-show_format", "-show_streams", path)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return MediaInfo{}, fmt.Errorf("ffprobe: %w: %s", err, errb.String())
	}
	return parseProbe(out.Bytes())
}

func parseProbe(b []byte) (MediaInfo, error) {
	var doc struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
			CodecName string `json:"codec_name"`
			AvgFPS    string `json:"avg_frame_rate"`
			Channels  int    `json:"channels"`
			Dispo     struct {
				AttachedPic int `json:"attached_pic"`
			} `json:"disposition"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return MediaInfo{}, fmt.Errorf("ffprobe: bad json: %w", err)
	}
	var mi MediaInfo
	if d, err := strconv.ParseFloat(doc.Format.Duration, 64); err == nil {
		mi.DurationMS = int64(d*1000 + 0.5)
	}
	for _, s := range doc.Streams {
		switch s.CodecType {
		case "video":
			if s.Dispo.AttachedPic == 1 { // cover art, not a real video stream
				continue
			}
			mi.VideoStreams++
			if mi.VideoCodec == "" {
				mi.VideoCodec = s.CodecName
				mi.FrameRate = parseRate(s.AvgFPS)
			}
		case "audio":
			mi.AudioCodecs = append(mi.AudioCodecs, s.CodecName)
			mi.AudioChannels = append(mi.AudioChannels, s.Channels)
		case "subtitle":
			mi.SubtitleTracks++
		case "attachment":
			mi.AttachmentTracks++
		}
	}
	return mi, nil
}

// parseRate reads ffprobe's "num/den" frame rate; unknown or malformed values give 0.
func parseRate(r string) float64 {
	num, den, ok := strings.Cut(r, "/")
	n, err := strconv.ParseFloat(num, 64)
	if err != nil || n <= 0 {
		return 0
	}
	if !ok {
		return n
	}
	d, err := strconv.ParseFloat(den, 64)
	if err != nil || d <= 0 {
		return 0
	}
	return n / d
}
