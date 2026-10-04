package library

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

type fakeFS map[string][2]int64 // dir base name -> capacity, free

func (f fakeFS) stat(path string) (int64, int64, error) {
	v, ok := f[filepath.Base(path)]
	if !ok {
		return 0, 0, os.ErrNotExist
	}
	return v[0], v[1], nil
}

func newPoolTest(t *testing.T, fs fakeFS, pct int, bytes int64, dirs ...string) (*Pool, *Store, string) {
	t.Helper()
	root := t.TempDir()
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	for _, d := range dirs {
		if err := os.Mkdir(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := EnsureMarker(filepath.Join(root, d)); err != nil {
			t.Fatal(err)
		}
	}
	return NewPool(root, store, fs.stat, pct, bytes), store, root
}

func TestScanFindsOnlyMarkedDirs(t *testing.T) {
	p, _, root := newPoolTest(t, fakeFS{"a": {1000, 500}, "b": {1000, 500}, "c": {1000, 500}}, 10, 0, "a", "b")
	if err := os.Mkdir(filepath.Join(root, "c"), 0o755); err != nil {
		t.Fatal(err)
	}
	added, removed, err := p.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 2 || len(removed) != 0 || len(p.Disks()) != 2 {
		t.Fatalf("added=%d removed=%d disks=%d", len(added), len(removed), len(p.Disks()))
	}
	for _, d := range p.Disks() {
		if !d.Present || d.ID == "" || (d.Label != "a" && d.Label != "b") || d.Path != filepath.Join(root, d.Label) {
			t.Fatalf("bad disk %+v", d)
		}
	}
}

func TestReservePicksMostFreeAndHonoursReserve(t *testing.T) {
	// reserve is 100 on each disk: available A=200, B=150.
	p, _, root := newPoolTest(t, fakeFS{"A": {1000, 300}, "B": {1000, 250}}, 10, 0, "A", "B")
	if _, _, err := p.Scan(); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Reserve(201); err != ErrNoSpace {
		t.Fatalf("Reserve(201) = %v, want ErrNoSpace", err)
	}
	r1, err := p.Reserve(150)
	if err != nil || r1.Path != filepath.Join(root, "A") {
		t.Fatalf("Reserve(150) = %+v, %v; want disk A", r1, err)
	}
	// A now has 50 left: 100 does not fit there, so it goes to B.
	r2, err := p.Reserve(100)
	if err != nil || r2.Path != filepath.Join(root, "B") {
		t.Fatalf("Reserve(100) = %+v, %v; want disk B", r2, err)
	}
	if _, err := p.Reserve(500); err != ErrNoSpace {
		t.Fatalf("Reserve(500) = %v, want ErrNoSpace", err)
	}
	p.Release(r1)
	if _, err := p.Reserve(200); err != nil {
		t.Fatalf("after Release, Reserve(200): %v", err)
	}
}

func TestConcurrentReservationsDoNotOverbook(t *testing.T) {
	p, _, _ := newPoolTest(t, fakeFS{"A": {1000, 100}}, 0, 0, "A")
	if _, _, err := p.Scan(); err != nil {
		t.Fatal(err)
	}
	var ok atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := p.Reserve(10); err == nil {
				ok.Add(1)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 10 {
		t.Fatalf("%d reservations succeeded, want 10", ok.Load())
	}
}

func TestRemovedDiskMarksEntriesUnavailableAndRestores(t *testing.T) {
	p, store, root := newPoolTest(t, fakeFS{"X": {1000, 500}, "Y": {1000, 500}}, 10, 0, "X")
	if _, _, err := p.Scan(); err != nil {
		t.Fatal(err)
	}
	id := p.Disks()[0].ID
	k := Key{SeasonID: 1, Episode: 1, Lang: "vf"}
	if err := store.Create(Entry{Key: k, State: StateAV1, DiskID: id, RelPath: "1/1-vf.mkv"}); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "X", markerName)
	saved, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	_, removed, err := p.Scan()
	if err != nil || len(removed) != 1 || removed[0].ID != id {
		t.Fatalf("removed=%+v err=%v", removed, err)
	}
	e, _ := store.Get(k)
	if e.State != StateUnavailable || e.PrevState != StateAV1 {
		t.Fatalf("after removal: state=%s prev=%s", e.State, e.PrevState)
	}
	if _, err := p.Path(id, "1/1-vf.mkv"); err != ErrDiskAbsent {
		t.Fatalf("Path on absent disk: %v", err)
	}
	// The same disk comes back under another folder name.
	if err := os.Mkdir(filepath.Join(root, "Y"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Y", markerName), saved, 0o644); err != nil {
		t.Fatal(err)
	}
	added, _, err := p.Scan()
	if err != nil || len(added) != 1 || added[0].Label != "Y" {
		t.Fatalf("added=%+v err=%v", added, err)
	}
	e, _ = store.Get(k)
	if e.State != StateAV1 || e.PrevState != "" {
		t.Fatalf("after return: state=%s prev=%s", e.State, e.PrevState)
	}
}

func TestReserveUsesLargerOfPercentAndBytes(t *testing.T) {
	fs := fakeFS{"A": {1_000_000, 900_000}}
	p, _, _ := newPoolTest(t, fs, 10, 50_000, "A")
	if _, _, err := p.Scan(); err != nil {
		t.Fatal(err)
	}
	if got := p.Disks()[0].Reserve; got != 100_000 {
		t.Fatalf("10%% vs 50000 bytes: reserve = %d, want 100000", got)
	}
	p2, _, _ := newPoolTest(t, fs, 10, 200_000, "A")
	if _, _, err := p2.Scan(); err != nil {
		t.Fatal(err)
	}
	if got := p2.Disks()[0].Reserve; got != 200_000 {
		t.Fatalf("10%% vs 200000 bytes: reserve = %d, want 200000", got)
	}
	fs["A"] = [2]int64{1_000_000, 150_000}
	if got := p2.Under(p2.Disks()[0].ID); got != 50_000 {
		t.Fatalf("Under = %d, want 50000", got)
	}
}

func TestPoolPathRejectsEscape(t *testing.T) {
	p, _, _ := newPoolTest(t, fakeFS{"A": {1000, 500}}, 10, 0, "A")
	if _, _, err := p.Scan(); err != nil {
		t.Fatal(err)
	}
	id := p.Disks()[0].ID
	if _, err := p.Path(id, "../etc/passwd"); err == nil {
		t.Fatal("expected escape to be rejected")
	}
	if got, err := p.Path(id, "1/2-vf.mkv"); err != nil || filepath.Base(got) != "2-vf.mkv" {
		t.Fatalf("Path = %q, %v", got, err)
	}
}
