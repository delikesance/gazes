package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gazes/gazes/internal/diagnostics"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// AudioTrack represents a probed audio stream.
type AudioTrack struct {
	Index       int    `json:"index"`
	StreamIndex int    `json:"stream_index"`
	Language    string `json:"language"`
	Title       string `json:"title"`
	Codec       string `json:"codec"`
	Channels    int    `json:"channels"`
	IsDefault   bool   `json:"is_default"`
}

// SubtitleTrack represents a probed subtitle stream.
type SubtitleTrack struct {
	Index       int    `json:"index"`
	StreamIndex int    `json:"stream_index"`
	Language    string `json:"language"`
	Title       string `json:"title"`
	Codec       string `json:"codec"`
	IsDefault   bool   `json:"is_default"`
	IsForced    bool   `json:"is_forced"`
}

// VideoMetadata encapsulates parsed stream details, dimensions, codecs, durations, and multi-tracks.
type VideoMetadata struct {
	ProbeStatus       string          `json:"probe_status,omitempty"`
	ProbeErrorCode    string          `json:"probe_error_code,omitempty"`
	DurationSec       float64         `json:"duration_sec"`
	FormattedDuration string          `json:"formatted_duration"`
	Width             int             `json:"width"`
	Height            int             `json:"height"`
	Resolution        string          `json:"resolution"`
	VideoCodec        string          `json:"video_codec"`
	AudioCodec        string          `json:"audio_codec"`
	Bitrate           int64           `json:"bitrate_bps"`
	TotalBytes        int64           `json:"total_bytes"`
	AudioTracks       []AudioTrack    `json:"audio_tracks"`
	SubtitleTracks    []SubtitleTrack `json:"subtitle_tracks"`
	Chapters          []Chapter       `json:"chapters,omitempty"`
	SkipSegments      []SkipSegment   `json:"skip_segments,omitempty"`
}

