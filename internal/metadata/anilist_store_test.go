package metadata

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

type storeProbe struct {
	Data struct {
		N int `json:"n"`
	} `json:"data"`
}

func TestAnilistStore(t *testing.T) {
	if err := OpenAnilistStore(filepath.Join(t.TempDir(), "a.sqlite")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(CloseAnilistStore)

	tr := &scheduleTransport{body: `{"data":{"n":7}}`}
	client := newAnilistClient(&http.Client{Transport: tr}, nil)
	ctx := context.Background()
	vars := map[string]any{"id": 1}

	var out storeProbe
	if err := client.post(ctx, "q", vars, &out); err != nil || out.Data.N != 7 {
		t.Fatalf("first call: %v %+v", err, out)
	}
	out = storeProbe{}
	if err := client.post(ctx, "q", vars, &out); err != nil || out.Data.N != 7 {
		t.Fatalf("second call: %v %+v", err, out)
	}
	if got := tr.calls.Load(); got != 1 {
		t.Fatalf("upstream calls = %d, want 1 (second answered from the store)", got)
	}

	// Past its max age an answer is fetched again, then served as a fallback when AniList fails.
	time.Sleep(1100 * time.Millisecond)
	tr.body = `{"data":{"n":8}}`
	if err := client.postFresh(ctx, "q", vars, &out, time.Second); err != nil || out.Data.N != 8 || tr.calls.Load() != 2 {
		t.Fatalf("refresh: %v %+v calls=%d", err, out, tr.calls.Load())
	}
	failing := newAnilistClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("down") })}, nil)
	time.Sleep(1100 * time.Millisecond)
	out = storeProbe{}
	if err := failing.postFresh(ctx, "q", vars, &out, time.Second); err != nil || out.Data.N != 8 {
		t.Fatalf("stale fallback: %v %+v", err, out)
	}

	// GraphQL errors and empty data are never stored.
	bad := &scheduleTransport{body: `{"errors":[{"message":"boom"}],"data":null}`}
	if err := newAnilistClient(&http.Client{Transport: bad}, nil).post(ctx, "q", map[string]any{"id": 2}, &out); err != nil {
		t.Fatal(err)
	}
	good := &scheduleTransport{body: `{"data":{"n":9}}`}
	out = storeProbe{}
	if err := newAnilistClient(&http.Client{Transport: good}, nil).post(ctx, "q", map[string]any{"id": 2}, &out); err != nil || out.Data.N != 9 || good.calls.Load() != 1 {
		t.Fatalf("errors were stored: %v %+v", err, out)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
