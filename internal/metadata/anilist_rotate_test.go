package metadata

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

type statusTransport struct {
	status atomic.Int32
	calls  atomic.Int32
}

func (t *statusTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	t.calls.Add(1)
	code := int(t.status.Load())
	body := ""
	if code == 200 {
		body = `{"data":{}}`
	}
	return &http.Response{StatusCode: code, Header: http.Header{"Retry-After": {"60"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
}

func TestAnilistRotatesOnThrottle(t *testing.T) {
	for _, code := range []int32{429, 502, 503} {
		t.Run(http.StatusText(int(code)), func(t *testing.T) {
			tr := &statusTransport{}
			tr.status.Store(code)
			var rotations atomic.Int32
			SetAnilistRotator(func(context.Context) error {
				rotations.Add(1)
				tr.status.Store(200) // the new exit IP is not throttled
				return nil
			})
			t.Cleanup(func() { SetAnilistRotator(nil) })

			a := newAnilistClient(&http.Client{Transport: tr}, nil)
			if _, err := a.fetch(context.Background(), "{x}", nil); err != nil {
				t.Fatalf("fetch after rotation: %v", err)
			}
			if rotations.Load() != 1 || tr.calls.Load() != 2 {
				t.Fatalf("rotations=%d calls=%d, want 1 and 2", rotations.Load(), tr.calls.Load())
			}
			// A 429 must not leave the shared cooldown armed for the new IP.
			if d := a.gov.Cooldown(context.Background()); d != 0 {
				t.Fatalf("cooldown %v still armed after rotation", d)
			}
		})
	}
}

func TestAnilistRotationRefusedKeepsError(t *testing.T) {
	tr := &statusTransport{}
	tr.status.Store(429)
	SetAnilistRotator(func(context.Context) error { return errors.New("cooldown") })
	t.Cleanup(func() { SetAnilistRotator(nil) })

	a := newAnilistClient(&http.Client{Transport: tr}, nil)
	_, err := a.fetch(context.Background(), "{x}", nil)
	var limited *RateLimitError
	if !errors.As(err, &limited) || tr.calls.Load() != 1 {
		t.Fatalf("err=%v calls=%d, want RateLimitError and no retry", err, tr.calls.Load())
	}
}

func TestAnilistWithoutRotatorUnchanged(t *testing.T) {
	tr := &statusTransport{}
	tr.status.Store(503)
	a := newAnilistClient(&http.Client{Transport: tr}, nil)
	if _, err := a.fetch(context.Background(), "{x}", nil); err == nil || tr.calls.Load() != 1 {
		t.Fatalf("err=%v calls=%d, want one failing call", err, tr.calls.Load())
	}
}