type ffprobeOutput struct {
	Streams []struct {
		Index       int    `json:"index"`
		CodecName   string `json:"codec_name"`
		CodecType   string `json:"codec_type"`
		Width       int    `json:"width,omitempty"`
		Height      int    `json:"height,omitempty"`
		Channels    int    `json:"channels,omitempty"`
		Disposition struct {
			Default int `json:"default"`
			Forced  int `json:"forced"`
		} `json:"disposition"`
		Tags struct {
			Language string `json:"language"`
			Title    string `json:"title"`
		} `json:"tags"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
		Size     string `json:"size"`
		BitRate  string `json:"bit_rate"`
	} `json:"format"`
}

// Analyzer provides video metadata extraction capabilities.
type Analyzer interface {
	ProbeReader(ctx context.Context, r io.Reader, totalBytes int64) (*VideoMetadata, error)
}

// FFprobeAnalyzer implements Analyzer using the ffprobe utility.
type FFprobeAnalyzer struct {
	logger *slog.Logger
}

// NewFFprobeAnalyzer creates a new FFprobeAnalyzer instance.
func NewFFprobeAnalyzer(logger *slog.Logger) *FFprobeAnalyzer {
	return &FFprobeAnalyzer{
		logger: logger,
	}
}

// ProbeReader inspects an on-the-fly stream using ffprobe to extract video, audio, and subtitle properties.
func (a *FFprobeAnalyzer) ProbeReader(ctx context.Context, r io.Reader, totalBytes int64) (*VideoMetadata, error) {
	// Header sizes grow in steps: most files describe their tracks in the first 64 KB, while MKVs
	// with many attached fonts (fansub batches) only list them after several MB. Each step is
	// bounded and rewinds the reader; a slow swarm never triggers a whole-file scan.
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var meta *VideoMetadata
	var err error
	for i, limit := range probeLimits {
		if i > 0 {
			seekable, ok := r.(io.Seeker)
			if !ok || ctx.Err() != nil || totalBytes <= probeLimits[i-1] {
				break
			}
			if _, seekErr := seekable.Seek(0, io.SeekStart); seekErr != nil {
				break
			}
		}
		meta, err = a.probeReader(ctx, r, totalBytes, limit)
		if err == nil && (meta.DurationSec > 0 || len(meta.AudioTracks) > 0) {
			attachChapters(ctx, diagnostics.Logger(ctx, a.logger), r, meta)
			return meta, nil
		}
	}
	return meta, err
}

// chapterReadTimeout bounds the chapter read so a swarm stall never delays playback.
var chapterReadTimeout = 5 * time.Second

// attachChapters reads Matroska chapters from the start of r and derives skip segments.
// Failures are logged and ignored: detection is best-effort and must not block playback.
func attachChapters(ctx context.Context, logger *slog.Logger, r io.Reader, meta *VideoMetadata) {
	seeker, ok := r.(io.ReadSeeker)
	if !ok {
		return
	}
	if _, err := seeker.Seek(0, io.SeekStart); err != nil {
		return
	}
	defer func() { _, _ = seeker.Seek(0, io.SeekStart) }()
	ctx, cancel := context.WithTimeout(ctx, chapterReadTimeout)
	defer cancel()
	chapters, err := readMatroskaChapters(ctx, seeker, meta.TotalBytes)
	if err != nil {
		logger.Debug("media.chapters_failed", "err", err)
		return
	}
	meta.Chapters = chapters
	meta.SkipSegments = DetectSkipSegments(chapters, meta.DurationSec)
}

// probeLimits are the bytes read from the start of the file at each probe attempt.
var probeLimits = []int64{64 * 1024, 1_000_000, 10_000_000}

func (a *FFprobeAnalyzer) probeReader(ctx context.Context, r io.Reader, totalBytes, maxBytes int64) (*VideoMetadata, error) {
	diagnostics.Logger(ctx, a.logger).Debug("media.probe_started", "bytes", totalBytes)
	ffprobeBin := findFFprobePath()
	r = skipMatroskaAttachments(ctx, r, totalBytes)

	args := []string{
		"-v", "error",
		"-probesize", strconv.FormatInt(maxBytes, 10),
		"-analyzeduration", "1000000",
		"-show_entries", "format=duration,size,bit_rate:stream=index,codec_name,codec_type,width,height,channels,disposition:stream_tags=language,title",
		"-of", "json",
		"pipe:0",
	}

	probeCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()

	cmd := exec.CommandContext(probeCtx, ffprobeBin, args...)
	var stdoutBuf bytes.Buffer
	var stderrBuf diagnostics.LimitedBuffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	var runErr, contextErr error
	if contextual, ok := r.(interface {
		ReadContext(context.Context, []byte) (int, error)
	}); ok {
		// Own the stdin pump: exec.Cmd otherwise waits for a copier that can be
		// stuck in a torrent read even after FFprobe has produced its result.
		input, err := cmd.StdinPipe()
		if err != nil {
			return nil, err
		}
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer input.Close()
			_, _ = io.Copy(input, io.LimitReader(&probeContextReader{ctx: probeCtx, reader: contextual}, maxBytes))
		}()
		runErr = cmd.Run()
		contextErr = probeCtx.Err()
		cancel()
		_ = input.Close()
		<-done // Finish the reader before callers restore its seek position.
	} else {
		cmd.Stdin = io.LimitReader(r, maxBytes)
		runErr = cmd.Run()
		contextErr = probeCtx.Err()
	}
	if runErr != nil {
		if contextErr != nil {
			return nil, fmt.Errorf("ffprobe timeout or cancelled: %w", contextErr)
		}
		diagnostics.Logger(ctx, a.logger).Debug("ffprobe execution note", "err", runErr, "stderr", stderrBuf.String())
		return nil, fmt.Errorf("ffprobe failed: %w", runErr)
	}

	var parsed ffprobeOutput
	if err := json.Unmarshal(stdoutBuf.Bytes(), &parsed); err != nil {
		return nil, fmt.Errorf("invalid ffprobe output: %w", err)
	}

	meta := &VideoMetadata{
		ProbeStatus:    "complete",
		TotalBytes:     totalBytes,
		AudioTracks:    make([]AudioTrack, 0),
		SubtitleTracks: make([]SubtitleTrack, 0),
	}

	// Parse duration
	if parsed.Format.Duration != "" {
		if dur, err := strconv.ParseFloat(parsed.Format.Duration, 64); err == nil && dur > 0 {
			meta.DurationSec = dur
			meta.FormattedDuration = FormatDuration(dur)
		}
	}

	// Parse bitrate
	if parsed.Format.BitRate != "" {
		if br, err := strconv.ParseInt(parsed.Format.BitRate, 10, 64); err == nil {
			meta.Bitrate = br
		}
	}

	var audioIdx, subIdx int

	// Parse stream codecs, dimensions, and multi-tracks
	for _, stream := range parsed.Streams {
		switch stream.CodecType {
		case "video":
			if meta.VideoCodec == "" {
				meta.VideoCodec = stream.CodecName
				meta.Width = stream.Width
				meta.Height = stream.Height
				meta.Resolution = GetResolutionLabel(stream.Width, stream.Height)
			}
		case "audio":
			if meta.AudioCodec == "" {
				meta.AudioCodec = stream.CodecName
			}
			lang := stream.Tags.Language
			if lang == "" {
				lang = "und"
			}
			title := stream.Tags.Title
			if title == "" {
				title = formatAudioTitle(lang, stream.Channels, stream.CodecName, audioIdx)
			}
			meta.AudioTracks = append(meta.AudioTracks, AudioTrack{
				Index:       audioIdx,
				StreamIndex: stream.Index,
				Language:    lang,
				Title:       title,
				Codec:       stream.CodecName,
				Channels:    stream.Channels,
				IsDefault:   stream.Disposition.Default == 1,
			})
			audioIdx++
		case "subtitle":
			lang := stream.Tags.Language
			if lang == "" {
				lang = "und"
			}
			title := stream.Tags.Title
			if title == "" {
				title = formatSubTitle(lang, stream.CodecName, subIdx)
			}
			meta.SubtitleTracks = append(meta.SubtitleTracks, SubtitleTrack{
				Index:       subIdx,
				StreamIndex: stream.Index,
				Language:    lang,
				Title:       title,
				Codec:       stream.CodecName,
				IsDefault:   stream.Disposition.Default == 1,
				IsForced:    stream.Disposition.Forced == 1,
			})
			subIdx++
		}
	}

	if meta.VideoCodec == "" {
		return nil, fmt.Errorf("ffprobe returned no video stream")
	}
	diagnostics.Logger(ctx, a.logger).Debug("media.probe_completed", "video_codec", meta.VideoCodec, "audio_tracks", meta.AudioTracks, "subtitle_tracks", meta.SubtitleTracks, "duration", meta.DurationSec)
	return meta, nil
}

func formatAudioTitle(lang string, channels int, codec string, idx int) string {
	langName := getLanguageName(lang)
	chStr := "Stereo"
	if channels >= 6 {
		chStr = "5.1 Surround"
	} else if channels == 1 {
		chStr = "Mono"
	}
	return fmt.Sprintf("%s (%s, %s)", langName, chStr, strings.ToUpper(codec))
}

func formatSubTitle(lang string, codec string, idx int) string {
	langName := getLanguageName(lang)
	return fmt.Sprintf("%s (%s)", langName, strings.ToUpper(codec))
}

func getLanguageName(code string) string {
	switch strings.ToLower(code) {
	case "jpn", "ja", "japanese":
		return "Japanese"
	case "eng", "en", "english":
		return "English"
	case "fre", "fra", "fr", "french":
		return "French"
	case "ger", "deu", "de", "german":
		return "German"
	case "spa", "es", "spanish":
		return "Spanish"
	case "ita", "it", "italian":
		return "Italian"
	case "por", "pt", "portuguese":
		return "Portuguese"
	case "rus", "ru", "russian":
		return "Russian"
	case "ara", "ar", "arabic":
		return "Arabic"
	case "chi", "zho", "zh", "chinese":
		return "Chinese"
	case "kor", "ko", "korean":
		return "Korean"
	default:
		if code == "und" || code == "" {
			return "Track"
		}
		return strings.ToUpper(code)
	}
}

// FormatDuration formats seconds into MM:SS or HH:MM:SS.
func FormatDuration(seconds float64) string {
	if seconds <= 0 {
		return "00:00"
	}
	totalSec := int(seconds)
	h := totalSec / 3600
	m := (totalSec % 3600) / 60
	s := totalSec % 60

	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

// GetResolutionLabel converts width and height to a human-readable tag (e.g. 1080p, 4K).
func GetResolutionLabel(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	if width >= 3800 || height >= 2100 {
		return "4K"
	}
	if width >= 2500 || height >= 1400 {
		return "1440p"
	}
	if width >= 1900 || height >= 1000 {
		return "1080p"
	}
	if width >= 1200 || height >= 700 {
		return "720p"
	}
	if width >= 800 || height >= 450 {
		return "480p"
	}
	if width >= 600 || height >= 350 {
		return "360p"
	}
	return "SD"
}

func findFFprobePath() string {
	if p := os.Getenv("FFPROBE_PATH"); p != "" {
		if _, err := exec.LookPath(p); err == nil {
			return p
		}
	}
	if p, err := exec.LookPath("ffprobe"); err == nil {
		return p
	}
	candidates := []string{
		"/usr/local/bin/ffprobe",
		"/usr/bin/ffprobe",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return "ffprobe"
}

// A torrent reader can otherwise outlive FFprobe's timeout while waiting for a
// missing piece, preventing exec.Cmd's stdin copier from finishing.
type probeContextReader struct {
	ctx    context.Context
	reader interface {
		ReadContext(context.Context, []byte) (int, error)
	}
}

func (r *probeContextReader) Read(p []byte) (int, error) { return r.reader.ReadContext(r.ctx, p) }
