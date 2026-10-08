package library

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// fakeFFmpeg writes a shell script standing in for ffmpeg. Verify's decode calls (-f null) always
// succeed; body runs for the encode call.
func fakeFFmpeg(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "ffmpeg")
	script := "#!/bin/sh\ncase \"$*\" in *\"-f null\"*) exit 0;; esac\nfor a; do out=$a; done\n" + body
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func writeOutput(size int) string {
	return fmt.Sprintf("echo out_time_us=500000\nhead -c %d /dev/zero > \"$out\"\n", size)
}

type encEnv struct {
	enc      *Encoder
	store    *Store
	pool     *Pool
	key      Key
	diskDir  string
	src      string
	original []byte
}

const origSize = 1000

func newEncEnv(t *testing.T, ffmpegBody string, s EncodeSettings) *encEnv {
	t.Helper()
	pool, store, root := newPoolTest(t, fakeFS{"d1": {1 << 40, 1 << 39}}, 0, 0, "d1")
	if _, _, err := pool.Scan(); err != nil {
		t.Fatal(err)
	}
	disk := pool.Disks()[0]
	rel := "7/1-vostfr.mkv"
	src := filepath.Join(disk.Path, rel)
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	data := bytes.Repeat([]byte("x"), origSize)
	if err := os.WriteFile(src, data, 0o644); err != nil {
		t.Fatal(err)
	}
	k := Key{SeasonID: 7, Episode: 1, Lang: "vostfr"}
	if err := store.Create(Entry{Key: k, AnimeID: 1, Title: "T", State: StateOriginal, DiskID: disk.ID, RelPath: rel,
		SizeBytes: origSize, OriginalSizeBytes: origSize, VideoCodec: "h264"}); err != nil {
		t.Fatal(err)
	}
	_ = root
	s.FFmpeg = fakeFFmpeg(t, ffmpegBody)
	enc := NewEncoder(store, pool, fakeProber{}, nil, s, nil)
	enc.useNice = false
	enc.retryDelay = time.Millisecond
	enc.poll, enc.tick, enc.statusEvery = 10*time.Millisecond, 10*time.Millisecond, 10*time.Millisecond
	return &encEnv{enc, store, pool, k, disk.Path, src, data}
}

func (e *encEnv) entry(t *testing.T) Entry {
	t.Helper()
	en, err := e.store.Get(e.key)
	if err != nil {
		t.Fatal(err)
	}
	return en
}

func (e *encEnv) reserved() int64 {
	e.pool.mu.Lock()
	defer e.pool.mu.Unlock()
	var n int64
	for _, v := range e.pool.reserved {
		n += v
	}
	return n
}

func (e *encEnv) noTmp(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(e.diskDir, "7/1-vostfr.av1.tmp.mkv")); !os.IsNotExist(err) {
		t.Fatalf("tmp file left behind (err=%v)", err)
	}
}

func TestEncoderSuccessReplacesOriginal(t *testing.T) {
	env := newEncEnv(t, writeOutput(400), EncodeSettings{})
	if !env.enc.encodeNext(context.Background()) {
		t.Fatal("no work done")
	}
	en := env.entry(t)
	if en.State != StateAV1 || en.RelPath != "7/1-vostfr.mkv" || en.SizeBytes != 400 || en.OriginalSizeBytes != origSize ||
		en.VideoCodec != "av1" || en.ReservedBytes != 0 || en.Attempts != 0 {
		t.Fatalf("entry %+v", en)
	}
	if st, err := os.Stat(env.src); err != nil || st.Size() != 400 {
		t.Fatalf("final file: %v %v", st, err)
	}
	env.noTmp(t)
	if env.reserved() != 0 {
		t.Fatalf("reservation leaked: %d", env.reserved())
	}
	if st, _ := env.store.EncoderStatus(); st.Key != nil {
		t.Fatalf("status not idle: %+v", st)
	}
	if env.enc.encodeNext(context.Background()) {
		t.Fatal("AV1 entry picked again")
	}
}

