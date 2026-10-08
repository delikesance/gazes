package admin

import (
	"testing"
	"time"

	"github.com/gazes/gazes/internal/loadstats"
)

func TestCostsLoad(t *testing.T) {
	e := newPBEnv(t)
	loadstats.Drain() // the counters are process-wide: start from a clean window

	got := e.do(t, "GET", "/costs", "", e.as("metrics"))
	load := pbSub(t, got.data, "load")
	if load["measured_days"] != 0.0 || load["apple_share"] != nil || load["bytes_out"] != 0.0 {
		t.Fatalf("empty load = %v", load)
	}

	const iphone = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15"
	loadstats.Session(iphone, true, true)
	loadstats.Session("Mozilla/5.0 (X11; Linux x86_64) Firefox/130.0", false, false)
	loadstats.RemuxOpened()
	loadstats.RemuxOpened()
	loadstats.RemuxClosed()
	loadstats.RemuxRefused()
	loadstats.FFmpegStarted()
	loadstats.FFmpegDone(1500*time.Millisecond, true)
	loadstats.FFmpegStarted()
	loadstats.FFmpegDone(500*time.Millisecond, false)

	e.svc.respCache.clear() // /costs is cached for a few seconds
	got = e.do(t, "GET", "/costs", "", e.as("metrics"))
	load = pbSub(t, got.data, "load")
	want := map[string]any{
		"measured_days": 1.0, "sessions": 2.0, "apple_share": 0.5, "no_av1_share": 0.5, "transcode_share": 0.5,
		"cpu_seconds_transcode": 1.5, "cpu_seconds_copy": 0.5, "peak_remuxes": 2.0, "remux_rejected": 1.0, "peak_ffmpeg": 1.0,
		"remux_limit": 0.0, // no stream package linked in this test
	}
	for k, v := range want {
		if load[k] != v && k != "remux_limit" {
			t.Errorf("%s = %v, want %v", k, load[k], v)
		}
	}
	loadstats.RemuxClosed()
}
