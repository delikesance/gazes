package indexer

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gazes/gazes/internal/kv"
	"github.com/redis/go-redis/v9"
)

// pacedFake stands in for a Prowlarr indexer with a request delay: it records how many calls
// overlap, when each one started and how much of the call deadline was left.
type pacedFake struct {
	name     string
	pace     Pacing
	work     time.Duration
	failWith func(call int) error
	empty    bool

	mu        sync.Mutex
	starts    []time.Time
	budgets   []time.Duration
	inflight  int
	maxFlight int
	calls     atomic.Int32
}

func (p *pacedFake) Name() string { return p.name }
func (p *pacedFake) Pacing() Pacing {
	return p.pace
}
func (p *pacedFake) Search(ctx context.Context, o SearchOptions) ([]TorrentItem, error) {
	n := int(p.calls.Add(1))
	p.mu.Lock()
	p.starts = append(p.starts, time.Now())
	if dl, ok := ctx.Deadline(); ok {
		p.budgets = append(p.budgets, time.Until(dl))
	}
	p.inflight++
	p.maxFlight = max(p.maxFlight, p.inflight)
	p.mu.Unlock()
	defer func() { p.mu.Lock(); p.inflight--; p.mu.Unlock() }()
	select {
	case <-time.After(p.work):
	case <-ctx.Done():
		return nil, fmt.Errorf("%s: request interrupted: %w", p.name, ctx.Err())
	}
	if p.failWith != nil {
		if err := p.failWith(n); err != nil {
			return nil, err
		}
	}
	if p.empty {
		return nil, nil
	}
	return []TorrentItem{{InfoHash: fmt.Sprintf("%040d", n), Title: o.Query, Seeders: 1}}, nil
}
func (p *pacedFake) GetLatest(ctx context.Context, _ string, _ int) ([]TorrentItem, error) {
	return p.Search(ctx, SearchOptions{})
}

// each variant runs on the in-process breaker and on the Redis-shared path.
func variants(t *testing.T, providers ...Provider) map[string]*MultiProvider {
	t.Helper()
	local := NewMultiProvider(providers...)
	shared := NewMultiProvider(providers...)
	shared.SetRedis(kv.New(redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()}), "test"))
	return map[string]*MultiProvider{"local": local, "shared": shared}
}

func burst(m *MultiProvider, n int, ctx context.Context) []error {
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = m.Search(ctx, SearchOptions{Query: fmt.Sprintf("query %d", i)})
		}(i)
	}
	wg.Wait()
	return errs
}

func TestPacedProviderBurstIsSerializedAndSpaced(t *testing.T) {
	const interval = 60 * time.Millisecond
	for name := range variants(t, &pacedFake{name: "x"}) {
		t.Run(name, func(t *testing.T) {
			p := &pacedFake{name: "c411", pace: Pacing{MaxConcurrent: 1, MinInterval: interval}, work: 10 * time.Millisecond}
			m := variants(t, p)[name]
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			for i, err := range burst(m, 4, ctx) {
				if err != nil {
					t.Fatalf("query %d failed: %v", i, err)
				}
			}
			if p.calls.Load() != 4 || p.maxFlight != 1 {
				t.Fatalf("calls=%d max concurrency=%d", p.calls.Load(), p.maxFlight)
			}
			for i := 1; i < len(p.starts); i++ {
				if gap := p.starts[i].Sub(p.starts[i-1]); gap < interval-5*time.Millisecond {
					t.Fatalf("calls %d and %d started %s apart, want >= %s", i-1, i, gap, interval)
				}
			}
			if cd := m.states[0].gov.Cooldown(context.Background()); cd != 0 {
				t.Fatalf("a paced burst must not trip the cooldown: %s", cd)
			}
		})
	}
}

func TestPacedCallTimeoutStartsAfterTheSlot(t *testing.T) {
	p := &pacedFake{name: "c411", pace: Pacing{MinInterval: 80 * time.Millisecond}}
	m := NewMultiProvider(p)
	m.states[0].callTimeout = 2 * time.Second
	burst(m, 3, context.Background())
	for i, b := range p.budgets {
		if b < 1900*time.Millisecond {
			t.Fatalf("call %d only had %s of its 2s timeout: queue time was charged to it", i, b)
		}
	}
}

func TestPacedDetection(t *testing.T) {
	m := NewMultiProvider(&pacedFake{name: "c411", pace: Pacing{MaxConcurrent: 1, MinInterval: time.Second}}, &pacedFake{name: "anidex"})
	if !m.Paced("c411") || m.Paced("anidex") || m.Paced("unknown") {
		t.Fatal("Paced must be true only for providers declaring pacing")
	}
	if got := m.Pacing("c411"); got.MinInterval != time.Second {
		t.Fatalf("pacing: %+v", got)
	}
	if cap(m.states[0].slots) != 1 || cap(m.states[1].slots) != 2 {
		t.Fatalf("slots: paced=%d unpaced=%d", cap(m.states[0].slots), cap(m.states[1].slots))
	}
}

