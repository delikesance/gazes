package playback

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/gazes/gazes/internal/diagnostics"
	"github.com/gazes/gazes/internal/metadata"
	"github.com/gazes/gazes/internal/torrent"
	"github.com/google/uuid"
)

type Options struct {
	Directory, RawURL      string
	MemoryBytes, DiskBytes int64
}
type Session struct {
	ID          string                  `json:"id"`
	Duration    float64                 `json:"duration"`
	Origin      float64                 `json:"timeline_origin"`
	Playlist    string                  `json:"playlist_url"`
	Metadata    *metadata.VideoMetadata `json:"metadata"`
	Generation  uint64                  `json:"generation"`
	Position    float64                 `json:"position"`
	index       *Index
	hash        string
	file, audio int
	used        time.Time
}
type job struct {
	done        chan struct{}
	cancel      context.CancelFunc
	owners      map[string]bool
	init, media string
	err         error
	used        time.Time
	bytes       int64
	memory      []byte
}
type Manager struct {
	mu       sync.Mutex
	engine   torrent.Engine
	analyzer metadata.Analyzer
	logger   *slog.Logger
	opts     Options
	sessions map[string]*Session
	jobs     map[string]*job
	closed   chan struct{}
	workers  chan struct{}
}

func New(engine torrent.Engine, analyzer metadata.Analyzer, logger *slog.Logger, opts Options) *Manager {
	if opts.MemoryBytes <= 0 {
		opts.MemoryBytes = 64 << 20
	}
	if opts.DiskBytes <= 0 {
		opts.DiskBytes = 1 << 30
	}
	m := &Manager{engine: engine, analyzer: analyzer, logger: logger, opts: opts, sessions: map[string]*Session{}, jobs: map[string]*job{}, closed: make(chan struct{}), workers: make(chan struct{}, 4)}
	go m.sweep()
	return m
}
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	select {
	case <-m.closed:
		return
	default:
		close(m.closed)
	}
	for _, j := range m.jobs {
		j.cancel()
	}
}
func (m *Manager) Create(ctx context.Context, hash string, file, audio int, position float64) (*Session, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	r, info, err := m.engine.GetFileStream(ctx, hash, file)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	idx, err := ReadIndex(ctx, r, info.Length)
	if err != nil {
		return nil, err
	}
	if _, err = r.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	meta, err := m.analyzer.ProbeReader(ctx, r, info.Length)
	if err != nil || meta == nil {
		return nil, fmt.Errorf("media metadata unavailable: %v", err)
	}
	if audio < 0 || (len(meta.AudioTracks) > 0 && audio >= len(meta.AudioTracks)) {
		return nil, fmt.Errorf("audio track unavailable")
	}
	if !finite(position) || position < 0 {
		return nil, fmt.Errorf("invalid position")
	}
	position = min(position, idx.Duration)
	s := &Session{ID: uuid.NewString(), Duration: idx.Duration, Origin: idx.Origin, Metadata: meta, index: idx, hash: hash, file: file, audio: audio, Position: position, used: time.Now()}
	s.Playlist = "/api/v1/playback/sessions/" + s.ID + "/index.m3u8"
	m.mu.Lock()
	m.sessions[s.ID] = s
	m.mu.Unlock()
	return cloneSession(s), nil
}
func cloneSession(s *Session) *Session { v := *s; return &v }
func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil, false
	}
	s.used = time.Now()
	return cloneSession(s), true
}

