package imagecache

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

var png = append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{7}, 64)...)

type fakeCDN struct {
	calls  atomic.Int32
	status int
	ctype  string
	body   []byte
}

func (f *fakeCDN) RoundTrip(r *http.Request) (*http.Response, error) {
	f.calls.Add(1)
	status := f.status
	if status == 0 {
		status = 200
	}
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {f.ctype}}, Body: io.NopCloser(bytes.NewReader(f.body)), Request: r}, nil
}

func newHandler(t *testing.T, cdn *fakeCDN) (*Cache, string) {
	t.Helper()
	dir := t.TempDir()
	return New(Options{Dir: dir, Hosts: []string{"s4.anilist.co"}, Client: &http.Client{Transport: cdn}}), dir
}

func get(h http.Handler, raw string) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/api/v1/img?u="+url.QueryEscape(raw), nil))
	return rr
}

const cover = "https://s4.anilist.co/file/anilistcdn/media/anime/cover/large/bx1-abc.jpg"

func TestFetchedOnceThenServedFromDiskForever(t *testing.T) {
	cdn := &fakeCDN{ctype: "image/png", body: png}
	c, dir := newHandler(t, cdn)
	first := get(c, cover)
	if first.Code != 200 || !bytes.Equal(first.Body.Bytes(), png) || first.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("first: %d %q", first.Code, first.Header())
	}
	if cc := first.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") || !strings.Contains(cc, "max-age=31536000") {
		t.Fatalf("Cache-Control=%q", cc)
	}
	// A new process (new Cache, same dir) with the CDN gone still answers.
	dead := &fakeCDN{status: 500}
	again := New(Options{Dir: dir, Hosts: []string{"s4.anilist.co"}, Client: &http.Client{Transport: dead}})
	second := get(again, cover)
	if second.Code != 200 || !bytes.Equal(second.Body.Bytes(), png) || dead.calls.Load() != 0 || cdn.calls.Load() != 1 {
		t.Fatalf("second: %d upstream calls cdn=%d dead=%d", second.Code, cdn.calls.Load(), dead.calls.Load())
	}
	if second.Header().Get("ETag") == "" {
		t.Fatal("ETag expected")
	}
}

func TestConditionalRequestGets304(t *testing.T) {
	c, _ := newHandler(t, &fakeCDN{ctype: "image/png", body: png})
	etag := get(c, cover).Header().Get("ETag")
	rr := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/img?u="+url.QueryEscape(cover), nil)
	r.Header.Set("If-None-Match", etag)
	c.ServeHTTP(rr, r)
	if rr.Code != 304 {
		t.Fatalf("code=%d", rr.Code)
	}
}

func TestConcurrentMissesFetchOnce(t *testing.T) {
	cdn := &fakeCDN{ctype: "image/png", body: png}
	c, _ := newHandler(t, cdn)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if rr := get(c, cover); rr.Code != 200 {
				t.Errorf("code=%d", rr.Code)
			}
		}()
	}
	wg.Wait()
	if cdn.calls.Load() != 1 {
		t.Fatalf("upstream calls=%d", cdn.calls.Load())
	}
}

func TestOnlyAllowlistedHostsAreFetched(t *testing.T) {
	cdn := &fakeCDN{ctype: "image/png", body: png}
	c, _ := newHandler(t, cdn)
	for _, raw := range []string{
		"http://169.254.169.254/latest/meta-data",
		"https://evil.example/x.jpg",
		"https://s4.anilist.co.evil.example/x.jpg",
		"https://user@evil.example/x.jpg",
		"http://s4.anilist.co/x.jpg", // plain http is not fetched
		"file:///etc/passwd",
		"",
		"//s4.anilist.co/x.jpg",
	} {
		if rr := get(c, raw); rr.Code != 400 {
			t.Errorf("%q: code=%d", raw, rr.Code)
		}
	}
	if cdn.calls.Load() != 0 {
		t.Fatalf("a refused URL must never be fetched: %d", cdn.calls.Load())
	}
}

func TestRefusesNonImagesAndOversizedBodies(t *testing.T) {
	c, dir := newHandler(t, &fakeCDN{ctype: "text/html", body: []byte("<html>")})
	if rr := get(c, cover); rr.Code != 502 {
		t.Fatalf("html: code=%d", rr.Code)
	}
	big := New(Options{Dir: t.TempDir(), Hosts: []string{"s4.anilist.co"}, MaxBytes: 100, Client: &http.Client{Transport: &fakeCDN{ctype: "image/png", body: bytes.Repeat([]byte{1}, 500)}}})
	if rr := get(big, cover); rr.Code != 502 {
		t.Fatalf("oversized: code=%d", rr.Code)
	}
	// Neither left anything behind to be served as if valid.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("leftovers: %v", entries)
	}
}

func TestUpstreamFailureIsNotCachedAndRetries(t *testing.T) {
	cdn := &fakeCDN{status: 503, ctype: "text/plain", body: []byte("no")}
	c, _ := newHandler(t, cdn)
	if rr := get(c, cover); rr.Code != 502 {
		t.Fatalf("code=%d", rr.Code)
	}
	cdn.status, cdn.ctype, cdn.body = 200, "image/png", png
	if rr := get(c, cover); rr.Code != 200 {
		t.Fatalf("a later request must retry: %d", rr.Code)
	}
	// 404 from the CDN is passed on so the page shows its placeholder.
	nf := &fakeCDN{status: 404, ctype: "text/plain", body: []byte("x")}
	c2, _ := newHandler(t, nf)
	if rr := get(c2, cover); rr.Code != 404 {
		t.Fatalf("code=%d", rr.Code)
	}
}

func TestStoredUnderHashedPathInsideDir(t *testing.T) {
	c, dir := newHandler(t, &fakeCDN{ctype: "image/png", body: png})
	get(c, cover)
	var files []string
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, _ error) error {
		if !info.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	if len(files) != 1 || !strings.HasPrefix(files[0], dir) || strings.Contains(files[0], "anilist") {
		t.Fatalf("files=%v", files)
	}
}

func TestOnlyGETIsServed(t *testing.T) {
	c, _ := newHandler(t, &fakeCDN{ctype: "image/png", body: png})
	rr := httptest.NewRecorder()
	c.ServeHTTP(rr, httptest.NewRequest("POST", "/api/v1/img?u="+url.QueryEscape(cover), nil))
	if rr.Code != 405 {
		t.Fatalf("code=%d", rr.Code)
	}
}
