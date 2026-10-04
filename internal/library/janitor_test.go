package library

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

var janitorNow = time.UnixMilli(1700000000000)

type janitorEnv struct {
	j     *Janitor
	store *Store
	pool  *Pool
	root  string
	disk  string
	fs    fakeFS
}

// newJanitorEnv builds one disk "d1" whose free space is capacity minus the bytes of the files it holds,
// with a reserve of 100.
func newJanitorEnv(t *testing.T, inUse func(Key) bool) *janitorEnv {
	t.Helper()
	fsys := fakeFS{"d1": {10000, 0}}
	pool, store, root := newPoolTest(t, fsys, 0, 100, "d1")
	if _, _, err := pool.Scan(); err != nil {
		t.Fatal(err)
	}
	pool.statfs = func(p string) (int64, int64, error) {
		var used int64
		filepath.WalkDir(p, func(f string, de os.DirEntry, err error) error {
			if err == nil && de.Type().IsRegular() && filepath.Base(f) != markerName {
				if fi, e := de.Info(); e == nil {
					used += fi.Size()
				}
			}
			return nil
		})
		return 10000, 10000 - used, nil
	}
	return &janitorEnv{j: NewJanitor(store, pool, inUse, func() time.Time { return janitorNow }), store: store, pool: pool,
		root: filepath.Join(root, "d1"), disk: pool.Disks()[0].ID}
}

