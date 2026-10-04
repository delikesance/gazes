package library

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
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
	if ei.VideoStreams != 1 {
		return fmt.Errorf("verify: %d video streams, want exactly 1", ei.VideoStreams)
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
	backoff     bool // set after a failed attempt: Run waits poll before the next one

	inUse      func(Key) bool                               // whether a reader has the copy open; nil means never
	free       func(diskID string, need int64)              // makes room on a disk (janitor); nil means no eviction
	update     func(Key, func(*Entry) error) (Entry, error) // store.Update, replaceable in tests
	retryDelay time.Duration                                // pause between retries of the final index update

	// awaiting holds verified AV1 outputs whose swap timed out because the copy kept being read. Their
	// entries stay ENCODING (served from the untouched original) with the encode reservation held, and
	// Run retries the swap on every tick. It is in memory only: after a restart Store.Recover resets
	// ENCODING to ORIGINAL and Janitor.Reconcile removes the orphan .av1.tmp.mkv, so a restart costs one
	// re-encode of those entries (accepted, no schema change). A parked swap holds no pool reservation: the
	// temp bytes are already visible to statfs, so keeping it would count them twice.
	mu       sync.Mutex
	awaiting map[Key]pendingSwap
}

// pendingSwap is a verified encode waiting for the copy to be idle.
type pendingSwap struct {
	diskID   string
	src      string
	tmp      string
	newSize  int64
	parkedAt time.Time
}

// swapParkMax is how long a verified output may wait for its copy to go idle before it is dropped.
const swapParkMax = 24 * time.Hour

const (
	swapIdleAfter = 10 * time.Minute // a copy read more recently than this is not swapped
	finishRetries = 5
)

// envError marks a failure of the environment (cannot start ffmpeg, cannot create the output),
// which says nothing about the media file and must not count as an attempt.
type envError struct{ err error }

func (e envError) Error() string { return e.err.Error() }
func (e envError) Unwrap() error { return e.err }

func isEnvError(err error) bool {
	var ee envError
	return errors.As(err, &ee) || errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrPermission)
}

func NewEncoder(store *Store, pool *Pool, p Prober, active func() int, s EncodeSettings, clock func() time.Time) *Encoder {
	if clock == nil {
		clock = time.Now
	}
	if active == nil {
		active = func() int { return 0 }
	}
	if _, err := InWindow(s.Window, time.Now()); err != nil {
		slog.Warn("library: invalid encode window, encoding at any time", "window", s.Window, "err", err)
		s.Window = ""
	}
	_, err := exec.LookPath("nice")
	return &Encoder{
		store: store, pool: pool, probe: p, active: active, s: s, clock: clock,
		poll: 10 * time.Second, tick: time.Second, statusEvery: 10 * time.Second,
		signal:     func(p *os.Process, sig syscall.Signal) error { return p.Signal(sig) },
		useNice:    err == nil,
		retryDelay: 500 * time.Millisecond,
		update:     store.Update,
	}
}

// Run processes entries until ctx is cancelled.
func (e *Encoder) Run(ctx context.Context) {
	for ctx.Err() == nil {
		e.swapPending(ctx)
		if e.encodeNext(ctx) && !e.backoff {
			continue
		}
		e.backoff = false
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
		return e.markUnavailable(k, nil)
	}
	if err != nil {
		slog.Warn("library: encoder path", "key", k.String(), "err", err)
		return false
	}
	st, err := os.Stat(src)
	if errors.Is(err, os.ErrNotExist) {
		return e.markUnavailable(k, nil)
	}
	if err != nil {
		slog.Warn("library: encoder stat", "key", k.String(), "err", err)
		return false
	}

	res, err := e.pool.ReserveOn(ent.DiskID, st.Size()/2)
	if errors.Is(err, ErrNoSpace) && e.free != nil {
		e.free(ent.DiskID, st.Size()/2)
		res, err = e.pool.ReserveOn(ent.DiskID, st.Size()/2)
	}
	if errors.Is(err, ErrDiskAbsent) {
		return e.markUnavailable(k, nil)
	}
	if err != nil { // ErrNoSpace: leave it for later
		return false
	}
	claimed := false
	_, err = e.update(k, func(en *Entry) error {
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
			return e.finish(k, res, func(en *Entry) {
				en.State, en.EncodeSkipped, en.LastError = StateOriginal, "not_smaller", ""
			})
		}
		// Try the swap once without blocking. A copy in use (or read recently) is parked with its verified
		// output, ENCODING (served from the original) and its reservation: Run retries on later ticks and
		// the queue moves on instead of waiting or re-encoding.
		p := pendingSwap{diskID: ent.DiskID, src: src, tmp: tmp, newSize: newSize, parkedAt: e.clock()}
		e.park(k, p)
		e.pool.Release(res)
		e.clearReserved(k)
		if e.trySwap(k, p) {
			e.mu.Lock()
			delete(e.awaiting, k)
			e.mu.Unlock()
		} else {
			slog.Info("library: encoded copy in use or recently read, swap postponed", "key", k.String())
			e.setStatus(nil, 0, false)
		}
		return true
	}

	os.Remove(tmp)
	switch {
	case ctx.Err() != nil:
		return e.finish(k, res, func(en *Entry) { en.State = StateOriginal })
	case e.sourceGone(ent):
		return e.finish(k, res, func(en *Entry) { en.State, en.PrevState = StateUnavailable, StateOriginal })
	case isEnvError(encErr):
		slog.Warn("library: encoder environment failure", "key", k.String(), "err", encErr)
		e.backoff = true
		msg := encErr.Error()
		return e.finish(k, res, func(en *Entry) { en.State, en.LastError = StateOriginal, msg })
	default:
		slog.Warn("library: encode failed", "key", k.String(), "err", encErr)
		e.backoff = true
		msg := encErr.Error()
		return e.finish(k, res, func(en *Entry) {
			en.State = StateOriginal
			en.Attempts++
			en.LastError = msg
			if en.Attempts >= maxEncodeAttempts {
				en.EncodeSkipped = "encode_failed"
			}
		})
	}
}

