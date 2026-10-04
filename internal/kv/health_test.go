package kv

import (
	"context"
	"testing"
	"time"
)

// healthGovernors runs a scenario against the in-process fallback and against Redis (miniredis),
// because both implementations must behave identically.
func healthGovernors(t *testing.T, policy HealthPolicy) map[string]func() *Governor {
	t.Helper()
	c, _ := newClient(t)
	return map[string]func() *Governor{
		"local": func() *Governor {
			g := NewLocalGovernor("p")
			g.SetHealthPolicy(policy)
			return g
		},
		"redis": func() *Governor {
			g := c.NewGovernor("p", 600, 600, 0)
			g.SetHealthPolicy(policy)
			return g
		},
	}
}

var testPolicy = HealthPolicy{Threshold: 3, Window: 400 * time.Millisecond, Base: 100 * time.Millisecond, Max: 400 * time.Millisecond, ProbeTimeout: 300 * time.Millisecond}

func TestHealthGenericFailuresNeedThresholdInWindow(t *testing.T) {
	for name, mk := range healthGovernors(t, testPolicy) {
		t.Run(name, func(t *testing.T) {
			g, ctx := mk(), context.Background()
			if d := g.Failure(ctx, false, 0); d != 0 {
				t.Fatalf("first generic failure must not open: %s", d)
			}
			if d := g.Failure(ctx, false, 0); d != 0 {
				t.Fatalf("second generic failure must not open: %s", d)
			}
			if a := g.Admit(ctx); !a.Allowed {
				t.Fatal("still closed after two failures")
			}
			if d := g.Failure(ctx, false, 0); d != testPolicy.Base {
				t.Fatalf("third failure opens with the base backoff: %s", d)
			}
			if a := g.Admit(ctx); a.Allowed || a.Remaining <= 0 {
				t.Fatalf("open circuit must refuse: %+v", a)
			}
		})
	}
}

func TestHealthFailuresOutsideWindowDoNotAccumulate(t *testing.T) {
	p := testPolicy
	p.Window = 60 * time.Millisecond
	for name, mk := range healthGovernors(t, p) {
		t.Run(name, func(t *testing.T) {
			g, ctx := mk(), context.Background()
			for i := 0; i < 5; i++ {
				if d := g.Failure(ctx, false, 0); d != 0 {
					t.Fatalf("failure %d spread over time must not open: %s", i, d)
				}
				time.Sleep(90 * time.Millisecond)
			}
		})
	}
}

func TestHealthImmediateFailureOpensAndHonoursRetryAfter(t *testing.T) {
	for name, mk := range healthGovernors(t, testPolicy) {
		t.Run(name, func(t *testing.T) {
			g, ctx := mk(), context.Background()
			if d := g.Failure(ctx, true, 0); d != testPolicy.Base {
				t.Fatalf("immediate failure: %s", d)
			}
			g.Success(ctx)
			if d := g.Failure(ctx, true, 700*time.Millisecond); d != 700*time.Millisecond {
				t.Fatalf("Retry-After must be honoured: %s", d)
			}
			if rem := g.Cooldown(ctx); rem < 500*time.Millisecond {
				t.Fatalf("cooldown not applied: %s", rem)
			}
		})
	}
}

func TestHealthBackoffGrowsCapsAndSuccessResets(t *testing.T) {
	for name, mk := range healthGovernors(t, testPolicy) {
		t.Run(name, func(t *testing.T) {
			g, ctx := mk(), context.Background()
			want := []time.Duration{100, 200, 400, 400, 400}
			for i, w := range want {
				got := g.Failure(ctx, true, 0)
				if got != w*time.Millisecond {
					t.Fatalf("step %d: %s, want %s", i, got, w*time.Millisecond)
				}
				time.Sleep(got + 20*time.Millisecond) // let the cooldown expire; the next call is the probe
				if a := g.Admit(ctx); !a.Allowed || !a.Probe {
					t.Fatalf("step %d: expected a half-open probe: %+v", i, a)
				}
			}
			g.Success(ctx)
			if a := g.Admit(ctx); !a.Allowed || a.Probe {
				t.Fatalf("success must close the circuit: %+v", a)
			}
			if d := g.Failure(ctx, true, 0); d != testPolicy.Base {
				t.Fatalf("success must reset the backoff: %s", d)
			}
		})
	}
}

func TestHealthHalfOpenAllowsExactlyOneProbe(t *testing.T) {
	for name, mk := range healthGovernors(t, testPolicy) {
		t.Run(name, func(t *testing.T) {
			g, ctx := mk(), context.Background()
			g.Failure(ctx, true, 0)
			time.Sleep(130 * time.Millisecond)
			first := g.Admit(ctx)
			if !first.Allowed || !first.Probe {
				t.Fatalf("first caller after the cooldown is the probe: %+v", first)
			}
			if second := g.Admit(ctx); second.Allowed {
				t.Fatalf("only one probe at a time: %+v", second)
			}
			// A probe that ends neutrally (our own timeout) frees the slot for another.
			g.Neutral(ctx, true)
			if again := g.Admit(ctx); !again.Allowed || !again.Probe {
				t.Fatalf("neutral outcome must release the probe: %+v", again)
			}
			// A failed probe re-opens with the next step.
			if d := g.Failure(ctx, false, 0); d != 2*testPolicy.Base {
				t.Fatalf("failed probe must open at the next backoff step: %s", d)
			}
		})
	}
}

func TestHealthFailureWhileAlreadyOpenDoesNotEscalate(t *testing.T) {
	for name, mk := range healthGovernors(t, testPolicy) {
		t.Run(name, func(t *testing.T) {
			g, ctx := mk(), context.Background()
			g.Failure(ctx, true, 0)
			g.Failure(ctx, true, 0) // an in-flight call that started before the circuit opened
			g.Failure(ctx, true, 0)
			time.Sleep(130 * time.Millisecond)
			if d := g.Failure(ctx, true, 0); d != 2*testPolicy.Base {
				t.Fatalf("late failures must not stack backoff steps: %s", d)
			}
		})
	}
}

func TestHealthIsSharedAcrossInstances(t *testing.T) {
	c, _ := newClient(t)
	a, b := c.NewGovernor("shared", 600, 600, 0), c.NewGovernor("shared", 600, 600, 0)
	a.Failure(context.Background(), true, 0)
	if adm := b.Admit(context.Background()); adm.Allowed {
		t.Fatalf("another instance must see the open circuit: %+v", adm)
	}
}

func TestHealthStaleNeutralDoesNotFreeTheProbe(t *testing.T) {
	for name, mk := range healthGovernors(t, testPolicy) {
		t.Run(name, func(t *testing.T) {
			g, ctx := mk(), context.Background()
			g.Failure(ctx, true, 0)
			time.Sleep(130 * time.Millisecond)
			if a := g.Admit(ctx); !a.Probe {
				t.Fatalf("expected the probe: %+v", a)
			}
			g.Neutral(ctx, false) // a call admitted before the circuit opened ends neutrally
			if a := g.Admit(ctx); a.Allowed {
				t.Fatalf("a stale neutral admitted a second probe: %+v", a)
			}
			g.Neutral(ctx, true)
			if a := g.Admit(ctx); !a.Allowed || !a.Probe {
				t.Fatalf("the probe holder's neutral must free the slot: %+v", a)
			}
		})
	}
}
