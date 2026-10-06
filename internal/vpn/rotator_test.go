package vpn

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type statusCall struct{ Status string }

func fakeGluetun(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var mu sync.Mutex
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/publicip/ip" {
			_, _ = w.Write([]byte(`{"public_ip":"203.0.113.7"}`))
			return
		}
		if r.Method != http.MethodPut || r.URL.Path != "/v1/vpn/status" {
			http.NotFound(w, r)
			return
		}
		var c statusCall
		_ = json.NewDecoder(r.Body).Decode(&c)
		mu.Lock()
		calls = append(calls, c.Status)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func fast(r *Rotator) *Rotator {
	r.poll = time.Millisecond
	r.retryWait = time.Millisecond
	return r
}

func TestRotate(t *testing.T) {
	t.Run("disabled is a no-op", func(t *testing.T) {
		r := New("", time.Minute)
		if r.Enabled() {
			t.Fatal("empty control URL must disable the rotator")
		}
		if err := r.Rotate(context.Background()); err != nil {
			t.Fatalf("disabled Rotate: %v", err)
		}
	})

	t.Run("stops then starts the tunnel", func(t *testing.T) {
		srv, calls := fakeGluetun(t)
		if err := fast(New(srv.URL, time.Minute)).Rotate(context.Background()); err != nil {
			t.Fatalf("Rotate: %v", err)
		}
		if got := *calls; len(got) != 2 || got[0] != "stopped" || got[1] != "running" {
			t.Fatalf("calls = %v, want [stopped running]", got)
		}
	})

	t.Run("cooldown refuses a rotation, a late caller is told to retry", func(t *testing.T) {
		srv, calls := fakeGluetun(t)
		now := time.Now()
		r := fast(New(srv.URL, 10*time.Minute))
		r.now = func() time.Time { return now }
		if err := r.Rotate(context.Background()); err != nil {
			t.Fatal(err)
		}
		// Within the settle window: someone just rotated, the caller retries on the new IP.
		if err := r.Rotate(context.Background()); err != nil {
			t.Fatalf("late caller err = %v, want nil", err)
		}
		now = now.Add(2 * time.Minute)
		if err := r.Rotate(context.Background()); err != ErrCooldown {
			t.Fatalf("after settle err = %v, want ErrCooldown", err)
		}
		if len(*calls) != 2 {
			t.Fatalf("calls = %v, want exactly one rotation", *calls)
		}
		now = now.Add(10 * time.Minute)
		if err := r.Rotate(context.Background()); err != nil {
			t.Fatalf("after cooldown: %v", err)
		}
		if len(*calls) != 4 {
			t.Fatalf("calls = %v, want a second rotation", *calls)
		}
	})

	t.Run("waits for the tunnel to answer", func(t *testing.T) {
		var polls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && polls.Add(1) < 3 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()
		if err := fast(New(srv.URL, time.Minute)).Rotate(context.Background()); err != nil || polls.Load() != 3 {
			t.Fatalf("err=%v polls=%d, want nil and 3", err, polls.Load())
		}
	})

	t.Run("insists on bringing the tunnel back up", func(t *testing.T) {
		var running atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPut {
				var c statusCall
				_ = json.NewDecoder(r.Body).Decode(&c)
				if c.Status == "running" && running.Add(1) < 3 {
					w.WriteHeader(http.StatusBadGateway)
					return
				}
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()
		if err := fast(New(srv.URL, time.Minute)).Rotate(context.Background()); err != nil || running.Load() != 3 {
			t.Fatalf("err=%v running attempts=%d, want nil and 3", err, running.Load())
		}
	})

	t.Run("a cancelled caller does not leave the tunnel stopped", func(t *testing.T) {
		srv, calls := fakeGluetun(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := fast(New(srv.URL, time.Minute)).Rotate(ctx); err != nil {
			t.Fatalf("Rotate with a cancelled ctx: %v", err)
		}
		if got := *calls; len(got) != 2 || got[1] != "running" {
			t.Fatalf("calls = %v, want the tunnel back running", got)
		}
	})

	t.Run("control error is returned and only a short pause follows", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()
		now := time.Now()
		r := fast(New(srv.URL, 10*time.Minute))
		r.now = func() time.Time { return now }
		if err := r.Rotate(context.Background()); err == nil {
			t.Fatal("expected an error")
		}
		if err := r.Rotate(context.Background()); err != ErrCooldown {
			t.Fatalf("right after a failure err = %v, want ErrCooldown", err)
		}
		now = now.Add(time.Minute) // well under minGap: only the failure pause applies
		if err := r.Rotate(context.Background()); err == nil || err == ErrCooldown {
			t.Fatalf("after the failure pause err = %v, want a new attempt", err)
		}
	})
}
