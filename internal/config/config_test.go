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

func TestLibraryConfigDefaults(t *testing.T) {
	for _, k := range []string{"LIBRARY_ENABLED", "LIBRARY_POOL_DIR", "LIBRARY_INDEX_DIR", "LIBRARY_RESERVE_PERCENT", "LIBRARY_RESERVE_BYTES",
		"LIBRARY_STALL_TIMEOUT", "LIBRARY_ENCODE_PRESET", "LIBRARY_ENCODE_CRF", "LIBRARY_ENCODE_THREADS", "LIBRARY_ENCODE_WINDOW", "LIBRARY_ENCODE_PAUSE_STREAMS"} {
		t.Setenv(k, "")
	}
	c := Load()
	if !c.LibraryEnabled || c.LibraryPoolDir != "/app/library-pool" || c.LibraryIndexDir != "/app/library-index" ||
		c.LibraryReservePercent != 10 || c.LibraryReserveBytes != 50_000_000_000 || c.LibraryStallTimeout != 24*time.Hour ||
		c.LibraryEncodePreset != 8 || c.LibraryEncodeCRF != 30 || c.LibraryEncodeThreads != 8 ||
		c.LibraryEncodeWindow != "" || c.LibraryEncodePauseStreams != 3 {
		t.Fatalf("defaults: %+v", c)
	}
	t.Setenv("LIBRARY_ENABLED", "false")
	t.Setenv("LIBRARY_RESERVE_BYTES", "123")
	t.Setenv("LIBRARY_ENCODE_WINDOW", "01:00-06:00")
	c = Load()
	if c.LibraryEnabled || c.LibraryReserveBytes != 123 || c.LibraryEncodeWindow != "01:00-06:00" {
		t.Fatalf("overrides: %+v", c)
	}
}
