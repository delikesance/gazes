package diagnostics

import (
	"context"
	"github.com/google/uuid"
	"log/slog"
	"regexp"
)

type contextKey struct{}
type Correlation struct {
	RequestID string `json:"request_id,omitempty"`
	SessionID string `json:"playback_session_id,omitempty"`
	AttemptID string `json:"attempt_id,omitempty"`
	AnimeID   string `json:"anime_id,omitempty"`
	SeasonID  string `json:"season_id,omitempty"`
	Episode   string `json:"episode,omitempty"`
}

var validID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`)

func ID(value string) string {
	if validID.MatchString(value) {
		return value
	}
	return uuid.NewString()
}
func With(ctx context.Context, c Correlation) context.Context {
	return context.WithValue(ctx, contextKey{}, c)
}
func Get(ctx context.Context) Correlation { c, _ := ctx.Value(contextKey{}).(Correlation); return c }
func Log(ctx context.Context, level slog.Level, event string, attrs ...any) {
	slog.Default().Log(ctx, level, event, attrs...)
}

// Logger attaches correlation for libraries whose APIs use a logger rather than LogContext.
func Logger(ctx context.Context, logger *slog.Logger) *slog.Logger {
	if logger == nil {
		logger = slog.Default()
	}
	c := Get(ctx)
	return logger.With("request_id", c.RequestID, "playback_session_id", c.SessionID, "attempt_id", c.AttemptID, "anime_id", c.AnimeID, "season_id", c.SeasonID, "episode", c.Episode)
}

// StandardWriter routes third-party standard-library log output through redaction.
type StandardWriter struct{}

func (StandardWriter) Write(p []byte) (int, error) {
	Log(context.Background(), slog.LevelInfo, "library.log", "message", string(p))
	return len(p), nil
}
