package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func cacheRouter(e *pbEnv, calls *atomic.Int32, status int) chi.Router {
	r := chi.NewRouter()
	r.With(e.svc.AuthCached(ScopeMetricsRead, time.Minute)).Get("/heavy", func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"n":1}`))
	})
	r.With(e.svc.AuthCached(ScopeMetricsRead, time.Minute)).Post("/heavy", func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(204)
	})
	return r
}

func hit(r http.Handler, method, target string, mut func(*http.Request)) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	if mut != nil {
		mut(req)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestResponseCacheServesRepeatsUntilTheTTLEnds(t *testing.T) {
	e := opsEnv(t)
	var calls atomic.Int32
	r := cacheRouter(e, &calls, 200)
	tok := func(q *http.Request) { q.Header.Set("Authorization", "Bearer "+e.tokens["all"]) }

	if rec := hit(r, "GET", "/heavy?a=1", tok); rec.Code != 200 || rec.Header().Get("X-Gazes-Cache") != "miss" {
		t.Fatalf("first: %d %q", rec.Code, rec.Header().Get("X-Gazes-Cache"))
	}
	if rec := hit(r, "GET", "/heavy?a=1", tok); rec.Header().Get("X-Gazes-Cache") != "hit" || rec.Body.String() != `{"n":1}` || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("second should be a hit with the same body and headers: %q %q", rec.Header().Get("X-Gazes-Cache"), rec.Body.String())
	}
	if calls.Load() != 1 {
		t.Fatalf("handler ran %d times, want 1", calls.Load())
	}
	hit(r, "GET", "/heavy?a=2", tok) // another query is another entry
	if calls.Load() != 2 {
		t.Fatalf("different query must recompute: %d", calls.Load())
	}
	// a session and a token never share an entry (a session sees pseudos, a token does not)
	hit(r, "GET", "/heavy?a=1", pbAdminSession)
	if calls.Load() != 3 {
		t.Fatalf("a session must not reuse a token's answer: %d", calls.Load())
	}
	// after the TTL the answer is recomputed
	e.svc.now = func() time.Time { return at("2026-03-31T10:02:00Z") }
	hit(r, "GET", "/heavy?a=1", tok)
	if calls.Load() != 4 {
		t.Fatalf("expired entry must recompute: %d", calls.Load())
	}
}

func TestResponseCacheNeverBypassesAuthNorStoresErrors(t *testing.T) {
	e := opsEnv(t)
	var calls atomic.Int32
	r := cacheRouter(e, &calls, 200)
	tok := func(q *http.Request) { q.Header.Set("Authorization", "Bearer "+e.tokens["all"]) }
	hit(r, "GET", "/heavy", tok) // warms the entry
	if rec := hit(r, "GET", "/heavy", nil); rec.Code != 401 {
		t.Fatalf("a cached answer must not be served without credentials: %d", rec.Code)
	}
	if rec := hit(r, "GET", "/heavy", func(q *http.Request) { q.Header.Set("Authorization", "Bearer gzs_wrong") }); rec.Code != 401 {
		t.Fatalf("bad token: %d", rec.Code)
	}
	if rec := hit(r, "POST", "/heavy", tok); rec.Code != 204 {
		t.Fatalf("non-GET passes through: %d", rec.Code)
	}

	var failing atomic.Int32
	rf := cacheRouter(e, &failing, 500)
	for i := 0; i < 2; i++ {
		if rec := hit(rf, "GET", "/heavy?fail=1", tok); rec.Code != 500 {
			t.Fatalf("error answers are replayed as they are: %d", rec.Code)
		}
	}
	if failing.Load() != 2 {
		t.Fatalf("a 500 must never be cached: handler ran %d times", failing.Load())
	}
}

func TestResponseCacheSharesOneComputationBetweenConcurrentCalls(t *testing.T) {
	e := opsEnv(t)
	var calls atomic.Int32
	release := make(chan struct{})
	r := chi.NewRouter()
	r.With(e.svc.AuthCached(ScopeMetricsRead, time.Minute)).Get("/slow", func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		<-release
		_, _ = w.Write([]byte("ok"))
	})
	tok := func(q *http.Request) { q.Header.Set("Authorization", "Bearer "+e.tokens["all"]) }
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); hit(r, "GET", "/slow", tok) }()
	}
	time.Sleep(100 * time.Millisecond)
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("five concurrent identical requests ran the handler %d times, want 1", calls.Load())
	}
}

func TestResponseCacheIsBoundedAndClearable(t *testing.T) {
	var c responseCache
	for i := 0; i < responseCacheMax+50; i++ {
		c.store(string(rune('a'+i%26))+time.Duration(i).String(), cachedResponse{})
	}
	if len(c.items) != responseCacheMax {
		t.Fatalf("cache holds %d entries, want at most %d", len(c.items), responseCacheMax)
	}
	c.clear()
	if len(c.items) != 0 {
		t.Fatal("clear must drop everything")
	}
	_ = context.Background()
}