// clearReserved zeroes the entry's reserved bytes once its reservation is given back.
func (e *Encoder) clearReserved(k Key) {
	if _, err := e.update(k, func(en *Entry) error { en.ReservedBytes = 0; return nil }); err != nil && !errors.Is(err, ErrNotFound) {
		slog.Warn("library: clear reserved bytes", "key", k.String(), "err", err)
	}
}

func (e *Encoder) park(k Key, p pendingSwap) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.awaiting == nil {
		e.awaiting = map[Key]pendingSwap{}
	}
	e.awaiting[k] = p
}

// pendingSwaps is the number of verified outputs waiting for their copy to be idle.
func (e *Encoder) pendingSwaps() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.awaiting)
}

// swapPending tries, without blocking, to complete every postponed swap whose copy is now unused and idle.
// A swap whose entry changed or whose files vanished is dropped and cleaned up.
func (e *Encoder) swapPending(ctx context.Context) {
	e.mu.Lock()
	keys := make([]Key, 0, len(e.awaiting))
	for k := range e.awaiting {
		keys = append(keys, k)
	}
	e.mu.Unlock()
	for _, k := range keys {
		if ctx.Err() != nil {
			return
		}
		e.mu.Lock()
		p := e.awaiting[k]
		e.mu.Unlock()
		if e.trySwap(k, p) {
			e.mu.Lock()
			delete(e.awaiting, k)
			e.mu.Unlock()
		}
	}
}

