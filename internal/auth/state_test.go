package auth

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gazes/gazes/internal/kv"
	"github.com/redis/go-redis/v9"
)

// twoInstances builds two services of the same stack (same keys and accounts volume) on one Redis.
func twoInstances(t *testing.T, mr *miniredis.Miniredis) (*Service, *Service) {
	t.Helper()
	dir := t.TempDir()
	state := NewRedisState(kv.New(redis.NewClient(&redis.Options{Addr: mr.Addr()}), "test"))
	make1 := func() *Service {
		svc, err := New(Options{Dir: dir, Getenv: func(string) string { return "" }, State: state})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { svc.Close() })
		return svc
	}
	return make1(), make1()
}

func TestSharedNonceIsSingleUseAcrossInstances(t *testing.T) {
	mr := miniredis.RunT(t)
	a, b := twoInstances(t, mr)
	env := seal(t, a.kemInfo(t), "login", map[string]string{"email": "a@b.co"})
	if _, err := b.kem.Open(env, "login"); err != nil {
		t.Fatalf("a nonce issued by A must open on B: %v", err)
	}
	if _, err := a.kem.Open(env, "login"); err == nil {
		t.Fatal("the same nonce must not open again on A")
	}
	if _, err := b.kem.Open(env, "login"); err == nil {
		t.Fatal("the same nonce must not open again on B")
	}
}

func TestSharedCaptchaReplayIsRejectedAcrossInstances(t *testing.T) {
	mr := miniredis.RunT(t)
	a, b := twoInstances(t, mr)
	rr := httptest.NewRecorder()
	a.CaptchaChallenge(rr, httptest.NewRequest("GET", "/auth/captcha", nil))
	var c Challenge
	_ = json.Unmarshal(rr.Body.Bytes(), &c)
	payload := solve(t, c)
	if !a.captcha.Verify(payload) {
		t.Fatal("valid solution rejected")
	}
	if b.captcha.Verify(payload) {
		t.Fatal("a solution replayed on another instance must be rejected")
	}
}

func TestSharedRateLimitCountsAcrossInstances(t *testing.T) {
	mr := miniredis.RunT(t)
	a, b := twoInstances(t, mr)
	if !a.limits.hit("login|x", 3, time.Minute) || !b.limits.hit("login|x", 3, time.Minute) || !a.limits.hit("login|x", 3, time.Minute) {
		t.Fatal("first three attempts across two instances are within the limit")
	}
	if b.limits.hit("login|x", 3, time.Minute) {
		t.Fatal("the fourth attempt must be refused whichever instance serves it")
	}
	a.limits.reset("login|x")
	if !b.limits.hit("login|x", 3, time.Minute) {
		t.Fatal("reset must clear the shared counter")
	}
}

func TestSharedStateFailsClosedWhenRedisIsDown(t *testing.T) {
	mr := miniredis.RunT(t)
	a, _ := twoInstances(t, mr)
	env := seal(t, a.kemInfo(t), "login", map[string]string{})
	rr := httptest.NewRecorder()
	a.CaptchaChallenge(rr, httptest.NewRequest("GET", "/auth/captcha", nil))
	var c Challenge
	_ = json.Unmarshal(rr.Body.Bytes(), &c)
	payload := solve(t, c)

	mr.Close()
	if _, err := a.kem.IssueChecked(); err == nil {
		t.Fatal("no nonce may be issued without shared state")
	}
	if _, err := a.kem.Open(env, "login"); err == nil {
		t.Fatal("an envelope must not open when the nonce store is unreachable")
	}
	if a.captcha.Verify(payload) {
		t.Fatal("captcha must be refused when replay protection is unreachable")
	}
	if a.limits.hit("k", 100, time.Minute) {
		t.Fatal("rate limiter must deny when its store is unreachable")
	}
	rr = httptest.NewRecorder()
	a.Kem(rr, httptest.NewRequest("GET", "/auth/kem", nil))
	if rr.Code != 503 {
		t.Fatalf("/auth/kem must answer 503, got %d", rr.Code)
	}
}
