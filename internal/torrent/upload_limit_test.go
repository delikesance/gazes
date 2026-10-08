package torrent

import (
	"math"
	"testing"

	"golang.org/x/time/rate"
)

func TestUploadLimiter(t *testing.T) {
	if l := uploadLimiter(0); l.Limit() != rate.Inf {
		t.Fatalf("0 must be unlimited, got %v", l.Limit())
	}
	if l := uploadLimiter(-5); l.Limit() != rate.Inf {
		t.Fatalf("negative must be unlimited, got %v", l.Limit())
	}
	l := uploadLimiter(100 << 10)
	if l.Limit() != rate.Limit(100<<10) || l.Burst() < 1<<20 {
		t.Fatalf("small cap: limit=%v burst=%d, burst must cover a chunk", l.Limit(), l.Burst())
	}
	if l := uploadLimiter(8 << 20); l.Burst() != 8<<20 {
		t.Fatalf("burst = %d", l.Burst())
	}
	if d := DefaultEngineConfig("x"); d.UploadBytesPerSec <= 0 || float64(d.UploadBytesPerSec) > math.MaxInt32 {
		t.Fatalf("default cap should be set, got %d", d.UploadBytesPerSec)
	}
}