func TestEncoderFailureKeepsOriginalAndCountsAttempts(t *testing.T) {
	env := newEncEnv(t, "echo boom >&2\nexit 1\n", EncodeSettings{})
	for i := 1; i <= 3; i++ {
		if !env.enc.encodeNext(context.Background()) {
			t.Fatalf("attempt %d: no work", i)
		}
		en := env.entry(t)
		if en.State != StateOriginal || en.Attempts != i || en.LastError == "" || en.ReservedBytes != 0 {
			t.Fatalf("attempt %d: %+v", i, en)
		}
		want := ""
		if i == 3 {
			want = "encode_failed"
		}
		if en.EncodeSkipped != want {
			t.Fatalf("attempt %d: EncodeSkipped %q", i, en.EncodeSkipped)
		}
		got, err := os.ReadFile(env.src)
		if err != nil || !bytes.Equal(got, env.original) {
			t.Fatalf("original altered: %v", err)
		}
		env.noTmp(t)
		if env.reserved() != 0 {
			t.Fatalf("reservation leaked: %d", env.reserved())
		}
	}
	if env.enc.encodeNext(context.Background()) {
		t.Fatal("entry retried after encode_failed")
	}
}

func TestEncoderNotSmallerKeepsOriginal(t *testing.T) {
	env := newEncEnv(t, writeOutput(origSize+50), EncodeSettings{})
	env.enc.encodeNext(context.Background())
	en := env.entry(t)
	if en.State != StateOriginal || en.EncodeSkipped != "not_smaller" || en.Attempts != 0 || en.SizeBytes != origSize {
		t.Fatalf("entry %+v", en)
	}
	got, _ := os.ReadFile(env.src)
	if !bytes.Equal(got, env.original) {
		t.Fatal("original altered")
	}
	env.noTmp(t)
	if env.enc.encodeNext(context.Background()) {
		t.Fatal("not_smaller entry retried")
	}
}

func TestDiskRemovedDuringEncodeMarksUnavailable(t *testing.T) {
	env := newEncEnv(t, "echo out_time_us=1\nrm -rf \"$DISK_DIR\"\nexit 1\n", EncodeSettings{})
	t.Setenv("DISK_DIR", env.diskDir)
	env.enc.encodeNext(context.Background())
	en := env.entry(t)
	if en.State != StateUnavailable || en.PrevState != StateOriginal || en.Attempts != 0 || en.EncodeSkipped != "" {
		t.Fatalf("entry %+v", en)
	}
	if env.reserved() != 0 {
		t.Fatalf("reservation leaked: %d", env.reserved())
	}
}

func TestEncoderMissingFileMarksUnavailable(t *testing.T) {
	env := newEncEnv(t, writeOutput(10), EncodeSettings{})
	os.Remove(env.src)
	env.enc.encodeNext(context.Background())
	if en := env.entry(t); en.State != StateUnavailable || en.Attempts != 0 {
		t.Fatalf("entry %+v", en)
	}
}

func TestEncoderCancelKillsFFmpegAndRestoresOriginal(t *testing.T) {
	env := newEncEnv(t, "echo out_time_us=1\nhead -c 10 /dev/zero > \"$out\"\nexec sleep 30\n", EncodeSettings{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { env.enc.encodeNext(ctx); close(done) }()
	waitFor(t, func() bool { return env.entry(t).State == StateEncoding })
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("encodeNext did not return after cancel")
	}
	en := env.entry(t)
	if en.State != StateOriginal || en.Attempts != 0 || en.EncodeSkipped != "" || en.ReservedBytes != 0 {
		t.Fatalf("entry %+v", en)
	}
	env.noTmp(t)
	if env.reserved() != 0 {
		t.Fatalf("reservation leaked: %d", env.reserved())
	}
}

func TestEncoderNoSpaceLeavesEntryAlone(t *testing.T) {
	env := newEncEnv(t, writeOutput(10), EncodeSettings{})
	if _, err := env.pool.ReserveOn(env.entry(t).DiskID, 1<<39); err != nil {
		t.Fatal(err)
	}
	if env.enc.encodeNext(context.Background()) {
		t.Fatal("encoded without space")
	}
	if en := env.entry(t); en.State != StateOriginal || en.Attempts != 0 {
		t.Fatalf("entry %+v", en)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not reached in time")
}

type sigRecorder struct {
	mu   sync.Mutex
	sigs []syscall.Signal
}

func (r *sigRecorder) send(_ *os.Process, s syscall.Signal) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sigs = append(r.sigs, s)
	return nil
}

func (r *sigRecorder) last() syscall.Signal {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.sigs) == 0 {
		return 0
	}
	return r.sigs[len(r.sigs)-1]
}

