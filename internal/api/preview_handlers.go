package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"time"

	"github.com/gazes/gazes/internal/diagnostics"
	"github.com/gazes/gazes/internal/kv"
	"github.com/go-chi/chi/v5"
)

// Episodes the metadata provider has no still for get one cut from the middle of the file the
// viewer is playing. The JPEG lives only in Redis (process memory without it): there is no image
// host, and an evicted frame is simply cut again the next time someone plays the episode.
const (
	previewTTL           = 90 * 24 * time.Hour
	previewCutBudget     = 45 * time.Second
	previewMaxConcurrent = 2
	previewMaxBytes      = 512 << 10
)

var (
	errPreviewMissing = errors.New("episode preview not generated yet")
	infoHashPattern   = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)
)

// previewImage is JSON-encoded by the cache: Data travels as base64.
type previewImage struct {
	Data []byte `json:"data"`
}

type previewStore struct {
	cache *kv.Cache[previewImage]
	slots chan struct{}
}

func newPreviewStore(rc *kv.Client) *previewStore {
	return &previewStore{
		cache: kv.NewCache[previewImage](rc, "preview", kv.CacheOptions{L1TTL: time.Minute, L1Max: 64, FetchTimeout: previewCutBudget, WaitBudget: previewCutBudget + 5*time.Second}),
		slots: make(chan struct{}, previewMaxConcurrent),
	}
}

var previewPolicy = kv.Policy[previewImage]{
	TTL:      previewTTL,
	StaleFor: func(*previewImage) time.Duration { return 0 },
}

func (s *Server) previews() *previewStore {
	s.previewOnce.Do(func() {
		if s.previewStore == nil {
			s.previewStore = newPreviewStore(s.kv)
		}
	})
	return s.previewStore
}

func previewKey(r *http.Request) (string, bool) {
	season, e1 := strconv.Atoi(chi.URLParam(r, "season"))
	ep, e2 := strconv.Atoi(chi.URLParam(r, "ep"))
	if e1 != nil || e2 != nil || season <= 0 || ep <= 0 {
		return "", false
	}
	return fmt.Sprintf("%d:%d", season, ep), true
}

// HandleEpisodePreview serves the stored frame, or 404 when none was cut yet.
func (s *Server) HandleEpisodePreview(w http.ResponseWriter, r *http.Request) {
	key, ok := previewKey(r)
	if !ok {
		http.Error(w, "invalid season or episode", http.StatusBadRequest)
		return
	}
	img, err := s.previews().cache.Get(r.Context(), key, previewPolicy, func(context.Context) (*previewImage, error) { return nil, errPreviewMissing })
	if err != nil || img == nil || len(img.Data) == 0 {
		w.Header().Set("Cache-Control", "no-store")
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeContent(w, r, "preview.jpg", time.Time{}, bytes.NewReader(img.Data))
}

// HandleEpisodePreviewCreate cuts the frame at the middle of the file being played, unless one is
// already stored. The player calls it once per playback and ignores the answer.
func (s *Server) HandleEpisodePreviewCreate(w http.ResponseWriter, r *http.Request) {
	key, ok := previewKey(r)
	ih := r.URL.Query().Get("ih")
	duration, err := strconv.ParseFloat(r.URL.Query().Get("duration"), 64)
	if !ok || !infoHashPattern.MatchString(ih) || err != nil || math.IsNaN(duration) || duration < 120 || duration > 86400 {
		http.Error(w, "invalid preview request", http.StatusBadRequest)
		return
	}
	fileIdx := 0
	if v, err := strconv.Atoi(r.URL.Query().Get("file_idx")); err == nil && v >= 0 {
		fileIdx = v
	}
	store := s.previews()
	if img, err := store.cache.Get(r.Context(), key, previewPolicy, func(context.Context) (*previewImage, error) { return nil, errPreviewMissing }); err == nil && img != nil && len(img.Data) > 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	select {
	case store.slots <- struct{}{}:
	default:
		http.Error(w, "preview generation busy", http.StatusTooManyRequests)
		return
	}
	// The cut outlives the request: the player may close before ffmpeg is done.
	logger := diagnostics.Logger(r.Context(), s.logger)
	go func() {
		defer func() { <-store.slots }()
		ctx, cancel := context.WithTimeout(context.Background(), previewCutBudget)
		defer cancel()
		data, err := s.cutFrame(ctx, ih, fileIdx, duration/2)
		if err != nil {
			logger.Warn("episode preview not generated", "key", key, "err", err)
			return
		}
		store.cache.Put(ctx, key, &previewImage{Data: data}, previewTTL)
	}()
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) cutFrame(ctx context.Context, ih string, fileIdx int, at float64) ([]byte, error) {
	port := 8090
	if s.cfg != nil && s.cfg.Port > 0 {
		port = s.cfg.Port
	}
	input := fmt.Sprintf("http://127.0.0.1:%d/api/v1/stream/raw?ih=%s&file_idx=%d", port, ih, fileIdx)
	cmd := exec.CommandContext(ctx, findFFmpegBin(),
		"-hide_banner", "-loglevel", "error",
		"-probesize", "1000000", "-analyzeduration", "1000000",
		"-ss", strconv.FormatFloat(at, 'f', 2, 64), "-i", input,
		"-frames:v", "1", "-an", "-sn", "-vf", "scale=480:-2",
		"-q:v", "4", "-f", "mjpeg", "pipe:1")
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %w: %s", err, stderr.String())
	}
	if out.Len() == 0 || out.Len() > previewMaxBytes {
		return nil, fmt.Errorf("unusable frame of %d bytes", out.Len())
	}
	return out.Bytes(), nil
}
