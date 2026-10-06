package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/gazes/gazes/internal/diagnostics"
	"github.com/gazes/gazes/internal/indexer"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func (s *Server) diagnosticContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := diagnostics.Correlation{RequestID: diagnostics.ID(middleware.GetReqID(r.Context()))}
		if v := r.Header.Get("X-Playback-Session-ID"); v != "" {
			c.SessionID = diagnostics.ID(v)
		}
		if v := r.Header.Get("X-Playback-Attempt-ID"); v != "" {
			c.AttemptID = diagnostics.ID(v)
		}
		if c.SessionID == "" && strings.HasSuffix(r.URL.Path, "/sources") {
			c.SessionID = diagnostics.ID("")
		}
		if c.SessionID == "" && r.URL.Query().Get("playback_session_id") != "" {
			c.SessionID = diagnostics.ID(r.URL.Query().Get("playback_session_id"))
		}
		if c.AttemptID == "" && r.URL.Query().Get("attempt_id") != "" {
			c.AttemptID = diagnostics.ID(r.URL.Query().Get("attempt_id"))
		}
		c.AnimeID = r.Header.Get("X-Playback-Anime-ID")
		c.SeasonID = r.Header.Get("X-Playback-Season-ID")
		c.Episode = r.Header.Get("X-Playback-Episode")
		if c.AnimeID == "" {
			c.AnimeID = r.URL.Query().Get("anime_id")
		}
		if c.SeasonID == "" {
			c.SeasonID = r.URL.Query().Get("season_id")
		}
		if c.Episode == "" {
			c.Episode = r.URL.Query().Get("episode")
		}
		for _, p := range []*string{&c.AnimeID, &c.SeasonID, &c.Episode} {
			if len(*p) > 12 {
				*p = ""
				continue
			}
			for _, digit := range *p {
				if digit < '0' || digit > '9' {
					*p = ""
					break
				}
			}
		}
		ctx := diagnostics.With(r.Context(), c)
		w.Header().Set("X-Request-ID", c.RequestID)
		if c.SessionID != "" {
			w.Header().Set("X-Playback-Session-ID", c.SessionID)
		}
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		defer func() {
			level := slog.LevelDebug
			if ww.Status() >= 500 {
				level = slog.LevelError
			}
			diagnostics.Log(ctx, level, "http.request", "method", r.Method, "path", r.URL.Path, "status", ww.Status(), "bytes", ww.BytesWritten(), "duration_ms", time.Since(start).Milliseconds(), "canceled", ctx.Err() != nil, "infohash", r.URL.Query().Get("ih"), "file_index", r.URL.Query().Get("file_idx"))
		}()
		next.ServeHTTP(ww, r.WithContext(ctx))
	})
}
func sourceContext(w http.ResponseWriter, r *http.Request) *http.Request {
	c := diagnostics.Get(r.Context())
	if c.RequestID == "" {
		c.RequestID = diagnostics.ID("")
		w.Header().Set("X-Request-ID", c.RequestID)
	}
	c.AnimeID = chi.URLParam(r, "id")
	c.SeasonID = chi.URLParam(r, "season")
	c.Episode = chi.URLParam(r, "ep")
	if c.SessionID == "" {
		c.SessionID = diagnostics.ID("")
	}
	w.Header().Set("X-Playback-Session-ID", c.SessionID)
	return r.WithContext(diagnostics.With(r.Context(), c))
}

