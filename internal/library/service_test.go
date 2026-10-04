package library

import (
	"bytes"
	"log/slog"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gazes/gazes/internal/torrent"
)

func newServiceTest(t *testing.T, env *janitorEnv, f Fetcher, now func() time.Time) *Service {
	t.Helper()
	eng := NewEngine(&fakeInner{}, env.store, env.pool, now)
	return &Service{
		store: env.store, pool: env.pool, engine: eng, clock: now,
		acq:     NewAcquirer(env.store, env.pool, f, fakeProber{}, now, time.Hour, 100),
		janitor: NewJanitor(env.store, env.pool, eng.InUse, now),
		rate:    map[int64][]time.Time{},
	}
}

func svcReq(ep int) Request {
	return Request{Key: Key{1, ep, "vf"}, AnimeID: 1, Title: "T", InfoHash: "abc", FileIndex: 0}
}

func TestRegisterRateLimitPerUser(t *testing.T) {
	env := newJanitorEnv(t, nil)
	now := janitorNow
	s := newServiceTest(t, env, &fakeFetcher{data: make([]byte, 10)}, func() time.Time { return now })
	for ep := 1; ep <= 20; ep++ {
		if _, created, err := s.Register(7, svcReq(ep)); err != nil || !created {
			t.Fatalf("copy %d: created=%v err=%v", ep, created, err)
		}
	}
	if _, _, err := s.Register(7, svcReq(21)); err != ErrRateLimited {
		t.Fatalf("21st copy: %v, want ErrRateLimited", err)
	}
	// An existing key is only touched: it neither counts nor is refused.
	if _, created, err := s.Register(7, svcReq(1)); err != nil || created {
		t.Fatalf("repeat: created=%v err=%v", created, err)
	}
	// Another user has their own budget.
	if _, created, err := s.Register(8, svcReq(21)); err != nil || !created {
		t.Fatalf("other user: created=%v err=%v", created, err)
	}
	now = now.Add(61 * time.Minute)
	if _, created, err := s.Register(7, svcReq(22)); err != nil || !created {
		t.Fatalf("after the window: created=%v err=%v", created, err)
	}
}

func TestRegisterRepeatedKeyDoesNotCount(t *testing.T) {
	env := newJanitorEnv(t, nil)
	s := newServiceTest(t, env, &fakeFetcher{data: make([]byte, 10)}, func() time.Time { return janitorNow })
	for i := 0; i < 30; i++ {
		if _, _, err := s.Register(7, svcReq(1)); err != nil {
			t.Fatalf("repeat %d: %v", i, err)
		}
	}
}

func TestRegisterRetriesAfterFreeingSpace(t *testing.T) {
	env := newJanitorEnv(t, nil)
	old := env.add(t, 9, StateAV1, 9950, 48*time.Hour)
	s := newServiceTest(t, env, &fakeFetcher{data: make([]byte, 2000)}, func() time.Time { return janitorNow })
	e, created, err := s.Register(7, svcReq(1))
	if err != nil || !created || e.State != StateDownloading {
		t.Fatalf("register: %+v created=%v err=%v", e, created, err)
	}
	if exists(old) {
		t.Fatal("the least recently used copy should have been evicted")
	}
}

func TestRegisterEvictsWhenFreeSpaceIsOnlyJustAboveTheReserve(t *testing.T) {
	env := newJanitorEnv(t, nil)
	old := env.add(t, 8, StateAV1, 4000, 48*time.Hour)
	keep := env.add(t, 9, StateAV1, 5800, 24*time.Hour) // free 200 > reserve 100, but the new file needs 2000
	s := newServiceTest(t, env, &fakeFetcher{data: make([]byte, 2000)}, func() time.Time { return janitorNow })
	if _, created, err := s.Register(7, svcReq(1)); err != nil || !created {
		t.Fatalf("register: created=%v err=%v", created, err)
	}
	if exists(old) || !exists(keep) {
		t.Fatalf("survivors: old=%v keep=%v", exists(old), exists(keep))
	}
}

