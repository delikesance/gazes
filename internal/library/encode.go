package library

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

// maxEncodeAttempts is how many failed encodes an entry gets before it is left as ORIGINAL for good.
const maxEncodeAttempts = 3

// Verify checks that encoded is a faithful, playable re-encode of original: same duration (+-1 s),
// same number of audio and subtitle tracks, a non-empty file and a clean 2 s decode at the start,
// middle and end.
func Verify(ctx context.Context, p Prober, ffmpeg, original, encoded string) error {
	if ffmpeg == "" {
		ffmpeg = "ffmpeg"
	}
	st, err := os.Stat(encoded)
	if err != nil {
		return fmt.Errorf("verify: %w", err)
	}
	if st.Size() <= 0 {
		return errors.New("verify: encoded file is empty")
	}
	oi, err := p.Probe(ctx, original)
	if err != nil {
		return fmt.Errorf("verify: probe original: %w", err)
	}
	ei, err := p.Probe(ctx, encoded)
	if err != nil {
		return fmt.Errorf("verify: probe encoded: %w", err)
	}
	if d := ei.DurationMS - oi.DurationMS; d > 1000 || d < -1000 {
		return fmt.Errorf("verify: duration %d ms differs from original %d ms", ei.DurationMS, oi.DurationMS)
	}
	if len(ei.AudioCodecs) != len(oi.AudioCodecs) {
		return fmt.Errorf("verify: %d audio tracks, original has %d", len(ei.AudioCodecs), len(oi.AudioCodecs))
	}
	if ei.SubtitleTracks != oi.SubtitleTracks {
		return fmt.Errorf("verify: %d subtitle tracks, original has %d", ei.SubtitleTracks, oi.SubtitleTracks)
	}
	dur := float64(ei.DurationMS) / 1000
	for _, at := range []float64{0, dur / 2, math.Max(0, dur-2)} {
		cmd := exec.CommandContext(ctx, ffmpeg, "-v", "error", "-xerror", "-nostdin",
			"-ss", strconv.FormatFloat(at, 'f', 3, 64), "-t", "2", "-i", encoded, "-f", "null", "-")
		if out, err := cmd.CombinedOutput(); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("verify: decode at %.0fs: %w: %s", at, err, tail(out, 300))
		}
	}
	return nil
}

func tail(b []byte, n int) string {
	b = bytes.TrimSpace(b)
	if len(b) > n {
		b = b[len(b)-n:]
	}
	return string(b)
}

// Encoder re-encodes ORIGINAL library copies to AV1, one at a time, in the background.
type Encoder struct {
	store  *Store
	pool   *Pool
	probe  Prober
	active func() int
	s      EncodeSettings
	clock  func() time.Time

	poll        time.Duration // idle wait between scans for work
	tick        time.Duration // pause/resume check period while encoding
	statusEvery time.Duration // status row refresh period
	signal      func(p *os.Process, sig syscall.Signal) error
	useNice     bool
}

func NewEncoder(store *Store, pool *Pool, p Prober, active func() int, s EncodeSettings, clock func() time.Time) *Encoder {
	if clock == nil {
		clock = time.Now
	}
	if active == nil {
		active = func() int { return 0 }
	}
	_, err := exec.LookPath("nice")
	return &Encoder{
		store: store, pool: pool, probe: p, active: active, s: s, clock: clock,
		poll: 10 * time.Second, tick: time.Second, statusEvery: 10 * time.Second,
		signal:  func(p *os.Process, sig syscall.Signal) error { return p.Signal(sig) },
		useNice: err == nil,
	}
}

// Run processes entries until ctx is cancelled.
func (e *Encoder) Run(ctx context.Context) {
	for ctx.Err() == nil {
		if e.encodeNext(ctx) {
			continue
		}
		e.setStatus(nil, 0, false)
		select {
		case <-ctx.Done():
		case <-time.After(e.poll):
		}
	}
	e.setStatus(nil, 0, false)
}

func (e *Encoder) setStatus(k *Key, progress float64, paused bool) {
	if err := e.store.SetEncoderStatus(EncoderStatus{Key: k, Progress: progress, Paused: paused, UpdatedAt: e.clock()}); err != nil {
		slog.Warn("library: encoder status", "err", err)
	}
}

