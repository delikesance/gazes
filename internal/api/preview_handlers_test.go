package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func previewTestRouter(s *Server) http.Handler {
	r := chi.NewRouter()
	r.Get("/seasons/{season}/episodes/{ep}/preview", s.HandleEpisodePreview)
	r.Post("/seasons/{season}/episodes/{ep}/preview", s.HandleEpisodePreviewCreate)
	return r
}

func TestEpisodePreviewMissingThenServed(t *testing.T) {
	s := &Server{}
	h := previewTestRouter(s)
	get := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/seasons/12/episodes/3/preview", nil))
		return rec
	}
	if rec := get(); rec.Code != http.StatusNotFound || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("miss: %d %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	s.previews().cache.Put(t.Context(), "12:3", &previewImage{Data: []byte("jpeg")}, time.Hour)
	rec := get()
	if rec.Code != http.StatusOK || rec.Body.String() != "jpeg" || rec.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("hit: %d %q %q", rec.Code, rec.Body.String(), rec.Header().Get("Content-Type"))
	}
	// An already stored frame is never cut again.
	post := httptest.NewRecorder()
	h.ServeHTTP(post, httptest.NewRequest("POST", "/seasons/12/episodes/3/preview?ih=0123456789abcdef0123456789abcdef01234567&duration=1400", nil))
	if post.Code != http.StatusNoContent {
		t.Fatalf("create on stored frame: %d", post.Code)
	}
}

func TestEpisodePreviewCreateRejectsBadInput(t *testing.T) {
	h := previewTestRouter(&Server{})
	for _, q := range []string{"", "?ih=zz&duration=1400", "?ih=0123456789abcdef0123456789abcdef01234567&duration=5", "?ih=0123456789abcdef0123456789abcdef01234567&duration=NaN"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("POST", "/seasons/12/episodes/3/preview"+q, nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%q: %d", q, rec.Code)
		}
	}
}