// trySwap reports whether the pending swap is settled (done or dropped) and so leaves the awaiting set.
func (e *Encoder) trySwap(k Key, p pendingSwap) bool {
	cur, err := e.store.Get(k)
	if err != nil && !errors.Is(err, ErrNotFound) {
		slog.Warn("library: pending swap lookup", "key", k.String(), "err", err)
		return false
	}
	if err != nil || cur.State != StateEncoding {
		// Deleted, or someone else already moved the entry on: only the temp is ours to clean up.
		os.Remove(p.tmp)
		return true
	}
	none := Reservation{} // released when parked
	back := func(fn func(*Entry)) bool {
		os.Remove(p.tmp)
		e.finish(k, none, fn)
		return true
	}
	toOriginal := func(en *Entry) { en.State = StateOriginal }
	if cur.DiskID != p.diskID {
		return back(toOriginal) // the entry moved to another disk: the output is useless
	}
	if _, err := e.pool.Path(cur.DiskID, cur.RelPath); err != nil || e.sourceGone(cur) {
		return back(func(en *Entry) { en.State, en.PrevState = StateUnavailable, StateOriginal })
	}
	if e.inUse != nil && e.inUse(k) || e.clock().Sub(cur.LastAccessAt) <= swapIdleAfter {
		if e.clock().Sub(p.parkedAt) <= swapParkMax {
			return false
		}
		slog.Warn("library: pending swap expired, discarding the output", "key", k.String())
		// updated_at moves to now, so the entry sorts after the other ORIGINAL ones and is not re-picked at once.
		return back(func(en *Entry) { en.State, en.LastError = StateOriginal, "swap postponed 24h" })
	}
	if _, err := os.Stat(p.tmp); err != nil {
		slog.Warn("library: pending swap output vanished", "key", k.String(), "err", err)
		return back(toOriginal)
	}
	if err := os.Rename(p.tmp, p.src); err != nil {
		slog.Warn("library: pending swap rename", "key", k.String(), "err", err)
		e.backoff = true
		msg := fmt.Sprintf("rename: %v", err)
		return back(func(en *Entry) {
			en.State = StateOriginal
			en.Attempts++
			en.LastError = msg
			if en.Attempts >= maxEncodeAttempts {
				en.EncodeSkipped = "encode_failed"
			}
		})
	}
	e.finish(k, none, func(en *Entry) {
		en.State, en.PrevState = StateAV1, ""
		en.SizeBytes, en.VideoCodec, en.SHA256, en.LastError = p.newSize, "av1", "", ""
	})
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

// markUnavailable flags the entry UNAVAILABLE. It returns false if the index could not be updated.
func (e *Encoder) markUnavailable(k Key, res *Reservation) bool {
	_, err := e.update(k, func(en *Entry) error {
		if en.State == StateUnavailable {
			return nil
		}
		en.PrevState, en.State = en.State, StateUnavailable
		return nil
	})
	if res != nil {
		e.pool.Release(*res)
	}
	if err != nil && !errors.Is(err, ErrNotFound) {
		slog.Warn("library: mark unavailable", "key", k.String(), "err", err)
		return false
	}
	return true
}

// finish applies the terminal state change, clears the reservation and releases it. It returns
// false if the index could not be updated, so the caller does not spin on the same entry.
func (e *Encoder) finish(k Key, res Reservation, fn func(*Entry)) bool {
	// Retry: after a successful rename a lost update would leave the index at ENCODING on an AV1 file.
	var err error
	for i := 0; i < finishRetries; i++ {
		_, err = e.update(k, func(en *Entry) error {
			fn(en)
			en.ReservedBytes = 0
			return nil
		})
		if err == nil || errors.Is(err, ErrNotFound) {
			break
		}
		time.Sleep(e.retryDelay)
	}
	e.pool.Release(res)
	e.setStatus(nil, 0, false)
	if err != nil && !errors.Is(err, ErrNotFound) {
		slog.Warn("library: encoder finish", "key", k.String(), "err", err)
		return false
	}
	return true
}

// runFFmpeg encodes src into tmp, pausing and resuming the process as needed, then verifies the
// result. It returns the size of tmp.
func (e *Encoder) runFFmpeg(ctx context.Context, k Key, src, tmp string) (int64, error) {
	info, err := e.probe.Probe(ctx, src)
	if err != nil {
		return 0, fmt.Errorf("probe: %w", err)
	}
	if f, err := os.Create(tmp); err != nil { // fail early, and as an environment error, if the output is unwritable
		return 0, envError{fmt.Errorf("create output: %w", err)}
	} else {
		f.Close()
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
		return 0, envError{fmt.Errorf("start ffmpeg: %w", err)}
	}

	var progress atomic.Uint64 // float64 bits
	eof := make(chan struct{})
	go func() {
		defer close(eof)
		sc := bufio.NewScanner(stdout)
		pp := newProgressParser(info.DurationMS, info.FrameRate)
		for sc.Scan() {
			if p, ok := pp.Line(sc.Text()); ok {
				progress.Store(math.Float64bits(p))
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

// progressParser turns ffmpeg's -progress lines into a 0..1 fraction. Which fields carry a value depends
// on the argument set: out_time_us is N/A for some stream mappings, hence the fallbacks: out_time_ms
// (microseconds despite its name), out_time=HH:MM:SS.xx, then frame= against frame rate and duration.
type progressParser struct {
	durationMS int64
	fps        float64
	frames     int64
	timeSeen   bool // a time field gave a value: frame counts are then ignored
}

func newProgressParser(durationMS int64, fps float64) *progressParser {
	return &progressParser{durationMS: durationMS, fps: fps}
}

// Line consumes one line and reports the new progress when it carries one.
func (p *progressParser) Line(line string) (float64, bool) {
	if p.durationMS <= 0 {
		return 0, false
	}
	key, val, ok := strings.Cut(line, "=")
	if !ok {
		return 0, false
	}
	val = strings.TrimSpace(val)
	var us int64
	switch key {
	case "out_time_us", "out_time_ms":
		n, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return 0, false
		}
		us = n
	case "out_time":
		n, ok := parseOutTime(val)
		if !ok {
			return 0, false
		}
		us = n
	case "frame":
		n, err := strconv.ParseInt(val, 10, 64)
		if err != nil || n < 0 {
			return 0, false
		}
		p.frames = n
		if p.timeSeen || p.fps <= 0 {
			return 0, false
		}
		return clamp01(float64(n) / (p.fps * float64(p.durationMS) / 1000)), true
	default:
		return 0, false
	}
	p.timeSeen = true
	return clamp01(float64(us) / 1000 / float64(p.durationMS)), true
}

func clamp01(f float64) float64 { return math.Max(0, math.Min(1, f)) }

// parseOutTime reads ffmpeg's out_time ("HH:MM:SS.ffffff", possibly negative at the very start) in microseconds.
func parseOutTime(s string) (int64, bool) {
	neg := strings.HasPrefix(s, "-")
	parts := strings.Split(strings.TrimPrefix(s, "-"), ":")
	if len(parts) != 3 {
		return 0, false
	}
	h, err1 := strconv.ParseInt(parts[0], 10, 64)
	m, err2 := strconv.ParseInt(parts[1], 10, 64)
	sec, err3 := strconv.ParseFloat(parts[2], 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, false
	}
	us := (h*3600+m*60)*1_000_000 + int64(sec*1_000_000)
	if neg {
		us = -us
	}
	return us, true
}
