package api

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func partyRouter(h *partyHub) http.Handler {
	r := chi.NewRouter()
	r.Get("/party/{room}/events", h.handleEvents)
	r.Post("/party/{room}/events", h.handlePost)
	return r
}

func TestPartyBroadcastsToOtherMembersOnly(t *testing.T) {
	srv := httptest.NewServer(partyRouter(newPartyHub()))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/party/abc123/events?from=alice", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content type %q", ct)
	}
	lines := make(chan string, 8)
	go func() {
		sc := bufio.NewScanner(res.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
	}()
	waitFor := func(substr string) bool {
		deadline := time.After(2 * time.Second)
		for {
			select {
			case l := <-lines:
				if strings.Contains(l, substr) {
					return true
				}
			case <-deadline:
				return false
			}
		}
	}
	if !waitFor("ready") {
		t.Fatal("no ready event")
	}
	post := func(body string) int {
		rr, err := http.Post(srv.URL+"/party/abc123/events", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer rr.Body.Close()
		return rr.StatusCode
	}
	// Alice's own message must not echo back; Bob's must arrive.
	if c := post(`{"from":"alice","type":"pause","t":12.5}`); c != http.StatusNoContent {
		t.Fatalf("post status %d", c)
	}
	if c := post(`{"from":"bob","type":"seek","t":42}`); c != http.StatusNoContent {
		t.Fatalf("post status %d", c)
	}
	if !waitFor(`"from":"bob"`) {
		t.Fatal("bob's event not delivered")
	}
}

func TestPartyRejectsBadInput(t *testing.T) {
	srv := httptest.NewServer(partyRouter(newPartyHub()))
	defer srv.Close()
	for _, tc := range []struct{ path, body string }{
		{"/party/x/events", `{"from":"a","type":"play","t":1}`},
		{"/party/abc123/events", `{"from":"a","type":"explode","t":1}`},
		{"/party/abc123/events", `{"from":"","type":"play","t":1}`},
		{"/party/abc123/events", `{"from":"a","type":"play","t":-5}`},
	} {
		res, err := http.Post(srv.URL+tc.path, "application/json", strings.NewReader(tc.body))
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s %s: got %d", tc.path, tc.body, res.StatusCode)
		}
	}
}

func TestPartySendsHeartbeat(t *testing.T) {
	old := partyHeartbeat
	partyHeartbeat = 20 * time.Millisecond
	defer func() { partyHeartbeat = old }()
	srv := httptest.NewServer(partyRouter(newPartyHub()))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/party/abc123/events?from=alice", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	sc := bufio.NewScanner(res.Body)
	for sc.Scan() {
		// An SSE comment keeps idle proxies (Next's 30 s proxy timeout) from closing a quiet room.
		if strings.HasPrefix(sc.Text(), ":") {
			return
		}
	}
	t.Fatal("no heartbeat on an idle party stream")
}

func TestPartyCapsStreamsPerClient(t *testing.T) {
	h := newPartyHub()
	var held []*partyMember
	for i := range maxPartyStreamsPerIP {
		m := h.join("room"+strconv.Itoa(i)+"aaa", "a", "203.0.113.7")
		if m == nil {
			t.Fatalf("stream %d refused under the cap", i)
		}
		held = append(held, m)
	}
	// One client must not be able to fill every room and lock everyone else out.
	if h.join("otherroom", "a", "203.0.113.7") != nil {
		t.Fatal("client exceeded its stream cap")
	}
	if h.join("otherroom", "b", "203.0.113.8") == nil {
		t.Fatal("another client was refused")
	}
	h.leave("room0aaa", held[0])
	if h.join("otherroom", "a", "203.0.113.7") == nil {
		t.Fatal("a closed stream did not free its slot")
	}
}

func TestPartyStreamCapCoversIPv6Prefix(t *testing.T) {
	h := newPartyHub()
	srv := httptest.NewServer(partyRouter(h))
	defer srv.Close()
	// Rotating addresses inside one /64 is still one client.
	h.clientIP = func(r *http.Request) string { return clientBucket(r.Header.Get("X-Test-IP")) }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	open := func(i int) int {
		req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/party/room"+strconv.Itoa(i)+"aa/events?from=a", nil)
		req.Header.Set("X-Test-IP", "2001:db8::"+strconv.Itoa(i+1))
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != http.StatusOK {
			res.Body.Close()
		}
		return res.StatusCode
	}
	for i := range maxPartyStreamsPerIP {
		if c := open(i); c != http.StatusOK {
			t.Fatalf("stream %d refused under the cap: %d", i, c)
		}
	}
	if c := open(maxPartyStreamsPerIP); c != http.StatusServiceUnavailable {
		t.Fatalf("a new address in the same /64 got past the cap: %d", c)
	}
}

func TestPartyCatchesUpLateJoiner(t *testing.T) {
	h := newPartyHub()
	first := h.join("abc123", "alice", "1.1.1.1")
	if _, ok := h.snapshot("abc123"); ok {
		t.Fatal("a room nobody acted in has no state to replay")
	}
	h.publish("abc123", partyEvent{From: "alice", Type: "play", T: 10}, []byte(`{}`))
	h.publish("abc123", partyEvent{From: "alice", Type: "seek", T: 42}, []byte(`{}`))
	ev, ok := h.snapshot("abc123")
	if !ok || ev.Type != "play" || ev.T < 42 || ev.T > 43 {
		t.Fatalf("late joiner state %+v %v, want playing from 42 s", ev, ok)
	}
	h.publish("abc123", partyEvent{From: "alice", Type: "pause", T: 50}, []byte(`{}`))
	if ev, _ := h.snapshot("abc123"); ev.Type != "pause" || ev.T != 50 {
		t.Fatalf("late joiner state %+v, want paused at 50 s", ev)
	}
	h.leave("abc123", first)
	h.publish("gone12", partyEvent{From: "x", Type: "play", T: 1}, []byte(`{}`))
	if len(h.state) != 0 {
		t.Fatalf("state kept for empty rooms: %v", h.state)
	}
}