// encodeNext picks the first encodable ORIGINAL entry and encodes it. It reports whether it did any work.
func (e *Encoder) encodeNext(ctx context.Context) bool {
	list, err := e.store.List(Filter{States: []State{StateOriginal}})
	if err != nil {
		slog.Warn("library: encoder list", "err", err)
		return false
	}
	for _, ent := range list {
		if ent.EncodeSkipped != "" || ent.DiskID == "" || ent.RelPath == "" {
			continue
		}
		if ctx.Err() != nil {
			return false
		}
		if e.encode(ctx, ent) {
			return true
		}
	}
	return false
}

// encode runs one entry. It returns false when the entry could not even be started (no space, lost
// race) so the caller can try the next one.
func (e *Encoder) encode(ctx context.Context, ent Entry) bool {
	k := ent.Key
	src, err := e.pool.Path(ent.DiskID, ent.RelPath)
	if errors.Is(err, ErrDiskAbsent) {
		e.markUnavailable(k, nil)
		return true
	}
	if err != nil {
		slog.Warn("library: encoder path", "key", k.String(), "err", err)
		return false
	}
	st, err := os.Stat(src)
	if errors.Is(err, os.ErrNotExist) {
		e.markUnavailable(k, nil)
		return true
	}
	if err != nil {
		slog.Warn("library: encoder stat", "key", k.String(), "err", err)
		return false
	}

	res, err := e.pool.ReserveOn(ent.DiskID, st.Size()/2)
	if errors.Is(err, ErrDiskAbsent) {
		e.markUnavailable(k, nil)
		return true
	}
	if err != nil { // ErrNoSpace: leave it for later
		return false
	}
	claimed := false
	_, err = e.store.Update(k, func(en *Entry) error {
		if en.State != StateOriginal || en.EncodeSkipped != "" {
			return errors.New("library: entry no longer encodable")
		}
		en.State, en.PrevState = StateEncoding, ""
		en.ReservedBytes = res.Size
		claimed = true
		return nil
	})
	if err != nil || !claimed {
		e.pool.Release(res)
		return false
	}

	tmp := strings.TrimSuffix(src, ".mkv") + ".av1.tmp.mkv"
	os.Remove(tmp)
	e.setStatus(&k, 0, false)

	newSize, encErr := e.runFFmpeg(ctx, k, src, tmp)
	if encErr == nil {
		if newSize >= st.Size() {
			os.Remove(tmp)
			e.finish(k, res, func(en *Entry) {
				en.State, en.EncodeSkipped, en.LastError = StateOriginal, "not_smaller", ""
			})
			return true
		}
		if err := os.Rename(tmp, src); err != nil {
			encErr = fmt.Errorf("rename: %w", err)
		} else {
			e.finish(k, res, func(en *Entry) {
				en.State, en.PrevState = StateAV1, ""
				en.SizeBytes, en.VideoCodec, en.SHA256, en.LastError = newSize, "av1", "", ""
			})
			return true
		}
	}

	os.Remove(tmp)
	switch {
	case ctx.Err() != nil:
		e.finish(k, res, func(en *Entry) { en.State = StateOriginal })
	case e.sourceGone(ent):
		e.finish(k, res, func(en *Entry) { en.State, en.PrevState = StateUnavailable, StateOriginal })
	default:
		slog.Warn("library: encode failed", "key", k.String(), "err", encErr)
		msg := encErr.Error()
		e.finish(k, res, func(en *Entry) {
			en.State = StateOriginal
			en.Attempts++
			en.LastError = msg
			if en.Attempts >= maxEncodeAttempts {
				en.EncodeSkipped = "encode_failed"
			}
		})
	}
	return true
}

// sourceGone reports whether the original file or its disk has disappeared.
func (e *Encoder) sourceGone(ent Entry) bool {
	p, err := e.pool.Path(ent.DiskID, ent.RelPath)
	if errors.Is(err, ErrDiskAbsent) {
		return true
	}
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return errors.Is(err, os.ErrNotExist)
}