func TestRegisterNoSpaceWhenNothingEvictable(t *testing.T) {
	env := newJanitorEnv(t, nil)
	env.add(t, 9, StateAV1, 9950, 10*time.Minute) // protected: accessed less than an hour ago
	s := newServiceTest(t, env, &fakeFetcher{data: make([]byte, 2000)}, func() time.Time { return janitorNow })
	if _, _, err := s.Register(7, svcReq(1)); err != ErrNoSpace {
		t.Fatalf("err = %v, want ErrNoSpace", err)
	}
}

func TestCopiesListsReadyOnly(t *testing.T) {
	env := newJanitorEnv(t, nil)
	env.add(t, 1, StateDownloading, 1, 0)
	env.add(t, 2, StateAV1, 1, 0)
	s := newServiceTest(t, env, &fakeFetcher{}, func() time.Time { return janitorNow })
	got, err := s.Copies(1, 1)
	if err != nil || len(got) != 0 {
		t.Fatalf("downloading copy listed: %v %v", got, err)
	}
	got, err = s.Copies(1, 2)
	if err != nil || len(got) != 1 || got[0].State != "AV1" || len(got[0].StreamID) != 40 {
		t.Fatalf("copies: %+v %v", got, err)
	}
}

type filesInner struct{ fakeInner }

func (*filesInner) Files(h string) ([]torrent.FileInfo, bool) {
	return []torrent.FileInfo{{Index: 0, Path: h + ".mkv", IsVideo: true}}, h == "known"
}

func TestEngineForwardsFiles(t *testing.T) {
	env := newJanitorEnv(t, nil)
	e := NewEngine(&filesInner{}, env.store, env.pool, nil)
	if f, ok := e.Files("known"); !ok || len(f) != 1 || f[0].Path != "known.mkv" {
		t.Fatalf("files: %v %v", f, ok)
	}
	if _, ok := e.Files("other"); ok {
		t.Fatal("unknown torrent reported as loaded")
	}
	plain := NewEngine(&fakeInner{}, env.store, env.pool, nil)
	if _, ok := plain.Files("known"); ok {
		t.Fatal("an inner engine without Files must report not loaded")
	}
}

func TestStateChangesAreLoggedFromTheStoreHook(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	env := newJanitorEnv(t, nil)
	s := newServiceTest(t, env, &fakeFetcher{}, func() time.Time { return janitorNow })
	s.watch()
	set := func(k Key, st State) {
		if _, err := env.store.Update(k, func(e *Entry) error { e.State = st; return nil }); err != nil {
			t.Fatal(err)
		}
	}
	k := Key{1, 1, "vf"}
	if err := env.store.Create(entry(1, 1, "vf", StateDownloading)); err != nil {
		t.Fatal(err)
	}
	set(k, StateOriginal)
	set(k, StateEncoding)
	set(k, StateAV1)
	set(k, StateUnavailable)
	if err := env.store.Delete(Key{1, 1, "vf"}); err != nil {
		t.Fatal(err)
	}
	if err := env.store.Create(entry(1, 2, "vf", StateAV1)); err != nil {
		t.Fatal(err)
	}
	if err := env.store.Delete(Key{1, 2, "vf"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"msg=library.state", "msg=library.encode", "msg=library.disk", "msg=library.evict", "season_id=1", "lang=vf"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in log:\n%s", want, out)
		}
	}
	if regexp.MustCompile(`state=\d`).MatchString(out) {
		t.Errorf("state logged as a number:\n%s", out)
	}
	if !strings.Contains(out, "state=AV1") || !strings.Contains(out, "state=REMOVED") {
		t.Errorf("state names missing:\n%s", out)
	}
	if n := strings.Count(out, "msg=library.encode"); n != 1 {
		t.Errorf("library.encode logged %d times", n)
	}
}
