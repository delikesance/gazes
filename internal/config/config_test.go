package config

import (
	"testing"
	"time"
)

func TestResolverFastPhaseTimeout(t *testing.T) {
	t.Setenv("RESOLVER_FAST_PHASE_TIMEOUT", "")
	if got := Load().ResolverFastPhaseTimeout; got != 3*time.Second {
		t.Fatalf("default = %v, want 3s", got)
	}
	t.Setenv("RESOLVER_FAST_PHASE_TIMEOUT", "1500ms")
	if got := Load().ResolverFastPhaseTimeout; got != 1500*time.Millisecond {
		t.Fatalf("override = %v", got)
	}
	t.Setenv("RESOLVER_FAST_PHASE_TIMEOUT", "bogus")
	if got := Load().ResolverFastPhaseTimeout; got != 3*time.Second {
		t.Fatalf("invalid value must keep default, got %v", got)
	}
}