func (e *Encoder) markUnavailable(k Key, res *Reservation) {
	_, err := e.store.Update(k, func(en *Entry) error {
		if en.State == StateUnavailable {
			return nil
		}
		en.PrevState, en.State = en.State, StateUnavailable
		return nil
	})
	if err != nil && !errors.Is(err, ErrNotFound) {
		slog.Warn("library: mark unavailable", "key", k.String(), "err", err)
	}
	if res != nil {
		e.pool.Release(*res)
	}
}

// finish applies the terminal state change, clears the reservation and releases it.
func (e *Encoder) finish(k Key, res Reservation, fn func(*Entry)) {
	_, err := e.store.Update(k, func(en *Entry) error {
		fn(en)
		en.ReservedBytes = 0
		return nil
	})
	if err != nil && !errors.Is(err, ErrNotFound) {
		slog.Warn("library: encoder finish", "key", k.String(), "err", err)
	}
	e.pool.Release(res)
	e.setStatus(nil, 0, false)
}

// runFFmpeg encodes src into tmp, pausing and resuming the process as needed, then verifies the
// result. It returns the size of tmp.
func (e *Encoder) runFFmpeg(ctx context.Context, k Key, src, tmp string) (int64, error) {
	info, err := e.probe.Probe(ctx, src)
	if err != nil {
		return 0, fmt.Errorf("probe: %w", err)
	}
	ffmpeg := e.s.ffmpeg()
	args := BuildArgs(src, tmp, info, e.s)
	bin := ffmpeg
	if e.useNice {
		bin, args = "nice", append([]string{"-n", "19", ffmpeg}, args...)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 0, err
	}
	var stderr tailBuffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start ffmpeg: %w", err)
	}

	var progress atomic.Uint64 // float64 bits
	eof := make(chan struct{})
	go func() {
		defer close(eof)
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			v, ok := strings.CutPrefix(sc.Text(), "out_time_us=")
			if !ok || info.DurationMS <= 0 {
				continue
			}
			if us, err := strconv.ParseInt(v, 10, 64); err == nil {
				p := float64(us) / 1000 / float64(info.DurationMS)
				progress.Store(math.Float64bits(math.Max(0, math.Min(1, p))))
			}
		}
	}()
	exited := make(chan error, 1)
	go func() {
		<-eof // stdout must be drained to EOF before Wait closes the pipe
		exited <- cmd.Wait()
	}()

	paused := false
	lastStatus := time.Now()
	ticker := time.NewTicker(e.tick)
	defer ticker.Stop()
loop:
	for {
		select {
		case err := <-exited:
			if err != nil {
				if ctx.Err() != nil {
					return 0, ctx.Err()
				}
				return 0, fmt.Errorf("ffmpeg: %w: %s", err, stderr.String())
			}
			break loop
		case <-ticker.C:
		}
		if want := e.shouldPause(); want != paused {
			sig := syscall.SIGCONT
			if want {
				sig = syscall.SIGSTOP
			}
			if err := e.signal(cmd.Process, sig); err != nil {
				slog.Warn("library: encoder signal", "sig", sig, "err", err)
			} else {
				paused = want
				lastStatus = time.Time{} // report the change now
			}
		}
		if time.Since(lastStatus) >= e.statusEvery {
			lastStatus = time.Now()
			e.setStatus(&k, math.Float64frombits(progress.Load()), paused)
		}
	}

	if err := Verify(ctx, e.probe, ffmpeg, src, tmp); err != nil {
		return 0, err
	}
	st, err := os.Stat(tmp)
	if err != nil {
		return 0, err
	}
	return st.Size(), nil
}

// shouldPause is true outside the time window or while enough streams are being watched.
func (e *Encoder) shouldPause() bool {
	in, err := InWindow(e.s.Window, e.clock())
	if err != nil {
		slog.Warn("library: encoder window", "err", err)
		in = true
	}
	return !in || (e.s.PauseStreams > 0 && e.active() >= e.s.PauseStreams)
}

// tailBuffer keeps the last few KiB written to it (ffmpeg's stderr).
type tailBuffer struct{ b []byte }

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.b = append(t.b, p...)
	if len(t.b) > 4096 {
		t.b = t.b[len(t.b)-4096:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string { return tail(t.b, 500) }