func TestOwnTimeoutAndEmptyResultsDoNotPenalize(t *testing.T) {
	for name := range variants(t, &pacedFake{name: "x"}) {
		t.Run(name, func(t *testing.T) {
			slow := &pacedFake{name: "slow", work: time.Minute, pace: Pacing{MinInterval: time.Millisecond}}
			empty := &pacedFake{name: "empty", empty: true}
			ms := variants(t, slow, empty)[name]
			ms.states[0].callTimeout = 30 * time.Millisecond
			for i := 0; i < 5; i++ { // far more than the generic-failure threshold
				_, _ = ms.Search(context.Background(), SearchOptions{Query: fmt.Sprint("q", i)})
			}
			for _, s := range ms.states {
				if cd := s.gov.Cooldown(context.Background()); cd != 0 {
					t.Fatalf("%s: own timeout / zero results opened a cooldown of %s", s.provider.Name(), cd)
				}
			}
			if slow.calls.Load() != 5 {
				t.Fatalf("slow provider was refused after its own timeouts: %d calls", slow.calls.Load())
			}
		})
	}
}

func TestCooldownPolicyByErrorKind(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		wantMin time.Duration
		wantMax time.Duration
	}{
		{"429 without Retry-After", &HTTPError{Provider: "p", Status: 429}, 25 * time.Second, 31 * time.Second},
		{"429 with Retry-After", &HTTPError{Provider: "p", Status: 429, RetryAfter: 90 * time.Second}, 85 * time.Second, 91 * time.Second},
		{"503", &HTTPError{Provider: "p", Status: 503}, 25 * time.Second, 31 * time.Second},
		{"401", &HTTPError{Provider: "p", Status: 401}, 25 * time.Second, 31 * time.Second},
		{"403", &HTTPError{Provider: "p", Status: 403}, 25 * time.Second, 31 * time.Second},
	}
	for _, tc := range cases {
		for variant := range variants(t, &pacedFake{name: "x"}) {
			t.Run(variant+"/"+tc.name, func(t *testing.T) {
				p := &pacedFake{name: "p", failWith: func(int) error { return tc.err }}
				m := variants(t, p)[variant]
				_, _ = m.Search(context.Background(), SearchOptions{Query: "a"})
				cd := m.states[0].gov.Cooldown(context.Background())
				if cd < tc.wantMin || cd > tc.wantMax {
					t.Fatalf("cooldown %s, want %s..%s", cd, tc.wantMin, tc.wantMax)
				}
				before := p.calls.Load()
				if _, err := m.Search(context.Background(), SearchOptions{Query: "b"}); err == nil || p.calls.Load() != before {
					t.Fatalf("provider must not be called while cooling down (err=%v)", err)
				}
			})
		}
	}
}

func TestOtherClientErrorsAndNetworkErrorsNeedThreeFailures(t *testing.T) {
	for _, failure := range []error{errors.New("connection reset"), &HTTPError{Provider: "p", Status: http.StatusNotFound}} {
		for variant := range variants(t, &pacedFake{name: "x"}) {
			t.Run(variant+"/"+failure.Error(), func(t *testing.T) {
				p := &pacedFake{name: "p", failWith: func(int) error { return failure }}
				m := variants(t, p)[variant]
				for i := 0; i < 2; i++ {
					_, _ = m.Search(context.Background(), SearchOptions{Query: fmt.Sprint("q", i)})
				}
				if cd := m.states[0].gov.Cooldown(context.Background()); cd != 0 {
					t.Fatalf("two failures opened the cooldown: %s", cd)
				}
				_, _ = m.Search(context.Background(), SearchOptions{Query: "q2"})
				if cd := m.states[0].gov.Cooldown(context.Background()); cd < 25*time.Second {
					t.Fatalf("third failure must open the cooldown, got %s", cd)
				}
			})
		}
	}
}

func TestHalfOpenLetsExactlyOneProbeThrough(t *testing.T) {
	for variant := range variants(t, &pacedFake{name: "x"}) {
		t.Run(variant, func(t *testing.T) {
			p := &pacedFake{name: "p", work: 150 * time.Millisecond, failWith: func(n int) error {
				if n == 1 {
					return &HTTPError{Provider: "p", Status: 503}
				}
				return nil
			}}
			m := variants(t, p)[variant]
			m.states[0].gov.SetHealthPolicy(kv.HealthPolicy{Threshold: 3, Window: time.Second, Base: 100 * time.Millisecond, Max: time.Second, ProbeTimeout: 2 * time.Second})
			_, _ = m.Search(context.Background(), SearchOptions{Query: "first"}) // opens the circuit
			time.Sleep(130 * time.Millisecond)
			errs := burst(m, 4, context.Background())
			ok := 0
			for _, err := range errs {
				if err == nil {
					ok++
				}
			}
			if calls := p.calls.Load(); calls != 2 || ok != 1 { // the failing call + exactly one probe
				t.Fatalf("calls=%d successes=%d errs=%v", calls, ok, errs)
			}
			if _, err := m.Search(context.Background(), SearchOptions{Query: "after"}); err != nil {
				t.Fatalf("a successful probe must close the circuit: %v", err)
			}
		})
	}
}