var telemetryIDs = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`)
var telemetryHash = regexp.MustCompile(`^[a-fA-F0-9]{40}$`)
var telemetryCodes = regexp.MustCompile(`^[a-zA-Z0-9_.-]{0,80}$`)
var telemetryEvents = map[string]bool{"playback.attempt": true, "playback.file_selected": true, "playback.file_rejected": true, "playback.started": true, "playback.buffering": true, "playback.failed": true, "playback.abandoned": true, "playback.exhausted": true, "playback.swarm": true, "playback.media_error": true, "playback.gesture_required": true, "playback.seek": true, "playback.tracks": true, "playback.resolution": true, "browser.error": true, "playback.subtitle_failed": true, "playback.discovery_started": true, "playback.discovery_completed": true, "playback.discovery_failed": true, "playback.vf_deferred": true}
var telemetryAttrs = map[string]bool{"reason": true, "error_code": true, "position": true, "duration": true, "file_index": true, "file_path": true, "file_count": true, "ready_state": true, "network_state": true, "video_codec": true, "audio_codec": true, "buffered_seconds": true, "seeders": true, "download_speed": true, "pieces_ready": true, "buffer_percent": true, "width": true, "height": true, "source_count": true, "audio_track": true, "subtitle_track": true, "partial": true, "startup_ms": true}

type clientEvent struct {
	Event      string         `json:"event"`
	Session    string         `json:"playback_session_id"`
	Attempt    string         `json:"attempt_id"`
	Anime      string         `json:"anime_id"`
	Season     string         `json:"season_id"`
	Episode    string         `json:"episode"`
	InfoHash   string         `json:"infohash"`
	Attributes map[string]any `json:"attributes"`
}
type rateWindow struct {
	since time.Time
	span  time.Duration
	count int
}

const rateLimiterMaxKeys = 10000

// rateLimiter is an in-memory fixed-window counter. The zero value is ready to use.
type rateLimiter struct {
	mu      sync.Mutex
	windows map[string]rateWindow
}

// hit counts one event for key and reports whether it is within limit for the current span.
func (l *rateLimiter) hit(key string, limit int, span time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if l.windows == nil {
		l.windows = map[string]rateWindow{}
	}
	if len(l.windows) >= rateLimiterMaxKeys {
		for k, w := range l.windows {
			if now.Sub(w.since) >= w.span {
				delete(l.windows, k)
			}
		}
		// Still full of live windows: shed some so new clients are never locked out and the next
		// calls do not rescan the whole table. Single-hit windows go first: a flood of fresh keys
		// (one per IPv6 /64) must not reset the counters that are actually throttling someone.
		for _, maxCount := range []int{1, int(^uint(0) >> 1)} {
			for k, w := range l.windows {
				if len(l.windows) < rateLimiterMaxKeys-rateLimiterMaxKeys/10 {
					break
				}
				if w.count <= maxCount {
					delete(l.windows, k)
				}
			}
		}
	}
	window := l.windows[key]
	if now.Sub(window.since) >= span {
		window = rateWindow{since: now, span: span}
	}
	window.count++
	l.windows[key] = window
	return window.count <= limit
}

// allow is the telemetry endpoint's budget: 120 events a minute per client.
func (l *rateLimiter) allow(key string) bool { return l.hit(key, 120, time.Minute) }

// validTelemetryEvent applies the allow-lists and format checks to one client event.
func validTelemetryEvent(e clientEvent) bool {
	if !telemetryEvents[e.Event] || !telemetryIDs.MatchString(e.Session) || (e.Attempt != "" && !telemetryIDs.MatchString(e.Attempt)) || (e.InfoHash != "" && !telemetryHash.MatchString(e.InfoHash)) {
		return false
	}
	for _, v := range []string{e.Anime, e.Season, e.Episode} {
		if len(v) > 12 {
			return false
		}
		for _, c := range v {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	if len(e.Attributes) > 20 {
		return false
	}
	for k, v := range e.Attributes {
		if !telemetryAttrs[k] {
			return false
		}
		switch value := v.(type) {
		case string:
			if len(value) > 2048 {
				return false
			}
		case float64, bool, nil:
		default:
			return false
		}
	}
	if code, ok := e.Attributes["error_code"].(string); ok && !telemetryCodes.MatchString(code) {
		return false
	}
	return true
}

func (s *Server) HandleDiagnosticEvents(w http.ResponseWriter, r *http.Request) {
	if !s.diagnosticRate.allow(s.clientKey(r)) {
		w.Header().Set("Retry-After", "60")
		http.Error(w, "diagnostic rate limit", 429)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var body struct {
		Events []clientEvent `json:"events"`
	}
	if err := decoder.Decode(&body); err != nil {
		http.Error(w, "invalid diagnostic events", 400)
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		http.Error(w, "invalid diagnostic events", 400)
		return
	}
	if len(body.Events) == 0 || len(body.Events) > 20 {
		http.Error(w, "invalid diagnostic event count", 400)
		return
	}
	// One unknown event or attribute must not discard the valid events sent in the same batch
	// (a failure report travels with swarm and track events): invalid events are skipped, and the
	// request is refused only when none is valid.
	valid := body.Events[:0]
	for _, e := range body.Events {
		if validTelemetryEvent(e) {
			valid = append(valid, e)
		}
	}
	if len(valid) == 0 {
		http.Error(w, "invalid diagnostic event", 400)
		return
	}
	body.Events = valid
	for _, e := range body.Events {
		c := diagnostics.Get(r.Context())
		c.SessionID = e.Session
		c.AttemptID = e.Attempt
		c.AnimeID = e.Anime
		c.SeasonID = e.Season
		c.Episode = e.Episode
		ctx := diagnostics.With(r.Context(), c)
		level := slog.LevelInfo
		if strings.Contains(e.Event, "failed") || e.Event == "playback.media_error" || e.Event == "browser.error" {
			level = slog.LevelWarn
		}
		attrs := []any{"service", "browser", "infohash", strings.ToLower(e.InfoHash), "client_reported", true}
		for k, v := range e.Attributes {
			attrs = append(attrs, k, v)
		}
		diagnostics.Log(ctx, level, e.Event, attrs...)
		if ms, ok := e.Attributes["startup_ms"].(float64); ok && e.Event == "playback.started" && s.admin != nil {
			s.admin.RecordStartup(ctx, ms)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// Keep errors useful to the caller without exposing provider URLs or secrets.
func sourceFailure(w http.ResponseWriter, r *http.Request, err error) {
	c := diagnostics.Get(r.Context())
	if errors.Is(err, indexer.ErrAuthorityMappingMissing) {
		diagnostics.Log(r.Context(), slog.LevelWarn, "resolver.failed", "error_code", "authority_mapping_missing", "err", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "authority_mapping_missing", "message": "Cet anime n'est pas encore lié à Sonarr ou Radarr.", "request_id": c.RequestID, "playback_session_id": c.SessionID})
		return
	}
	if errors.Is(err, indexer.ErrNoApprovedRelease) {
		diagnostics.Log(r.Context(), slog.LevelInfo, "resolver.empty", "error_code", "no_approved_release")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "no_approved_release", "message": "Sonarr ou Radarr n'a approuvé aucune release exploitable pour cet épisode.", "request_id": c.RequestID, "playback_session_id": c.SessionID})
		return
	}
	diagnostics.Log(r.Context(), slog.LevelError, "resolver.failed", "error_code", "providers_unavailable", "err", err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadGateway)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": "providers_unavailable", "message": "Les fournisseurs de torrents sont temporairement indisponibles.", "request_id": c.RequestID, "playback_session_id": c.SessionID})
}

func (s *Server) diagnosticRecover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				if recovered == http.ErrAbortHandler {
					panic(recovered)
				}
				diagnostics.Log(r.Context(), slog.LevelError, "http.panic", "error_code", "panic", "reason", fmt.Sprint(recovered), "stack", string(debug.Stack()))
				if wrapped, ok := w.(middleware.WrapResponseWriter); ok && wrapped.Status() != 0 {
					panic(http.ErrAbortHandler)
				}
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
