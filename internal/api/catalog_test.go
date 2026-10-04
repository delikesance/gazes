package api

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gazes/gazes/internal/diagnostics"
	"github.com/gazes/gazes/internal/metadata"
)

func TestCatalogFailureMapsThrottleTo503AndKeepsRequestID(t *testing.T) {
	s := &Server{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	for _, tc := range []struct {
		err    error
		status int
		retry  string
	}{
		{&metadata.RateLimitError{RetryAfter: 6 * time.Second}, 503, "7"},
		{fmt.Errorf("wrapped: %w", &metadata.RateLimitError{RetryAfter: time.Second}), 503, "2"},
		{errors.New("boom"), 500, ""},
	} {
		rr := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/v1/catalog/search?q=x", nil)
		r = r.WithContext(diagnostics.With(r.Context(), diagnostics.Correlation{RequestID: "req-1"}))
		s.catalogFailure(rr, r, "failed", "failed to search catalog", tc.err)
		if rr.Code != tc.status || rr.Header().Get("Retry-After") != tc.retry || rr.Header().Get("X-Request-ID") != "req-1" || !strings.Contains(rr.Body.String(), `"request_id":"req-1"`) {
			t.Errorf("%v: code=%d retry=%q body=%s", tc.err, rr.Code, rr.Header().Get("Retry-After"), rr.Body)
		}
	}
}
