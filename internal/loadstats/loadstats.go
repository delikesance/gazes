// Package loadstats holds the cheap in-process counters behind the admin "Business" load figures:
// bytes served, CPU spent by ffmpeg, client mix and remux saturation. Everything is atomic and
// never blocks or fails a playback; the admin service drains the counters into a daily table.
package loadstats

import (
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// Delta is what was counted since the previous Drain. Peaks are maxima over that window.
type Delta struct {
	BytesOut          int64
	CPUCopyMS         int64 // user+system CPU of ffmpeg processes copying the video
	CPUTranscodeMS    int64 // same, for processes re-encoding the video to H.264
	Sessions          int64 // playback sessions created
	SessionsApple     int64 // ... from an iPhone/iPad/iPod user agent
	SessionsNoAV1     int64 // ... whose client declared it cannot decode AV1
	SessionsTranscode int64 // ... that actually need a video transcode
	RemuxRejected     int64 // /stream requests refused because every remux slot was taken
	PeakRemuxes       int64 // most simultaneous /stream remuxes
	PeakFFmpeg        int64 // most simultaneous ffmpeg processes (remux + HLS segments)
}

var (
	bytesOut, cpuCopy, cpuTranscode                  atomic.Int64
	sessions, apple, noAV1, transcode, remuxRejected atomic.Int64
	remuxes, peakRemuxes, ffmpeg, peakFFmpeg         atomic.Int64
	remuxLimit                                       atomic.Int64
)

func raise(peak *atomic.Int64, v int64) {
	for {
		p := peak.Load()
		if v <= p || peak.CompareAndSwap(p, v) {
			return
		}
	}
}

// SetRemuxLimit records the global cap of concurrent remuxes (shown as "in use / limit").
func SetRemuxLimit(n int) { remuxLimit.Store(int64(n)) }

// RemuxOpened / RemuxClosed bracket one held remux slot.
func RemuxOpened() { raise(&peakRemuxes, remuxes.Add(1)) }
func RemuxClosed() { remuxes.Add(-1) }

// RemuxRefused counts a request turned away by the remux cap.
func RemuxRefused() { remuxRejected.Add(1) }

// FFmpegStarted / FFmpegDone bracket one ffmpeg process; Done adds its CPU time.
func FFmpegStarted() { raise(&peakFFmpeg, ffmpeg.Add(1)) }
func FFmpegDone(cpu time.Duration, transcoding bool) {
	ffmpeg.Add(-1)
	if cpu <= 0 {
		return
	}
	if transcoding {
		cpuTranscode.Add(cpu.Milliseconds())
	} else {
		cpuCopy.Add(cpu.Milliseconds())
	}
}

// Session counts one created playback session by client class.
func Session(userAgent string, noAV1Client, needsTranscode bool) {
	sessions.Add(1)
	if IsApple(userAgent) {
		apple.Add(1)
	}
	if noAV1Client {
		noAV1.Add(1)
	}
	if needsTranscode {
		transcode.Add(1)
	}
}

// IsApple reports an iPhone, iPad or iPod user agent. iPadOS 13+ sends a desktop Safari agent
// by default, so iPads in that mode are not recognised: the no-AV1 count is the better proxy.
func IsApple(userAgent string) bool {
	return strings.Contains(userAgent, "iPhone") || strings.Contains(userAgent, "iPad") || strings.Contains(userAgent, "iPod")
}

// Live is the instantaneous saturation of the remux slots.
func Live() (remuxesInUse, limit, ffmpegRunning int64) {
	return remuxes.Load(), remuxLimit.Load(), ffmpeg.Load()
}

// Drain returns what was counted since the last call and resets the window. Peaks restart from
// the current level, not zero, so a long-running stream is still counted in the next window.
func Drain() Delta {
	d := Delta{
		BytesOut:          bytesOut.Swap(0),
		CPUCopyMS:         cpuCopy.Swap(0),
		CPUTranscodeMS:    cpuTranscode.Swap(0),
		Sessions:          sessions.Swap(0),
		SessionsApple:     apple.Swap(0),
		SessionsNoAV1:     noAV1.Swap(0),
		SessionsTranscode: transcode.Swap(0),
		RemuxRejected:     remuxRejected.Swap(0),
		PeakRemuxes:       peakRemuxes.Swap(remuxes.Load()),
		PeakFFmpeg:        peakFFmpeg.Swap(ffmpeg.Load()),
	}
	return d
}

// CountBytes wraps a handler and adds the response body bytes to the "served" counter.
func CountBytes(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(&countingWriter{ResponseWriter: w}, r)
	})
}

type countingWriter struct{ http.ResponseWriter }

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.ResponseWriter.Write(p)
	bytesOut.Add(int64(n))
	return n, err
}
func (c *countingWriter) Flush() {
	if f, ok := c.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
func (c *countingWriter) Unwrap() http.ResponseWriter { return c.ResponseWriter }