func TestEncoderPausesWhenBusyOutsideWindowOrPeak(t *testing.T) {
	for _, tc := range []string{"streams", "window", "peak"} {
		t.Run(tc, func(t *testing.T) {
			s := EncodeSettings{}
			var active atomic.Int32
			var blocked atomic.Bool // outside the window, or inside peak hours
			noon, night := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC), time.Date(2026, 1, 1, 2, 0, 0, 0, time.UTC)
			switch tc {
			case "streams":
				s.PauseStreams = 3
			case "window":
				s.Window = "01:00-09:00"
			case "peak":
				s.PeakWindow = "11:00-14:00"
			}
			gate := filepath.Join(t.TempDir(), "go")
			env := newEncEnv(t, fmt.Sprintf("while [ ! -f %q ]; do sleep 0.02; done\n%s", gate, writeOutput(400)), s)
			env.enc.active = func() int { return int(active.Load()) }
			env.enc.clock = func() time.Time {
				if blocked.Load() {
					return noon
				}
				return night
			}
			rec := &sigRecorder{}
			env.enc.signal = rec.send

			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() { env.enc.Run(ctx); close(done) }()
			defer func() { cancel(); <-done }()

			// Encoding starts unpaused, then the condition kicks in.
			waitFor(t, func() bool {
				st, _ := env.store.EncoderStatus()
				return st.Key != nil && *st.Key == env.key && !st.Paused
			})
			active.Store(3)
			blocked.Store(true)
			waitFor(t, func() bool {
				st, _ := env.store.EncoderStatus()
				return st.Paused && st.Key != nil && *st.Key == env.key
			})
			if rec.last() != syscall.SIGSTOP {
				t.Fatalf("last signal %v, want SIGSTOP", rec.last())
			}
			active.Store(0)
			blocked.Store(false)
			waitFor(t, func() bool {
				st, _ := env.store.EncoderStatus()
				return !st.Paused && st.Key != nil
			})
			if rec.last() != syscall.SIGCONT {
				t.Fatalf("last signal %v, want SIGCONT", rec.last())
			}
			if err := os.WriteFile(gate, nil, 0o644); err != nil {
				t.Fatal(err)
			}
			waitFor(t, func() bool { return env.entry(t).State == StateAV1 })
		})
	}
}

func TestEncoderDoesNotStartWhilePaused(t *testing.T) {
	env := newEncEnv(t, writeOutput(400), EncodeSettings{PeakWindow: "11:00-14:00"})
	env.enc.clock = func() time.Time { return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC) }
	if env.enc.encodeNext(context.Background()) {
		t.Fatal("encode started during peak hours")
	}
	if st := env.entry(t).State; st != StateOriginal {
		t.Fatalf("entry claimed during peak hours: %v", st)
	}
	env.enc.clock = func() time.Time { return time.Date(2026, 1, 1, 2, 0, 0, 0, time.UTC) }
	if !env.enc.encodeNext(context.Background()) {
		t.Fatal("encode did not start off-peak")
	}
}