// SubtitleStart includes active dialogues and the preceding display state.
// Only indexed cues are used; a subtitle request never scans the whole file.
func (m *Manager) SubtitleStart(id, hash string, file, track int, position float64) (float64, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok || s.hash != hash || s.file != file { return 0, false }
	start := position
	latest := -1.0
	for _, cue := range s.index.Subtitles[track] {
		t := cue.Start-s.Origin
		if t <= position && t > latest { latest = t }
		if t <= position && cue.Duration > 0 && t+cue.Duration > position { start = min(start,t) }
	}
	if latest >= 0 { start = min(start, latest) }
	return max(0,start), true
}
func (m *Manager) Update(id string, position float64, generation uint64) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok || !finite(position) || position < 0 {
		return nil, false
	}
	s.used = time.Now()
	if generation < s.Generation {
		return cloneSession(s), true
	}
	s.Generation = generation
	s.Position = min(position, s.Duration)
	for key, j := range m.jobs {
		if j.owners[id] && !m.wanted(s, key) {
			delete(j.owners, id)
			if len(j.owners) == 0 {
				select {
				case <-j.done:
				default:
					j.cancel()
					delete(m.jobs, key)
				}
			}
		}
	}
	return cloneSession(s), true
}
func (m *Manager) Delete(id string) { m.mu.Lock(); defer m.mu.Unlock(); m.removeSession(id) }
func (m *Manager) removeSession(id string) {
	delete(m.sessions, id)
	for key, j := range m.jobs {
		delete(j.owners, id)
		if len(j.owners) == 0 {
			select {
			case <-j.done:
			default:
				j.cancel()
				delete(m.jobs, key)
			}
		}
	}
}
func mediaKey(s *Session, n int) string {
	return fmt.Sprintf("%s/%d/%d/%d", s.hash, s.file, s.audio, n)
}
func (m *Manager) wanted(s *Session, key string) bool {
	for n, seg := range s.index.Segments {
		if key == mediaKey(s, n) {
			return seg.End >= s.Position-10 && seg.Start <= s.Position+30
		}
	}
	return false
}
func (s *Session) SegmentAt(position float64) int {
	n := sort.Search(len(s.index.Segments), func(n int) bool { return s.index.Segments[n].End > position })
	return min(n, len(s.index.Segments)-1)
}
func (m *Manager) Playlist(s *Session) string {
	maxDuration := float64(1)
	for _, seg := range s.index.Segments {
		maxDuration = max(maxDuration, seg.End-seg.Start)
	}
	text := fmt.Sprintf("#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-TARGETDURATION:%d\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-INDEPENDENT-SEGMENTS\n", int(math.Ceil(maxDuration)))
	// Each segment's init is equivalent after normalization. A per-segment map
	// lets a cold seek bootstrap without producing the episode's first segment.
	for n, seg := range s.index.Segments {
		text += fmt.Sprintf("#EXT-X-MAP:URI=\"%d/init.mp4\"\n#EXTINF:%.6f,\n%d/media.m4s\n", n, seg.End-seg.Start, n)
	}
	return text + "#EXT-X-ENDLIST\n"
}
func (m *Manager) Media(ctx context.Context, id string, n int, init bool, w http.ResponseWriter, r *http.Request) error {
	m.mu.Lock()
	s, ok := m.sessions[id]
	if !ok || n < 0 || n >= len(s.index.Segments) {
		m.mu.Unlock()
		return os.ErrNotExist
	}
	key := mediaKey(s, n)
	j := m.jobs[key]
	if j == nil {
		if !m.wanted(s, key) {
			m.mu.Unlock()
			return fmt.Errorf("segment outside playback window")
		}
		// Production belongs to viewers, not one HTTP connection (init/media share
		// it). Preserve diagnostics, but cancel through session leases or timeout.
		workCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 25*time.Second)
		j = &job{done: make(chan struct{}), cancel: cancel, owners: map[string]bool{}, used: time.Now()}
		m.jobs[key] = j
		snapshot := cloneSession(s)
		go m.produce(workCtx, key, j, snapshot, n)
	}
	j.owners[id] = true
	j.used = time.Now()
	m.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-j.done:
	}
	if j.err != nil {
		return j.err
	}
	m.mu.Lock()
	path := j.media
	if init {
		path = j.init
	}
	if !init && j.memory != nil {
		cached := j.memory
		j.used = time.Now()
		m.mu.Unlock()
		w.Header().Set("Cache-Control", "private, max-age=120")
		w.Header().Set("Content-Type", "video/mp4")
		http.ServeContent(w, r, "media.m4s", time.Time{}, bytes.NewReader(cached))
		return nil
	}
	f, err := os.Open(path)
	if err == nil {
		j.used = time.Now()
	}
	m.mu.Unlock()
	if err != nil {
		return err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return err
	}
	w.Header().Set("Cache-Control", "private, max-age=120")
	w.Header().Set("Content-Type", "video/mp4")
	http.ServeContent(w, r, filepath.Base(path), stat.ModTime(), f)
	return nil
}
func (m *Manager) produce(ctx context.Context, key string, j *job, s *Session, n int) {
	defer j.cancel()
	select {
	case m.workers <- struct{}{}:
		defer func() { <-m.workers }()
	case <-ctx.Done():
		m.finish(key, j, ctx.Err())
		return
	}
	sum := sha256.Sum256([]byte(key))
	name := hex.EncodeToString(sum[:])
	dir, err := os.MkdirTemp(m.opts.Directory, "hls-"+name[:8]+"-")
	if err != nil {
		m.finish(key, j, err)
		return
	}
	success := false
	defer func() {
		if !success {
			os.RemoveAll(dir)
		}
	}()
	seg := s.index.Segments[n]
	output := filepath.Join(dir, "window.mp4")
	bin := os.Getenv("FFMPEG_PATH")
	if bin == "" {
		bin = "ffmpeg"
	}
	input := fmt.Sprintf("%s?ih=%s&file_idx=%d&playback_session_id=%s", m.opts.RawURL, s.hash, s.file, s.ID)
	args := []string{"-hide_banner", "-loglevel", "error", "-copyts", "-noaccurate_seek", "-ss", strconv.FormatFloat(seg.Start+s.Origin, 'f', 6, 64), "-seek_timestamp", "1", "-i", input, "-map", "0:v:0", "-c:v", "copy", "-sn", "-dn"}
	if len(s.Metadata.AudioTracks) > 0 {
		args = append(args, "-map", fmt.Sprintf("0:a:%d", s.audio))
		if s.Metadata.AudioTracks[s.audio].Codec == "aac" {
			args = append(args, "-c:a", "copy")
		} else {
			args = append(args, "-c:a", "aac", "-b:a", "128k", "-ac", "2")
		}
	}
	if s.Metadata.VideoCodec == "hevc" {
		args = append(args, "-tag:v", "hvc1")
	}
	args = append(args, "-to", strconv.FormatFloat(seg.End+s.Origin+0.25, 'f', 6, 64), "-f", "mp4", "-video_track_timescale", "90000", "-movflags", "empty_moov+default_base_moof+frag_keyframe+delay_moov", "-avoid_negative_ts", "disabled", "-y", output)
	cmd := exec.CommandContext(ctx, bin, args...)
	var stderr diagnostics.LimitedBuffer
	cmd.Stderr = &stderr
	started := time.Now()
	err = cmd.Run()
	if err != nil {
		m.finish(key, j, fmt.Errorf("segment remux: %w: %s", err, stderr.String()))
		return
	}
	src, err := os.Open(output)
	if err != nil {
		m.finish(key, j, err)
		return
	}
	defer src.Close()
	initPath, mediaPath := filepath.Join(dir, "init.mp4"), filepath.Join(dir, "media.m4s")
	initFile, err := os.Create(initPath)
	if err != nil {
		m.finish(key, j, err)
		return
	}
	defer initFile.Close()
	mediaFile, err := os.Create(mediaPath)
	if err != nil {
		m.finish(key, j, err)
		return
	}
	defer mediaFile.Close()
	pts, err := SplitWindow(src, initFile, mediaFile, seg.Start, seg.End, s.Origin)
	if err != nil {
		m.finish(key, j, err)
		return
	}
	if err = ctx.Err(); err != nil {
		m.finish(key, j, err)
		return
	}
	os.Remove(output)
	a, _ := initFile.Stat()
	b, _ := mediaFile.Stat()
	m.mu.Lock()
	if m.jobs[key] != j || len(j.owners) == 0 || ctx.Err() != nil {
		m.mu.Unlock()
		m.finish(key, j, context.Canceled)
		return
	}
	j.init = initPath
	j.media = mediaPath
	j.bytes = a.Size() + b.Size()
	if b.Size() <= min(int64(8<<20), m.opts.MemoryBytes) {
		j.memory, _ = os.ReadFile(mediaPath)
		m.trimMemory()
	}
	m.mu.Unlock()
	success = true
	diagnostics.Logger(ctx, m.logger).Info("playback.segment_ready", "session_id", s.ID, "segment", n, "position", seg.Start, "video_pts", pts, "timeline_origin", s.Origin, "generation", s.Generation, "duration_ms", time.Since(started).Milliseconds())
	m.finish(key, j, nil)
}

