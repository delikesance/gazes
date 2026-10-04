package library

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
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

func TestEncoderPausesWhenBusyAndOutsideWindow(t *testing.T) {
	for _, tc := range []string{"streams", "window"} {
		t.Run(tc, func(t *testing.T) {
			s := EncodeSettings{}
			var active atomic.Int32
			var outside atomic.Bool
			if tc == "streams" {
				s.PauseStreams = 3
				active.Store(3)
			} else {
				s.Window = "01:00-09:00"
				outside.Store(true)
			}
			gate := filepath.Join(t.TempDir(), "go")
			env := newEncEnv(t, fmt.Sprintf("while [ ! -f %q ]; do sleep 0.02; done\n%s", gate, writeOutput(400)), s)
			env.enc.active = func() int { return int(active.Load()) }
			env.enc.clock = func() time.Time {
				if outside.Load() {
					return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
				}
				return time.Date(2026, 1, 1, 2, 0, 0, 0, time.UTC)
			}
			rec := &sigRecorder{}
			env.enc.signal = rec.send

			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() { env.enc.Run(ctx); close(done) }()
			defer func() { cancel(); <-done }()

			waitFor(t, func() bool {
				st, _ := env.store.EncoderStatus()
				return st.Paused && st.Key != nil && *st.Key == env.key
			})
			if rec.last() != syscall.SIGSTOP {
				t.Fatalf("last signal %v, want SIGSTOP", rec.last())
			}
			active.Store(0)
			outside.Store(false)
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