func (e *janitorEnv) add(t *testing.T, ep int, state State, size int, access time.Duration) string {
	t.Helper()
	rel := keyRel(Key{1, ep, "vf"})
	full := filepath.Join(e.root, rel)
	os.MkdirAll(filepath.Dir(full), 0o755)
	if err := os.WriteFile(full, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
	en := entry(1, ep, "vf", state)
	en.DiskID, en.RelPath, en.SizeBytes, en.LastAccessAt = e.disk, rel, int64(size), janitorNow.Add(-access)
	if err := e.store.Create(en); err != nil {
		t.Fatal(err)
	}
	return full
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

func TestFreeDeletesLeastRecentlyAccessedFirst(t *testing.T) {
	e := newJanitorEnv(t, nil)
	day := 24 * time.Hour
	f1 := e.add(t, 1, StateOriginal, 3000, 10*day)
	f2 := e.add(t, 2, StateAV1, 3000, 5*day)
	f3 := e.add(t, 3, StateOriginal, 2000, 2*day)
	f4 := e.add(t, 4, StateAV1, 1950, 1*day)
	// free is 50 and the reserve 6000: the two oldest copies (3000 each) must go.
	d := e.pool.disks[e.disk]
	d.Reserve = 6000
	e.pool.disks[e.disk] = d
	freed, err := e.j.Free(e.disk)
	if err != nil {
		t.Fatal(err)
	}
	if freed != 6000 {
		t.Fatalf("freed = %d, want 6000", freed)
	}
	if exists(f1) || exists(f2) || !exists(f3) || !exists(f4) {
		t.Fatalf("wrong survivors: %v %v %v %v", exists(f1), exists(f2), exists(f3), exists(f4))
	}
	if _, err := e.store.Get(Key{1, 1, "vf"}); err != ErrNotFound {
		t.Fatalf("entry 1 still present: %v", err)
	}
	if _, err := e.store.Get(Key{1, 3, "vf"}); err != nil {
		t.Fatal(err)
	}
}

func TestFreeSkipsProtectedCopies(t *testing.T) {
	reading := Key{1, 1, "vf"}
	e := newJanitorEnv(t, func(k Key) bool { return k == reading })
	day := 24 * time.Hour
	f1 := e.add(t, 1, StateOriginal, 3000, 10*day) // being read
	f2 := e.add(t, 2, StateEncoding, 3000, 9*day)  // encoding
	f3 := e.add(t, 3, StateAV1, 3000, 30*time.Minute)
	d := e.pool.disks[e.disk]
	d.Reserve = 5000
	e.pool.disks[e.disk] = d
	freed, err := e.j.Free(e.disk)
	if err != nil || freed != 0 {
		t.Fatalf("freed=%d err=%v", freed, err)
	}
	if !exists(f1) || !exists(f2) || !exists(f3) {
		t.Fatal("protected copy deleted")
	}
}

func TestFreeDropsEntryWhenFileAlreadyMissing(t *testing.T) {
	e := newJanitorEnv(t, nil)
	f1 := e.add(t, 1, StateOriginal, 3000, 10*24*time.Hour)
	e.add(t, 2, StateOriginal, 3000, 9*24*time.Hour)
	os.Remove(f1)
	d := e.pool.disks[e.disk]
	d.Reserve = 8000 // free 7000 after the removal: first candidate is the missing one, still not enough
	e.pool.disks[e.disk] = d
	if _, err := e.j.Free(e.disk); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.Get(Key{1, 1, "vf"}); err != ErrNotFound {
		t.Fatalf("entry with missing file kept: %v", err)
	}
}

func TestReconcileRemovesOrphansAndTemps(t *testing.T) {
	e := newJanitorEnv(t, nil)
	keep := e.add(t, 1, StateOriginal, 10, 0)
	e.add(t, 2, StateOriginal, 10, 0) // entry without file
	os.Remove(filepath.Join(e.root, keyRel(Key{1, 2, "vf"})))
	orphan := filepath.Join(e.root, "9", "9-vf.mkv")
	os.MkdirAll(filepath.Dir(orphan), 0o755)
	os.WriteFile(orphan, []byte("x"), 0o644)
	tmp := filepath.Join(e.root, "1", "5-vf.mkv.tmp")
	av1 := filepath.Join(e.root, "1", "6-vf.av1.tmp.mkv")
	os.WriteFile(tmp, []byte("x"), 0o644)
	os.WriteFile(av1, []byte("x"), 0o644)
	// active entries keep their temp files
	dl := filepath.Join(e.root, "1", "7-vf.mkv.tmp")
	enc := filepath.Join(e.root, "1", "8-vf.av1.tmp.mkv")
	for _, a := range []struct {
		ep    int
		state State
		path  string
	}{{7, StateDownloading, dl}, {8, StateEncoding, enc}} {
		en := entry(1, a.ep, "vf", a.state)
		en.DiskID = e.disk
		if a.state == StateEncoding {
			en.RelPath = keyRel(en.Key)
		}
		if err := e.store.Create(en); err != nil {
			t.Fatal(err)
		}
		os.WriteFile(a.path, []byte("x"), 0o644)
	}
	link := filepath.Join(e.root, "link.mkv")
	os.Symlink(keep, link)

	rep, err := e.j.Reconcile()
	if err != nil {
		t.Fatal(err)
	}
	if rep != (ReconcileReport{OrphanFiles: 1, MissingEntries: 1, TempFiles: 2}) {
		t.Fatalf("report = %+v", rep)
	}
	if exists(orphan) || exists(tmp) || exists(av1) {
		t.Fatal("orphan/temp survived")
	}
	if exists(filepath.Dir(orphan)) {
		t.Fatal("empty season dir kept")
	}
	if !exists(keep) || !exists(dl) || !exists(enc) || !exists(link) || !exists(filepath.Join(e.root, markerName)) {
		t.Fatal("protected file removed")
	}
	if _, err := e.store.Get(Key{1, 2, "vf"}); err != ErrNotFound {
		t.Fatalf("entry without file kept: %v", err)
	}
}

func TestReconcileIgnoresAbsentDisks(t *testing.T) {
	e := newJanitorEnv(t, nil)
	en := entry(1, 1, "vf", StateUnavailable)
	en.PrevState, en.DiskID, en.RelPath = StateOriginal, "gone-disk", "1/1-vf.mkv"
	if err := e.store.Create(en); err != nil {
		t.Fatal(err)
	}
	en2 := entry(1, 2, "vf", StateOriginal)
	en2.DiskID, en2.RelPath = "gone-disk", "1/2-vf.mkv"
	if err := e.store.Create(en2); err != nil {
		t.Fatal(err)
	}
	rep, err := e.j.Reconcile()
	if err != nil || rep.MissingEntries != 0 {
		t.Fatalf("rep=%+v err=%v", rep, err)
	}
	if _, err := e.store.Get(en.Key); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.Get(en2.Key); err != nil {
		t.Fatal(err)
	}
}

func TestReconcileAndFreeSkipDiskWithoutMarker(t *testing.T) {
	e := newJanitorEnv(t, nil)
	e.add(t, 1, StateOriginal, 3000, 10*24*time.Hour)
	orphan := filepath.Join(e.root, "9", "9-vf.mkv")
	os.MkdirAll(filepath.Dir(orphan), 0o755)
	os.WriteFile(orphan, []byte("x"), 0o644)
	d := e.pool.disks[e.disk]
	d.Reserve = 9000
	e.pool.disks[e.disk] = d
	os.Remove(filepath.Join(e.root, markerName)) // disk unmounted: empty/foreign directory
	os.Remove(filepath.Join(e.root, keyRel(Key{1, 1, "vf"})))
	rep, err := e.j.Reconcile()
	if err != nil || rep != (ReconcileReport{}) {
		t.Fatalf("rep=%+v err=%v", rep, err)
	}
	if freed, err := e.j.Free(e.disk); err != nil || freed != 0 {
		t.Fatalf("freed=%d err=%v", freed, err)
	}
	if !exists(orphan) {
		t.Fatal("file deleted on unverified disk")
	}
	if _, err := e.store.Get(Key{1, 1, "vf"}); err != nil {
		t.Fatalf("entry deleted on unverified disk: %v", err)
	}
}

func TestReconcileLeavesForeignFilesAlone(t *testing.T) {
	e := newJanitorEnv(t, nil)
	foreign := []string{"lost+found/x", ".DS_Store", "notes.tmp", "1/readme.txt", "1/2-fr.mkv", "1/2-vf.mkv.bak", "1/other.tmp", "a/1-vf.mkv", "1/2-vf.av1.tmp.mkv.old"}
	for _, f := range foreign {
		p := filepath.Join(e.root, f)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("x"), 0o644)
	}
	rep, err := e.j.Reconcile()
	if err != nil || rep != (ReconcileReport{}) {
		t.Fatalf("rep=%+v err=%v", rep, err)
	}
	for _, f := range foreign {
		if !exists(filepath.Join(e.root, f)) {
			t.Fatalf("foreign file %s deleted", f)
		}
	}
}

func TestFreeRechecksCandidateBeforeDeleting(t *testing.T) {
	for name, mutate := range map[string]func(*Janitor, Key){
		"encoding": func(j *Janitor, k Key) {
			j.store.Update(k, func(en *Entry) error { en.State = StateEncoding; return nil })
		},
		"touched": func(j *Janitor, k Key) { j.store.Touch(k, janitorNow) },
	} {
		t.Run(name, func(t *testing.T) {
			e := newJanitorEnv(t, nil)
			f1 := e.add(t, 1, StateOriginal, 3000, 10*24*time.Hour)
			d := e.pool.disks[e.disk]
			d.Reserve = 5000
			e.pool.disks[e.disk] = d
			e.j.beforeEvict = func(k Key) { mutate(e.j, k) }
			freed, err := e.j.Free(e.disk)
			if err != nil || freed != 0 || !exists(f1) {
				t.Fatalf("freed=%d err=%v exists=%v", freed, err, exists(f1))
			}
			if _, err := e.store.Get(Key{1, 1, "vf"}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
