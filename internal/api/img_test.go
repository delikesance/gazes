package api

import (
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gazes/gazes/internal/config"
)

// The image route is public and must refuse anything that is not AniList artwork, before any
// network call (the host check is the SSRF guard).
func TestImageRouteRefusesForeignHosts(t *testing.T) {
	router := NewServer(&config.Config{CatalogDir: t.TempDir()}, nil, nil, nil, nil).Router()
	for _, raw := range []string{"http://169.254.169.254/", "https://example.com/a.jpg", ""} {
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, httptest.NewRequest("GET", "/api/v1/img?u="+url.QueryEscape(raw), nil))
		if rr.Code != 400 {
			t.Errorf("%q: code=%d", raw, rr.Code)
		}
	}
}
