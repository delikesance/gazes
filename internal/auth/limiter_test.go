package auth

import (
	"strconv"
	"testing"
	"time"
)

func TestLimiterFloodKeepsActiveCounters(t *testing.T) {
	l := newLimiter()
	for range 3 {
		l.hit("login|victim", 3, time.Hour)
	}
	// A flood of fresh keys fills the table; the counter that is throttling someone must survive.
	for i := range 60000 {
		l.hit("flood|"+strconv.Itoa(i), 3, time.Hour)
	}
	if l.hit("login|victim", 3, time.Hour) {
		t.Fatal("flooding the limiter reset an exhausted counter")
	}
	if len(l.entries) > 50001 {
		t.Fatalf("table not shed: %d entries", len(l.entries))
	}
}