func (m *Manager) trimMemory() {
	var total int64
	var keys []string
	for key, j := range m.jobs {
		if j.memory != nil {
			total += int64(len(j.memory))
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(a, b int) bool { return m.jobs[keys[a]].used.Before(m.jobs[keys[b]].used) })
	for _, key := range keys {
		if total <= m.opts.MemoryBytes {
			break
		}
		j := m.jobs[key]
		total -= int64(len(j.memory))
		j.memory = nil
	}
}
func (m *Manager) finish(key string, j *job, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j.err = err
	if err != nil && m.jobs[key] == j {
		delete(m.jobs, key)
	}
	close(j.done)
}
func (m *Manager) sweep() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.closed:
			return
		case now := <-ticker.C:
			m.mu.Lock()
			for id, s := range m.sessions {
				if now.Sub(s.used) > 60*time.Second {
					m.removeSession(id)
				}
			}
			var keys []string
			var total int64
			for key, j := range m.jobs {
				select {
				case <-j.done:
					if j.err == nil {
						total += j.bytes
						keys = append(keys, key)
					}
				default:
				}
			}
			sort.Slice(keys, func(a, b int) bool { return m.jobs[keys[a]].used.Before(m.jobs[keys[b]].used) })
			for _, key := range keys {
				j := m.jobs[key]
				if total > m.opts.DiskBytes || now.Sub(j.used) > 2*time.Minute {
					os.RemoveAll(filepath.Dir(j.media))
					delete(m.jobs, key)
					total -= j.bytes
				}
			}
			m.mu.Unlock()
		}
	}
}