func TestCooldownOfOneProviderDoesNotBlockAnother(t *testing.T) {
	for variant := range variants(t, &pacedFake{name: "x"}) {
		t.Run(variant, func(t *testing.T) {
			bad := &pacedFake{name: "bad", failWith: func(int) error { return &HTTPError{Provider: "bad", Status: 429} }}
			good := &pacedFake{name: "good"}
			m := variants(t, bad, good)[variant]
			for i := 0; i < 3; i++ {
				items, err := m.Search(context.Background(), SearchOptions{Query: fmt.Sprint("q", i)})
				var partial *PartialError
				if len(items) != 1 || !errors.As(err, &partial) || partial.AllFailed {
					t.Fatalf("query %d: items=%v err=%v", i, items, err)
				}
			}
			if good.calls.Load() != 3 || bad.calls.Load() != 1 {
				t.Fatalf("good=%d bad=%d", good.calls.Load(), bad.calls.Load())
			}
		})
	}
}

func TestCancelledCallerDoesNotConsumePacingSlot(t *testing.T) {
	const interval = 250 * time.Millisecond
	p := &pacedFake{name: "c411", pace: Pacing{MaxConcurrent: 1, MinInterval: interval}}
	m := NewMultiProvider(p)
	if _, err := m.Search(context.Background(), SearchOptions{Query: "first"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	if _, err := m.Search(ctx, SearchOptions{Query: "gave up"}); err == nil {
		t.Fatal("the caller must give up while waiting for the pacing slot")
	}
	if _, err := m.Search(context.Background(), SearchOptions{Query: "third"}); err != nil {
		t.Fatal(err)
	}
	if p.calls.Load() != 2 {
		t.Fatalf("the cancelled caller reached the provider: %d calls", p.calls.Load())
	}
	if gap := p.starts[1].Sub(p.starts[0]); gap > interval+80*time.Millisecond {
		t.Fatalf("the cancelled caller delayed the next one: %s", gap)
	}
	if len(m.states[0].slots) != 0 {
		t.Fatal("the pacing slot leaked")
	}
	if cd := m.states[0].gov.Cooldown(context.Background()); cd != 0 {
		t.Fatalf("a cancelled caller opened a cooldown: %s", cd)
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		in   string
		want time.Duration
	}{
		{"120", 2 * time.Minute}, {"", 0}, {"-5", 0}, {"junk", 0},
		{now.Add(45 * time.Second).Format(http.TimeFormat), 45 * time.Second},
		{now.Add(-time.Minute).Format(http.TimeFormat), 0},
	} {
		if got := ParseRetryAfter(tc.in, now); got != tc.want {
			t.Errorf("ParseRetryAfter(%q) = %s, want %s", tc.in, got, tc.want)
		}
	}
}

func TestAbandonedPacedCallerIsNotSentLater(t *testing.T) {
	const interval = 250 * time.Millisecond
	for variant := range variants(t, &pacedFake{name: "x"}) {
		t.Run(variant, func(t *testing.T) {
			p := &pacedFake{name: "c411", pace: Pacing{MaxConcurrent: 1, MinInterval: interval}}
			m := variants(t, p)[variant]
			if _, err := m.Search(context.Background(), SearchOptions{Query: "first"}); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
			defer cancel()
			if _, err := m.Search(ctx, SearchOptions{Query: "abandoned"}); err == nil {
				t.Fatal("caller must give up")
			}
			time.Sleep(2 * interval)
			if n := p.calls.Load(); n != 1 {
				t.Fatalf("the abandoned query was still sent: %d calls", n)
			}
			if len(m.states[0].slots) != 0 {
				t.Fatal("slot leaked")
			}
		})
	}
}

func TestPacedQueueWaitIsBounded(t *testing.T) {
	p := &pacedFake{name: "c411", pace: Pacing{MinInterval: time.Hour}}
	m := NewMultiProvider(p)
	m.states[0].maxQueueWait = 50 * time.Millisecond
	_, _ = m.Search(context.Background(), SearchOptions{Query: "first"})
	start := time.Now()
	if _, err := m.states[0].call(context.Background(), SearchOptions{Query: "second"}, false, start); err == nil || time.Since(start) > time.Second {
		t.Fatalf("a paced call must not wait longer than maxQueueWait: err=%v after %s", err, time.Since(start))
	}
}

func TestBackgroundRefreshSkipsBusyPacedProvider(t *testing.T) {
	p := &pacedFake{name: "c411", pace: Pacing{MaxConcurrent: 1, MinInterval: time.Hour}}
	m := NewMultiProvider(p)
	s := m.states[0]
	bg := kv.Background(context.Background())
	if _, err := s.call(bg, SearchOptions{Query: "idle"}, false, time.Now()); err != nil {
		t.Fatalf("an idle paced provider serves a background refresh: %v", err)
	}
	start := time.Now()
	if _, err := s.call(bg, SearchOptions{Query: "interval not elapsed"}, false, start); err == nil || time.Since(start) > 200*time.Millisecond {
		t.Fatalf("background refresh must not queue behind the pacing clock: %v after %s", err, time.Since(start))
	}
	s.nextStart = time.Time{}
	s.slots <- struct{}{}
	if _, err := s.call(bg, SearchOptions{Query: "slot busy"}, false, start); err == nil {
		t.Fatal("background refresh must not queue for a busy slot")
	}
	<-s.slots
	if p.calls.Load() != 1 {
		t.Fatalf("calls=%d", p.calls.Load())
	}
	if cd := s.gov.Cooldown(context.Background()); cd != 0 {
		t.Fatalf("skipping is not a failure: %s", cd)
	}
}

func TestUnpacedOwnTimeoutsCountSoftPacedAndCallerCancelDoNot(t *testing.T) {
	// A blackholing unpaced tracker opens the circuit after 3 timeouts in the window.
	slow := &pacedFake{name: "slow", work: time.Minute}
	m := NewMultiProvider(slow)
	m.states[0].callTimeout = 20 * time.Millisecond
	for i := 0; i < 2; i++ {
		_, _ = m.Search(context.Background(), SearchOptions{Query: fmt.Sprint("q", i)})
	}
	if cd := m.states[0].gov.Cooldown(context.Background()); cd != 0 {
		t.Fatalf("two timeouts must not open: %s", cd)
	}
	_, _ = m.Search(context.Background(), SearchOptions{Query: "q2"})
	if cd := m.states[0].gov.Cooldown(context.Background()); cd < 25*time.Second {
		t.Fatalf("third timeout of an unpaced provider must open the circuit: %s", cd)
	}
	// A paced provider's own timeouts stay neutral.
	paced := &pacedFake{name: "c411", pace: Pacing{MinInterval: time.Millisecond}, work: time.Minute}
	mp := NewMultiProvider(paced)
	mp.states[0].callTimeout = 20 * time.Millisecond
	for i := 0; i < 5; i++ {
		_, _ = mp.Search(context.Background(), SearchOptions{Query: fmt.Sprint("q", i)})
	}
	if cd := mp.states[0].gov.Cooldown(context.Background()); cd != 0 {
		t.Fatalf("paced timeouts must stay neutral: %s", cd)
	}
	// Caller cancellation is neutral for everybody.
	m2 := NewMultiProvider(slow)
	for i := 0; i < 5; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		_, _ = m2.states[0].call(ctx, SearchOptions{Query: fmt.Sprint("c", i)}, false, time.Now())
		cancel()
	}
	if cd := m2.states[0].gov.Cooldown(context.Background()); cd != 0 {
		t.Fatalf("caller cancellation must stay neutral: %s", cd)
	}
}

func TestRolledBackPacingClaimDoesNotBurnTheInterval(t *testing.T) {
	p := &pacedFake{name: "c411", pace: Pacing{MinInterval: time.Hour}}
	s := NewMultiProvider(p).states[0]
	claim, err := s.acquire(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	s.release(claim, false) // refused or cancelled before the request went out
	start := time.Now()
	claim, err = s.acquire(context.Background(), true)
	if err != nil || time.Since(start) > 100*time.Millisecond {
		t.Fatalf("the interval was burnt by a request that never went out: %v after %s", err, time.Since(start))
	}
	s.release(claim, true) // sent: the interval stays claimed
	if _, err := s.acquire(context.Background(), true); err == nil {
		t.Fatal("a request that was sent must keep its interval")
	}
}

func TestAuditProviderForwardsPacing(t *testing.T) {
	m := NewMultiProvider(&pacedFake{name: "c411", pace: Pacing{MaxConcurrent: 1, MinInterval: 4100 * time.Millisecond}})
	if budget, ok := pacedBudget(NewAuditProvider(m), 9*time.Second); !ok || budget != 2 {
		t.Fatalf("budget=%d ok=%v: the audit recorder must expose the pacing", budget, ok)
	}
}
