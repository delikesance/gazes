package admin

import (
	"context"
	"time"

	"github.com/gazes/gazes/internal/loadstats"
)

// DefaultLoadFlushInterval is how often the in-process load counters are written to the daily table.
const DefaultLoadFlushInterval = time.Minute

// FlushLoad drains the in-process load counters into today's metrics_load_daily row (sums for the
// counters, maximum for the peaks). A write failure drops that window: a metric never affects a playback.
func (s *Service) FlushLoad(ctx context.Context) {
	if s == nil {
		return
	}
	d := loadstats.Drain()
	if d == (loadstats.Delta{}) {
		return
	}
	_, _ = s.adminDB().ExecContext(ctx, `INSERT INTO metrics_load_daily
		(day, bytes_out, cpu_copy_ms, cpu_transcode_ms, sessions, sessions_apple, sessions_noav1, sessions_transcode, remux_rejected, peak_remuxes, peak_ffmpeg)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(day) DO UPDATE SET
			bytes_out = bytes_out + excluded.bytes_out,
			cpu_copy_ms = cpu_copy_ms + excluded.cpu_copy_ms,
			cpu_transcode_ms = cpu_transcode_ms + excluded.cpu_transcode_ms,
			sessions = sessions + excluded.sessions,
			sessions_apple = sessions_apple + excluded.sessions_apple,
			sessions_noav1 = sessions_noav1 + excluded.sessions_noav1,
			sessions_transcode = sessions_transcode + excluded.sessions_transcode,
			remux_rejected = remux_rejected + excluded.remux_rejected,
			peak_remuxes = MAX(peak_remuxes, excluded.peak_remuxes),
			peak_ffmpeg = MAX(peak_ffmpeg, excluded.peak_ffmpeg)`,
		s.now().UTC().Format(dayLayout), d.BytesOut, d.CPUCopyMS, d.CPUTranscodeMS, d.Sessions, d.SessionsApple,
		d.SessionsNoAV1, d.SessionsTranscode, d.RemuxRejected, d.PeakRemuxes, d.PeakFFmpeg)
}

// RunLoadFlush flushes every interval until ctx ends, then once more so a restart loses nothing.
func (s *Service) RunLoadFlush(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			s.FlushLoad(context.Background())
			return
		case <-t.C:
			s.FlushLoad(ctx)
		}
	}
}

// pbLoad sums the daily load counters over the days from..to. since is the first day with data
// ("" when none): the counters only exist from the release that introduced them.
func (s *Service) pbLoad(ctx context.Context, from, to string) (map[string]any, error) {
	s.FlushLoad(ctx) // include what the current minute has counted
	var (
		bytes, cpuCopy, cpuTr, sess, apple, noav1, tr, rej, peakRx, peakFF int64
		days                                                               int64
		since                                                              *string
	)
	err := s.adminDB().QueryRowContext(ctx, `SELECT COALESCE(SUM(bytes_out),0), COALESCE(SUM(cpu_copy_ms),0),
		COALESCE(SUM(cpu_transcode_ms),0), COALESCE(SUM(sessions),0), COALESCE(SUM(sessions_apple),0),
		COALESCE(SUM(sessions_noav1),0), COALESCE(SUM(sessions_transcode),0), COALESCE(SUM(remux_rejected),0),
		COALESCE(MAX(peak_remuxes),0), COALESCE(MAX(peak_ffmpeg),0), COUNT(*), MIN(day)
		FROM metrics_load_daily WHERE day >= ? AND day <= ?`, from, to).
		Scan(&bytes, &cpuCopy, &cpuTr, &sess, &apple, &noav1, &tr, &rej, &peakRx, &peakFF, &days, &since)
	if err != nil {
		return nil, err
	}
	inUse, limit, running := loadstats.Live()
	share := func(n int64) any {
		if sess == 0 {
			return nil
		}
		return float64(n) / float64(sess)
	}
	out := map[string]any{
		"measured_days":         days, // days of the period with load data (0: nothing recorded yet)
		"bytes_out":             bytes,
		"cpu_seconds_copy":      float64(cpuCopy) / 1000,
		"cpu_seconds_transcode": float64(cpuTr) / 1000,
		"sessions":              sess,
		"apple_share":           share(apple),
		"no_av1_share":          share(noav1),
		"transcode_share":       share(tr),
		"remux_rejected":        rej,
		"peak_remuxes":          peakRx,
		"remux_limit":           limit,
		"peak_ffmpeg":           peakFF,
		"live_remuxes":          inUse,
		"live_ffmpeg":           running,
		"since":                 since,
	}
	return out, nil
}
