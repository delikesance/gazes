package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gazes/gazes/internal/metadata"
	"github.com/go-chi/chi/v5"
)

// skipTransport answers AniList and AniSkip by host.
type skipTransport struct {
	idMal      any // nil -> null
	anilistErr bool
	aniskip    int // HTTP status of the AniSkip answer
	aniskipHit atomic.Int32
}

func (t *skipTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	resp := func(status int, body string) (*http.Response, error) {
		return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader([]byte(body))), Header: make(http.Header)}, nil
	}
	if r.URL.Host == "api.aniskip.com" {
		t.aniskipHit.Add(1)
		if t.aniskip != 200 {
			return resp(t.aniskip, "boom")
		}
		return resp(200, `{"found":true,"statusCode":200,"results":[
			{"interval":{"startTime":10,"endTime":95},"skipType":"op","episodeLength":1440},
			{"interval":{"startTime":1330,"endTime":1420},"skipType":"ed","episodeLength":1440}]}`)
	}
	if t.anilistErr {
		return resp(200, `{"errors":[{"message":"Not Found"}],"data":{"Media":null}}`)
	}
	body, _ := json.Marshal(map[string]any{"data": map[string]any{"Media": map[string]any{
		"id": 5, "idMal": t.idMal, "title": map[string]any{"english": "Example"}, "format": "TV", "status": "FINISHED", "episodes": 12,
	}}})
	return resp(200, string(body))
}

func skipRequest(t *testing.T, tr *skipTransport, path string) *httptest.ResponseRecorder {
	t.Helper()
	s := &Server{catalogService: metadata.NewAnimeCatalogService(&http.Client{Transport: tr})}
	router := chi.NewRouter()
	router.Get("/seasons/{season}/episodes/{ep}/skip-times", s.HandleSkipTimes)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest("GET", path, nil))
	return rr
}

func TestSkipTimesRejectsInvalidParams(t *testing.T) {
	for _, path := range []string{
		"/seasons/x/episodes/1/skip-times?duration=1440",
		"/seasons/5/episodes/0/skip-times?duration=1440",
		"/seasons/5/episodes/1/skip-times",
		"/seasons/5/episodes/1/skip-times?duration=-1",
		"/seasons/5/episodes/1/skip-times?duration=NaN",
		"/seasons/5/episodes/1/skip-times?duration=30000",
	} {
		if rr := skipRequest(t, &skipTransport{idMal: 37430, aniskip: 200}, path); rr.Code != 400 {
			t.Errorf("%s: %d", path, rr.Code)
		}
	}
}

func TestSkipTimesReturnsSegments(t *testing.T) {
	tr := &skipTransport{idMal: 37430, aniskip: 200}
	rr := skipRequest(t, tr, "/seasons/5/episodes/1/skip-times?duration=1440")
	var out struct {
		Segments []metadata.SkipSegment `json:"segments"`
	}
	json.Unmarshal(rr.Body.Bytes(), &out)
	if rr.Code != 200 || len(out.Segments) != 2 {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
	if cc := rr.Header().Get("Cache-Control"); cc != "private, max-age=3600" {
		t.Fatalf("cache-control %q", cc)
	}
}

func assertEmptyNoStore(t *testing.T, rr *httptest.ResponseRecorder) {
	t.Helper()
	if rr.Code != 200 || strings.TrimSpace(rr.Body.String()) != `{"segments":[]}` {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
	if cc := rr.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("cache-control %q", cc)
	}
}

func TestSkipTimesWithoutMalID(t *testing.T) {
	tr := &skipTransport{aniskip: 200}
	assertEmptyNoStore(t, skipRequest(t, tr, "/seasons/5/episodes/1/skip-times?duration=1440"))
	if tr.aniskipHit.Load() != 0 {
		t.Fatal("AniSkip must not be called without a MAL id")
	}
}

func TestSkipTimesAniSkipDown(t *testing.T) {
	tr := &skipTransport{idMal: 37430, aniskip: 500}
	assertEmptyNoStore(t, skipRequest(t, tr, "/seasons/5/episodes/1/skip-times?duration=1440"))
}

func TestSkipTimesUnknownSeason(t *testing.T) {
	tr := &skipTransport{anilistErr: true, aniskip: 200}
	assertEmptyNoStore(t, skipRequest(t, tr, "/seasons/5/episodes/1/skip-times?duration=1440"))
	if tr.aniskipHit.Load() != 0 {
		t.Fatal("AniSkip must not be called for an unknown season")
	}
}
