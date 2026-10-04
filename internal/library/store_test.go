package library

import (
	"bytes"
	"errors"
	"regexp"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func openTest(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, dir
}

func entry(season, ep int, lang string, state State) Entry {
	return Entry{Key: Key{season, ep, lang}, AnimeID: 7, Title: "T", State: state}
}

func TestCreateGetAndUniqueKey(t *testing.T) {
	s, _ := openTest(t)
	e := entry(1, 2, "vf", StateOriginal)
	e.InfoHash, e.FileIndex, e.SizeBytes, e.AudioTracks = "abc", 3, 1234, 2
	e.LastAccessAt = time.UnixMilli(1700000000123)
	if err := s.Create(e); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(e.Key)
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != e.Key || got.InfoHash != "abc" || got.FileIndex != 3 || got.SizeBytes != 1234 || got.AudioTracks != 2 || got.State != StateOriginal {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	if !got.LastAccessAt.Equal(e.LastAccessAt) || got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("dates wrong: %+v", got)
	}
	if err := s.Create(e); !errors.Is(err, ErrExists) {
		t.Fatalf("want ErrExists, got %v", err)
	}
	if _, err := s.Get(Key{9, 9, "vf"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if err := s.Delete(e.Key); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(e.Key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound after delete, got %v", err)
	}
}

func TestUpdateIsTransactionalAndStampsUpdatedAt(t *testing.T) {
	s, _ := openTest(t)
	e := entry(1, 1, "vostfr", StateOriginal)
	e.UpdatedAt = time.UnixMilli(1000)
	if err := s.Create(e); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("boom")
	if _, err := s.Update(e.Key, func(x *Entry) error { x.State = StateAV1; return boom }); !errors.Is(err, boom) {
		t.Fatalf("want boom, got %v", err)
	}
	got, _ := s.Get(e.Key)
	if got.State != StateOriginal || got.UpdatedAt.UnixMilli() != 1000 {
		t.Fatalf("failed update leaked: %+v", got)
	}
	upd, err := s.Update(e.Key, func(x *Entry) error { x.State = StateEncoding; x.PrevState = StateOriginal; return nil })
	if err != nil {
		t.Fatal(err)
	}
	got, _ = s.Get(e.Key)
	if got.State != StateEncoding || got.PrevState != StateOriginal || got.UpdatedAt.UnixMilli() <= 1000 || !got.UpdatedAt.Equal(upd.UpdatedAt) {
		t.Fatalf("update not applied: %+v", got)
	}
	if _, err := s.Update(Key{5, 5, "vf"}, func(*Entry) error { return nil }); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestListFiltersAndOrdersByLastAccess(t *testing.T) {
	s, _ := openTest(t)
	mk := func(ep int, state State, disk string, access int64) {
		e := entry(1, ep, "vf", state)
		e.DiskID = disk
		e.LastAccessAt = time.UnixMilli(access)
		if err := s.Create(e); err != nil {
			t.Fatal(err)
		}
	}
	mk(1, StateAV1, "d1", 300)
	mk(2, StateAV1, "d1", 100)
	mk(3, StateOriginal, "d1", 200)
	mk(4, StateAV1, "d2", 50)
	got, err := s.List(Filter{States: []State{StateAV1}, DiskID: "d1", OrderBy: "last_access_at"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Episode != 2 || got[1].Episode != 1 {
		t.Fatalf("unexpected list: %+v", got)
	}
	all, _ := s.List(Filter{OrderBy: "last_access_at", Limit: 2})
	if len(all) != 2 || all[0].Episode != 4 || all[1].Episode != 2 {
		t.Fatalf("limit/order wrong: %+v", all)
	}
	one, _ := s.List(Filter{SeasonID: 1, Episode: 3})
	if len(one) != 1 || one[0].Episode != 3 {
		t.Fatalf("episode filter wrong: %+v", one)
	}
	if _, err := s.List(Filter{OrderBy: "title; DROP"}); err == nil {
		t.Fatal("bad OrderBy accepted")
	}
	if err := s.Touch(Key{1, 4, "vf"}, time.UnixMilli(999)); err != nil {
		t.Fatal(err)
	}
	g, _ := s.Get(Key{1, 4, "vf"})
	if g.LastAccessAt.UnixMilli() != 999 {
		t.Fatalf("touch failed: %+v", g)
	}
}

func TestRecoverResetsEncodingAndReservations(t *testing.T) {
	s, _ := openTest(t)
	a := entry(1, 1, "vf", StateEncoding)
	a.ReservedBytes = 500
	b := entry(1, 2, "vf", StateDownloading)
	b.ReservedBytes = 900
	c := entry(1, 3, "vf", StateAV1)
	c.ReservedBytes = 10
	for _, e := range []Entry{a, b, c} {
		if err := s.Create(e); err != nil {
			t.Fatal(err)
		}
	}
	rep, err := s.Recover()
	if err != nil {
		t.Fatal(err)
	}
	if rep.Encoding != 1 || rep.Downloading != 1 {
		t.Fatalf("report: %+v", rep)
	}
	for ep, want := range map[int]State{1: StateOriginal, 2: StateDownloading, 3: StateAV1} {
		g, _ := s.Get(Key{1, ep, "vf"})
		if g.State != want || g.ReservedBytes != 0 {
			t.Fatalf("ep %d: %+v", ep, g)
		}
	}
}

func TestStreamIDIsStableHexAndResolvable(t *testing.T) {
	s, dir := openTest(t)
	k := Key{4, 5, "vostfr"}
	id, err := s.StreamID(k)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(id) {
		t.Fatalf("bad id %q", id)
	}
	if err := s.Create(entry(4, 5, "vostfr", StateAV1)); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s2, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	id2, _ := s2.StreamID(k)
	if id2 != id {
		t.Fatalf("id not stable: %s vs %s", id, id2)
	}
	e, err := s2.ByStreamID(id)
	if err != nil || e.Key != k {
		t.Fatalf("ByStreamID: %+v %v", e, err)
	}
	if _, err := s2.ByStreamID("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	other, _ := s2.StreamID(Key{4, 5, "vf"})
	if other == id {
		t.Fatal("distinct keys share an id")
	}
}

func TestConcurrentCreateSameKeyOnlyOneWins(t *testing.T) {
	s, _ := openTest(t)
	var ok, exists atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			switch err := s.Create(entry(1, 1, "vf", StateDownloading)); {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, ErrExists):
				exists.Add(1)
			default:
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 1 || exists.Load() != 9 {
		t.Fatalf("ok=%d exists=%d", ok.Load(), exists.Load())
	}
}

func TestEncoderStatusRoundTrip(t *testing.T) {
	s, _ := openTest(t)
	if st, err := s.EncoderStatus(); err != nil || st.Key != nil {
		t.Fatalf("empty status: %+v %v", st, err)
	}
	k := Key{1, 2, "vf"}
	if err := s.SetEncoderStatus(EncoderStatus{Key: &k, Progress: 0.5, Paused: true, UpdatedAt: time.UnixMilli(42)}); err != nil {
		t.Fatal(err)
	}
	st, _ := s.EncoderStatus()
	if st.Key == nil || *st.Key != k || st.Progress != 0.5 || !st.Paused || st.UpdatedAt.UnixMilli() != 42 {
		t.Fatalf("status: %+v", st)
	}
	_ = s.SetEncoderStatus(EncoderStatus{})
	if st, _ = s.EncoderStatus(); st.Key != nil {
		t.Fatalf("key not cleared: %+v", st)
	}
}

func TestOnChangeSeesCreateUpdateDeleteInOrder(t *testing.T) {
	s, _ := openTest(t)
	type ev struct{ old, new State }
	var got []ev
	s.SetOnChange(func(old, cur *Entry) {
		e := ev{}
		if old != nil {
			e.old = old.State
		}
		if cur != nil {
			e.new = cur.State
		}
		got = append(got, e)
	})
	k := Key{1, 1, "vf"}
	if err := s.Create(entry(1, 1, "vf", StateDownloading)); err != nil {
		t.Fatal(err)
	}
	if err := s.Create(entry(1, 1, "vf", StateDownloading)); err != ErrExists {
		t.Fatal(err)
	}
	if _, err := s.Update(k, func(e *Entry) error { e.State = StateOriginal; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(k, func(e *Entry) error { return errors.New("no") }); err == nil {
		t.Fatal("expected failure")
	}
	if err := s.Delete(k); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(k); err != nil { // already gone: no callback
		t.Fatal(err)
	}
	want := []ev{{"", StateDownloading}, {StateDownloading, StateOriginal}, {StateOriginal, ""}}
	if len(got) != len(want) {
		t.Fatalf("callbacks = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("callbacks = %v, want %v", got, want)
		}
	}
}

func TestSecretIsCachedAndNotRewritten(t *testing.T) {
	s, _ := openTest(t)
	first, err := s.Secret()
	if err != nil || len(first) != 32 {
		t.Fatalf("secret: %v %v", len(first), err)
	}
	// Remove the persisted row behind the store's back: a cached Secret must not touch the table again.
	if _, err := s.db.Exec(`DELETE FROM meta WHERE key = 'secret'`); err != nil {
		t.Fatal(err)
	}
	again, err := s.Secret()
	if err != nil || !bytes.Equal(first, again) {
		t.Fatalf("secret changed: %v", err)
	}
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM meta WHERE key = 'secret'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("Secret wrote to the index again (rows=%d err=%v)", n, err)
	}
}