func TestEncoderReportsProgress(t *testing.T) {
	gate := filepath.Join(t.TempDir(), "go")
	// fakeProber says 1420000 ms; 710000000 us is half of it.
	env := newEncEnv(t, fmt.Sprintf("echo out_time_us=710000000\nwhile [ ! -f %q ]; do sleep 0.02; done\n%s", gate, writeOutput(400)), EncodeSettings{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { env.enc.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	waitFor(t, func() bool {
		st, _ := env.store.EncoderStatus()
		return st.Key != nil && st.Progress > 0.45 && st.Progress < 0.55
	})
	os.WriteFile(gate, nil, 0o644)
	waitFor(t, func() bool { return env.entry(t).State == StateAV1 })
}

func TestEncodeReturnsFalseWhenStoreUpdateFails(t *testing.T) {
	// Early path: file gone -> markUnavailable, whose Update fails on a closed store.
	env := newEncEnv(t, writeOutput(10), EncodeSettings{})
	ent := env.entry(t)
	os.Remove(env.src)
	env.store.Close()
	if env.enc.encode(context.Background(), ent) {
		t.Fatal("encode reported progress although the store update failed (Run would spin)")
	}
}

func TestEncodeReturnsFalseWhenTerminalUpdateFails(t *testing.T) {
	dir := t.TempDir()
	started, gate := filepath.Join(dir, "started"), filepath.Join(dir, "go")
	env := newEncEnv(t, fmt.Sprintf("touch %q\nwhile [ ! -f %q ]; do sleep 0.02; done\nexit 1\n", started, gate), EncodeSettings{})
	ent := env.entry(t)
	result := make(chan bool, 1)
	go func() { result <- env.enc.encode(context.Background(), ent) }()
	waitFor(t, func() bool { _, err := os.Stat(started); return err == nil })
	env.store.Close()
	os.WriteFile(gate, nil, 0o644)
	select {
	case ok := <-result:
		if ok {
			t.Fatal("encode reported progress although the terminal update failed")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("encode did not return")
	}
}

func TestEncoderMissingFFmpegDoesNotBurnAttempts(t *testing.T) {
	env := newEncEnv(t, writeOutput(10), EncodeSettings{})
	env.enc.s.FFmpeg = filepath.Join(t.TempDir(), "no-such-ffmpeg")
	for i := 0; i < 4; i++ {
		if !env.enc.encodeNext(context.Background()) {
			t.Fatalf("round %d: no work", i)
		}
		en := env.entry(t)
		if en.State != StateOriginal || en.Attempts != 0 || en.EncodeSkipped != "" || en.LastError == "" || en.ReservedBytes != 0 {
			t.Fatalf("round %d: %+v", i, en)
		}
	}
	if env.reserved() != 0 {
		t.Fatalf("reservation leaked: %d", env.reserved())
	}
}

func TestEncoderUnwritableTempDoesNotBurnAttempts(t *testing.T) {
	env := newEncEnv(t, writeOutput(10), EncodeSettings{})
	// A directory squatting on the temp path makes it impossible to create the file.
	squat := filepath.Join(env.diskDir, "7/1-vostfr.av1.tmp.mkv")
	if err := os.Mkdir(squat, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, squat, "keep", 1) // non-empty: the pre-encode cleanup cannot remove it

	env.enc.encodeNext(context.Background())
	if en := env.entry(t); en.State != StateOriginal || en.Attempts != 0 || en.LastError == "" {
		t.Fatalf("entry %+v", en)
	}
}

func TestRunWaitsPollBetweenFailedAttempts(t *testing.T) {
	log := filepath.Join(t.TempDir(), "calls")
	env := newEncEnv(t, fmt.Sprintf("date +%%s%%N >> %q\nexit 1\n", log), EncodeSettings{})
	env.enc.poll = 200 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { env.enc.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	var stamps []int64
	waitFor(t, func() bool {
		b, _ := os.ReadFile(log)
		stamps = stamps[:0]
		for _, f := range strings.Fields(string(b)) {
			n, _ := strconv.ParseInt(f, 10, 64)
			stamps = append(stamps, n)
		}
		return len(stamps) >= 2
	})
	if gap := time.Duration(stamps[1] - stamps[0]); gap < 180*time.Millisecond {
		t.Fatalf("failed attempts only %v apart, want >= poll (200ms)", gap)
	}
}

func TestNewEncoderDropsInvalidWindow(t *testing.T) {
	env := newEncEnv(t, writeOutput(10), EncodeSettings{})
	enc := NewEncoder(env.store, env.pool, fakeProber{}, nil, EncodeSettings{Window: "bogus"}, nil)
	if enc.s.Window != "" || enc.shouldPause() {
		t.Fatalf("invalid window kept: %q", enc.s.Window)
	}
}

func TestProgressParser(t *testing.T) {
	cases := []struct {
		name  string
		fps   float64
		lines []string
		want  float64 // -1: no progress reported
	}{
		{"out_time_us", 25, []string{"frame=10", "out_time_us=710000000"}, 0.5},
		{"out_time_ms is microseconds too", 25, []string{"out_time_us=N/A", "out_time_ms=355000000"}, 0.25},
		{"out_time clock", 25, []string{"out_time_us=N/A", "out_time_ms=N/A", "out_time=00:11:50.000000"}, 0.5},
		{"clamped above 1", 25, []string{"out_time_us=9000000000"}, 1},
		{"negative start", 25, []string{"out_time=-00:00:00.040000"}, 0},
		{"all time fields N/A falls back to frames", 25, []string{"frame=17750", "out_time_us=N/A", "out_time_ms=N/A", "out_time=N/A"}, 0.5},
		{"frames need a frame rate", 0, []string{"frame=17750", "out_time_us=N/A", "out_time=N/A"}, -1},
		{"nothing usable", 25, []string{"frame=N/A", "out_time_us=N/A", "progress=continue"}, -1},
		{"time wins over frames afterwards", 25, []string{"out_time_us=355000000", "frame=35500"}, 0.25},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := newProgressParser(1420000, c.fps) // 1420 s
			got := -1.0
			for _, l := range c.lines {
				if v, ok := p.Line(l); ok {
					got = v
				}
			}
			if math.Abs(got-c.want) > 1e-9 {
				t.Fatalf("got %v want %v", got, c.want)
			}
		})
	}
}

func TestEncoderReportsProgressFromFrames(t *testing.T) {
	gate := filepath.Join(t.TempDir(), "go")
	// fakeProber: 1420000 ms at 25 fps = 35500 frames.
	env := newEncEnv(t, fmt.Sprintf("echo frame=17750\necho out_time_us=N/A\necho out_time=N/A\nwhile [ ! -f %q ]; do sleep 0.02; done\n%s", gate, writeOutput(400)), EncodeSettings{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { env.enc.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	waitFor(t, func() bool {
		st, _ := env.store.EncoderStatus()
		return st.Key != nil && st.Progress > 0.45 && st.Progress < 0.55
	})
	os.WriteFile(gate, nil, 0o644)
	waitFor(t, func() bool { return env.entry(t).State == StateAV1 })
}

func TestEncoderParksImmediatelyWhileCopyIsInUse(t *testing.T) {
	env := newEncEnv(t, writeOutput(400), EncodeSettings{})
	var busy atomic.Bool
	busy.Store(true)
	env.enc.inUse = func(Key) bool { return busy.Load() }
	start := time.Now()
	if !env.enc.encodeNext(context.Background()) {
		t.Fatal("no work done")
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("encoder waited for the copy")
	}
	if st, err := os.Stat(env.src); err != nil || st.Size() != origSize {
		t.Fatalf("original replaced while a reader is open: %v %v", st, err)
	}
	if en := env.entry(t); en.State != StateEncoding {
		t.Fatalf("state %s, want ENCODING", en.State)
	}
	busy.Store(false)
	env.enc.clock = func() time.Time { return time.Now().Add(time.Hour) }
	env.enc.swapPending(context.Background())
	if en := env.entry(t); en.State != StateAV1 || en.SizeBytes != 400 {
		t.Fatalf("entry %+v", en)
	}
	if st, _ := os.Stat(env.src); st == nil || st.Size() != 400 {
		t.Fatalf("final file %v", st)
	}
}

func TestEncoderParksWhenAccessedRecently(t *testing.T) {
	now := time.Now()
	env := newEncEnv(t, writeOutput(400), EncodeSettings{})
	var offset atomic.Int64 // seconds added to the clock
	env.enc.clock = func() time.Time { return now.Add(time.Duration(offset.Load()) * time.Second) }
	if err := env.store.Touch(env.key, now.Add(time.Second)); err != nil { // accessed just now
		t.Fatal(err)
	}
	if !env.enc.encodeNext(context.Background()) {
		t.Fatal("no work done")
	}
	if en := env.entry(t); en.State != StateEncoding || env.enc.pendingSwaps() != 1 {
		t.Fatalf("renamed although accessed under 10 min ago: %s", en.State)
	}
	offset.Store(11 * 60)
	env.enc.swapPending(context.Background())
	if en := env.entry(t); en.State != StateAV1 {
		t.Fatalf("state %s", en.State)
	}
}

func TestEncoderSwapsAtOnceWhenCopyIsIdle(t *testing.T) {
	env := newEncEnv(t, writeOutput(400), EncodeSettings{})
	env.enc.clock = func() time.Time { return time.Now().Add(time.Hour) }
	if !env.enc.encodeNext(context.Background()) {
		t.Fatal("no work done")
	}
	if en := env.entry(t); en.State != StateAV1 || env.enc.pendingSwaps() != 0 || env.reserved() != 0 {
		t.Fatalf("entry %+v pending=%d", en, env.enc.pendingSwaps())
	}
}

// addSecond adds a second ORIGINAL entry (episode 2) next to the one of the env.
func (e *encEnv) addSecond(t *testing.T) Key {
	t.Helper()
	rel := "7/2-vostfr.mkv"
	if err := os.WriteFile(filepath.Join(e.diskDir, rel), e.original, 0o644); err != nil {
		t.Fatal(err)
	}
	k := Key{SeasonID: 7, Episode: 2, Lang: "vostfr"}
	if err := e.store.Create(Entry{Key: k, AnimeID: 1, Title: "T", State: StateOriginal, DiskID: e.pool.Disks()[0].ID, RelPath: rel,
		SizeBytes: origSize, OriginalSizeBytes: origSize, VideoCodec: "h264"}); err != nil {
		t.Fatal(err)
	}
	return k
}

func (e *encEnv) tmpPath() string { return filepath.Join(e.diskDir, "7/1-vostfr.av1.tmp.mkv") }

// parkSwap runs an encode whose swap wait times out because the copy stays in use, leaving it awaiting swap.
func parkSwap(t *testing.T, env *encEnv, busy *atomic.Bool) {
	t.Helper()
	busy.Store(true)
	env.enc.inUse = func(Key) bool { return busy.Load() }
	if !env.enc.encodeNext(context.Background()) {
		t.Fatal("no work done")
	}
}

func TestEncoderSwapTimeoutKeepsTheVerifiedOutput(t *testing.T) {
	env := newEncEnv(t, writeOutput(400), EncodeSettings{})
	second := env.addSecond(t)
	var busy atomic.Bool
	parkSwap(t, env, &busy)
	en := env.entry(t)
	if en.State != StateEncoding || en.Attempts != 0 || en.EncodeSkipped != "" || en.ReservedBytes != 0 {
		t.Fatalf("entry %+v", en)
	}
	if st, err := os.Stat(env.tmpPath()); err != nil || st.Size() != 400 {
		t.Fatalf("verified output not kept: %v %v", st, err)
	}
	if st, _ := os.Stat(env.src); st == nil || st.Size() != origSize {
		t.Fatalf("original touched: %v", st)
	}
	if env.reserved() != 0 {
		t.Fatalf("reservation held while parked (the temp bytes are already on disk): %d", env.reserved())
	}
	if n := env.enc.pendingSwaps(); n != 1 {
		t.Fatalf("pending swaps = %d, want 1", n)
	}
	if env.enc.backoff {
		t.Fatal("encoder backs off instead of moving on to the next entry")
	}
	// The queue is not blocked: the next call encodes the other entry (which parks as well).
	if !env.enc.encodeNext(context.Background()) {
		t.Fatal("encoder did not proceed to the next entry")
	}
	if sec, err := env.store.Get(second); err != nil || sec.State != StateEncoding {
		t.Fatalf("second entry %+v %v", sec, err)
	}
	if n := env.enc.pendingSwaps(); n != 2 {
		t.Fatalf("pending swaps = %d, want 2", n)
	}
}

func TestEncoderPendingSwapCompletesOnceIdle(t *testing.T) {
	now := time.Now()
	env := newEncEnv(t, writeOutput(400), EncodeSettings{})
	var offset atomic.Int64
	env.enc.clock = func() time.Time { return now.Add(time.Duration(offset.Load()) * time.Second) }
	var busy atomic.Bool
	parkSwap(t, env, &busy)
	ctx := context.Background()
	env.enc.swapPending(ctx)
	if env.entry(t).State != StateEncoding || env.enc.pendingSwaps() != 1 {
		t.Fatal("swapped while the copy is in use")
	}
	busy.Store(false)
	if err := env.store.Touch(env.key, now); err != nil { // read just now
		t.Fatal(err)
	}
	env.enc.swapPending(ctx)
	if env.entry(t).State != StateEncoding || env.enc.pendingSwaps() != 1 {
		t.Fatal("swapped although accessed under 10 min ago")
	}
	offset.Store(11 * 60)
	env.enc.swapPending(ctx)
	en := env.entry(t)
	if en.State != StateAV1 || en.SizeBytes != 400 || en.VideoCodec != "av1" || en.Attempts != 0 || en.ReservedBytes != 0 {
		t.Fatalf("entry %+v", en)
	}
	if st, _ := os.Stat(env.src); st == nil || st.Size() != 400 {
		t.Fatalf("final file %v", st)
	}
	env.noTmp(t)
	if env.reserved() != 0 {
		t.Fatalf("reservation leaked: %d", env.reserved())
	}
	if env.enc.pendingSwaps() != 0 {
		t.Fatal("swap still pending")
	}
}

func TestEncoderPendingSwapWithVanishedTempGoesBackToOriginal(t *testing.T) {
	env := newEncEnv(t, writeOutput(400), EncodeSettings{})
	var busy atomic.Bool
	parkSwap(t, env, &busy)
	if err := os.Remove(env.tmpPath()); err != nil {
		t.Fatal(err)
	}
	busy.Store(false)
	env.enc.clock = func() time.Time { return time.Now().Add(time.Hour) }
	env.enc.swapPending(context.Background())
	en := env.entry(t)
	if en.State != StateOriginal || en.Attempts != 0 || en.EncodeSkipped != "" || en.ReservedBytes != 0 {
		t.Fatalf("entry %+v", en)
	}
	if st, _ := os.Stat(env.src); st == nil || st.Size() != origSize {
		t.Fatalf("original touched: %v", st)
	}
	if env.reserved() != 0 || env.enc.pendingSwaps() != 0 {
		t.Fatalf("reserved=%d pending=%d", env.reserved(), env.enc.pendingSwaps())
	}
}

func TestEncoderPendingSwapDroppedWhenEntryDeleted(t *testing.T) {
	env := newEncEnv(t, writeOutput(400), EncodeSettings{})
	var busy atomic.Bool
	parkSwap(t, env, &busy)
	if err := env.store.Delete(env.key); err != nil {
		t.Fatal(err)
	}
	busy.Store(false)
	env.enc.swapPending(context.Background())
	if env.reserved() != 0 || env.enc.pendingSwaps() != 0 {
		t.Fatalf("reserved=%d pending=%d", env.reserved(), env.enc.pendingSwaps())
	}
	env.noTmp(t)
}

func TestEncoderPendingSwapRenameFailureCountsAnAttempt(t *testing.T) {
	env := newEncEnv(t, writeOutput(400), EncodeSettings{})
	if _, err := env.store.Update(env.key, func(en *Entry) error { en.Attempts = maxEncodeAttempts - 2; return nil }); err != nil {
		t.Fatal(err)
	}
	var busy atomic.Bool
	parkSwap(t, env, &busy)
	// Make the rename fail persistently: the original path becomes a non-empty directory.
	if err := os.Remove(env.src); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(env.src, "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	busy.Store(false)
	env.enc.clock = func() time.Time { return time.Now().Add(time.Hour) }
	env.enc.swapPending(context.Background())
	en := env.entry(t)
	if en.State != StateOriginal || en.Attempts != maxEncodeAttempts-1 || en.LastError == "" || en.EncodeSkipped != "" {
		t.Fatalf("entry %+v", en)
	}
	if !env.enc.backoff {
		t.Fatal("no backoff after a failed swap")
	}
	if env.enc.pendingSwaps() != 0 || env.reserved() != 0 {
		t.Fatalf("pending=%d reserved=%d", env.enc.pendingSwaps(), env.reserved())
	}
	env.noTmp(t)
	// The last allowed attempt gives up for good.
	if _, err := env.store.Update(env.key, func(en *Entry) error { en.State = StateEncoding; return nil }); err != nil {
		t.Fatal(err)
	}
	env.enc.park(env.key, pendingSwap{diskID: en.DiskID, src: env.src, tmp: env.tmpPath(), newSize: 400, parkedAt: time.Now()})
	if err := os.WriteFile(env.tmpPath(), make([]byte, 400), 0o644); err != nil {
		t.Fatal(err)
	}
	env.enc.swapPending(context.Background())
	if en := env.entry(t); en.State != StateOriginal || en.Attempts != maxEncodeAttempts || en.EncodeSkipped != "encode_failed" {
		t.Fatalf("entry %+v", en)
	}
}

func TestEncoderPendingSwapNeverReleasesAReservationTwice(t *testing.T) {
	env := newEncEnv(t, writeOutput(400), EncodeSettings{})
	var busy atomic.Bool
	parkSwap(t, env, &busy)
	other, err := env.pool.ReserveOn(env.pool.Disks()[0].ID, 100) // someone else's reservation
	if err != nil {
		t.Fatal(err)
	}
	busy.Store(false)
	env.enc.clock = func() time.Time { return time.Now().Add(time.Hour) }
	env.enc.swapPending(context.Background())
	if en := env.entry(t); en.State != StateAV1 {
		t.Fatalf("state %s", en.State)
	}
	if env.reserved() != other.Size {
		t.Fatalf("reserved = %d, want %d: the swap released a reservation it no longer held", env.reserved(), other.Size)
	}
}

func TestEncoderPendingSwapExpiresAfterADay(t *testing.T) {
	now := time.Now()
	env := newEncEnv(t, writeOutput(400), EncodeSettings{})
	second := env.addSecond(t)
	var offset atomic.Int64
	env.enc.clock = func() time.Time { return now.Add(time.Duration(offset.Load()) * time.Second) }
	var busy atomic.Bool
	parkSwap(t, env, &busy) // still in use for good
	offset.Store(23 * 3600)
	env.enc.swapPending(context.Background())
	if env.entry(t).State != StateEncoding || env.enc.pendingSwaps() != 1 {
		t.Fatal("dropped before 24 h")
	}
	offset.Store(25 * 3600)
	time.Sleep(5 * time.Millisecond)
	env.enc.swapPending(context.Background())
	en := env.entry(t)
	if en.State != StateOriginal || en.Attempts != 0 || en.EncodeSkipped != "" || en.LastError != "swap postponed 24h" || en.ReservedBytes != 0 {
		t.Fatalf("entry %+v", en)
	}
	env.noTmp(t)
	if env.enc.pendingSwaps() != 0 || env.reserved() != 0 {
		t.Fatalf("pending=%d reserved=%d", env.enc.pendingSwaps(), env.reserved())
	}
	// It must not be re-picked ahead of the other entry: the update moved it to the end of the updated_at order.
	list, err := env.store.List(Filter{States: []State{StateOriginal}})
	if err != nil || len(list) != 2 || list[0].Key != second {
		t.Fatalf("order %+v %v", list, err)
	}
}

func TestEncoderPendingSwapWithChangedDiskGoesBackToOriginal(t *testing.T) {
	env := newEncEnv(t, writeOutput(400), EncodeSettings{})
	var busy atomic.Bool
	parkSwap(t, env, &busy)
	if _, err := env.store.Update(env.key, func(en *Entry) error { en.DiskID = "other"; return nil }); err != nil {
		t.Fatal(err)
	}
	env.enc.swapPending(context.Background())
	if en := env.entry(t); en.State != StateOriginal || en.Attempts != 0 {
		t.Fatalf("entry %+v", en)
	}
	env.noTmp(t)
	if env.enc.pendingSwaps() != 0 || env.reserved() != 0 {
		t.Fatalf("pending=%d reserved=%d", env.enc.pendingSwaps(), env.reserved())
	}
}

func TestEncoderRunSwapsPendingBeforeEncodingNext(t *testing.T) {
	env := newEncEnv(t, writeOutput(400), EncodeSettings{})
	var busy atomic.Bool
	parkSwap(t, env, &busy)
	busy.Store(false)
	env.enc.clock = func() time.Time { return time.Now().Add(time.Hour) }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { env.enc.Run(ctx); close(done) }()
	waitFor(t, func() bool { return env.entry(t).State == StateAV1 })
	cancel()
	<-done
}

func TestEncoderCancelWhilePendingKeepsEncodingForRecover(t *testing.T) {
	env := newEncEnv(t, writeOutput(400), EncodeSettings{})
	var busy atomic.Bool
	parkSwap(t, env, &busy)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	env.enc.Run(ctx) // returns at once
	if en := env.entry(t); en.State != StateEncoding {
		t.Fatalf("state %s, want ENCODING kept", en.State)
	}
	if _, err := os.Stat(env.tmpPath()); err != nil {
		t.Fatalf("temp output removed on shutdown: %v", err)
	}
	// After a restart Recover resets it; the temp is then an orphan for the janitor (one re-encode accepted).
	if _, err := env.store.Recover(); err != nil {
		t.Fatal(err)
	}
	if en := env.entry(t); en.State != StateOriginal {
		t.Fatalf("after Recover: %s", en.State)
	}
}

func TestJanitorKeepsPendingSwapTemp(t *testing.T) {
	env := newEncEnv(t, writeOutput(400), EncodeSettings{})
	var busy atomic.Bool
	parkSwap(t, env, &busy)
	j := NewJanitor(env.store, env.pool, nil, nil)
	if _, err := j.Reconcile(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(env.tmpPath()); err != nil {
		t.Fatalf("janitor removed the pending temp: %v", err)
	}
}

func TestEncoderFinishRetriesTheIndexUpdate(t *testing.T) {
	env := newEncEnv(t, writeOutput(400), EncodeSettings{})
	var fails atomic.Int32
	real := env.enc.update
	env.enc.update = func(k Key, fn func(*Entry) error) (Entry, error) {
		if en, _ := env.store.Get(k); en.State == StateEncoding && fails.Add(1) <= 2 {
			return Entry{}, fmt.Errorf("database is locked")
		}
		return real(k, fn)
	}
	env.enc.retryDelay = time.Millisecond
	if !env.enc.encodeNext(context.Background()) {
		t.Fatal("encode reported failure")
	}
	if en := env.entry(t); en.State != StateAV1 {
		t.Fatalf("index left at %s after a transient update failure", en.State)
	}
}

func TestEncoderFreesSpaceWhenReservationFails(t *testing.T) {
	env := newEncEnv(t, writeOutput(400), EncodeSettings{})
	disk := env.entry(t).DiskID
	hold, err := env.pool.ReserveOn(disk, 1<<39) // leaves less than origSize/2 available
	if err != nil {
		t.Fatal(err)
	}
	var asked int64
	env.enc.free = func(d string, need int64) {
		asked = need
		env.pool.Release(hold)
	}
	if !env.enc.encodeNext(context.Background()) {
		t.Fatal("no work done")
	}
	if asked != origSize/2 {
		t.Fatalf("free asked for %d, want %d", asked, origSize/2)
	}
	if en := env.entry(t); en.State != StateAV1 {
		t.Fatalf("state %s", en.State)
	}
}
