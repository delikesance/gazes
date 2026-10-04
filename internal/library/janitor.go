package library

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gazes/gazes/internal/diagnostics"
)

const (
	protectWindow     = time.Hour
	janitorFreeEvery  = 5 * time.Minute
	janitorReconEvery = time.Hour
)

// ReconcileReport counts what Reconcile removed.
type ReconcileReport struct{ OrphanFiles, MissingEntries, TempFiles int }

// Janitor frees disk space by evicting least recently used copies and reconciles the index with the disks.
type Janitor struct {
	store *Store
	pool  *Pool
	inUse func(Key) bool
	clock func() time.Time
}

func NewJanitor(store *Store, pool *Pool, inUse func(Key) bool, clock func() time.Time) *Janitor {
	if clock == nil {
		clock = time.Now
	}
	if inUse == nil {
		inUse = func(Key) bool { return false }
	}
	return &Janitor{store: store, pool: pool, inUse: inUse, clock: clock}
}

// Free deletes ORIGINAL/AV1 copies of the disk, least recently accessed first, until the disk is back above
// its reserve. Copies being read, or accessed less than an hour ago, are kept. It returns the bytes freed.
func (j *Janitor) Free(diskID string) (int64, error) {
	if j.pool.Under(diskID) <= 0 {
		return 0, nil
	}
	cands, err := j.store.List(Filter{States: []State{StateOriginal, StateAV1}, DiskID: diskID, OrderBy: "last_access_at"})
	if err != nil {
		return 0, err
	}
	var freed int64
	var firstErr error
	now := j.clock()
	for _, e := range cands {
		if j.pool.Under(diskID) <= 0 {
			break
		}
		if now.Sub(e.LastAccessAt) < protectWindow || j.inUse(e.Key) {
			continue
		}
		path, err := j.pool.Path(e.DiskID, e.RelPath)
		if err != nil {
			continue // disk gone or unsafe path: leave the entry alone
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := j.store.Delete(e.Key); err != nil && !errors.Is(err, ErrNotFound) {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		freed += e.SizeBytes
		diagnostics.Log(context.Background(), slog.LevelInfo, "library.evict",
			"key", e.Key.String(), "disk", diskID, "state", string(e.State), "size_bytes", e.SizeBytes,
			"last_access_at", e.LastAccessAt)
	}
	return freed, firstErr
}

func isTempName(name string) bool {
	return strings.HasSuffix(name, ".tmp") || strings.HasSuffix(name, ".av1.tmp.mkv")
}

func keyRel(k Key) string { return fmt.Sprintf("%d/%d-%s.mkv", k.SeasonID, k.Episode, k.Lang) }

// Reconcile removes files without index entry, orphan temporary files and entries whose file vanished from a
// present disk. Temporary files of DOWNLOADING and ENCODING entries are left alone.
func (j *Janitor) Reconcile() (ReconcileReport, error) {
	var rep ReconcileReport
	var firstErr error
	note := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	// Walk first, list entries second: an entry created while walking is then seen as known.
	type found struct{ disk, rel, full string }
	var files []found
	disks := j.pool.Disks()
	for _, d := range disks {
		note(filepath.WalkDir(d.Path, func(p string, de fs.DirEntry, err error) error {
			if err != nil {
				note(err)
				return nil
			}
			if !de.Type().IsRegular() { // directories, symlinks, devices...
				return nil
			}
			rel, rerr := filepath.Rel(d.Path, p)
			if rerr != nil || rel == markerName {
				return nil
			}
			files = append(files, found{d.ID, filepath.ToSlash(rel), p})
			return nil
		}))
	}

	entries, err := j.store.List(Filter{})
	if err != nil {
		return rep, err
	}
	present := map[string]bool{}
	for _, d := range disks {
		present[d.ID] = true
	}
	known := map[string]bool{}     // disk|rel of final files referenced by an entry
	protected := map[string]bool{} // temp files of active entries
	for _, e := range entries {
		if e.DiskID == "" {
			continue
		}
		rels := []string{e.RelPath, keyRel(e.Key)}
		for _, rel := range rels {
			if rel == "" {
				continue
			}
			known[e.DiskID+"|"+rel] = true
			if e.State == StateDownloading || e.State == StateEncoding {
				protected[e.DiskID+"|"+rel+".tmp"] = true
				protected[e.DiskID+"|"+strings.TrimSuffix(rel, ".mkv")+".av1.tmp.mkv"] = true
			}
		}
	}

	dirs := map[string]bool{}
	for _, f := range files {
		id := f.disk + "|" + f.rel
		switch {
		case isTempName(f.rel):
			if protected[id] {
				continue
			}
			if err := os.Remove(f.full); err != nil {
				note(err)
				continue
			}
			rep.TempFiles++
		case known[id]:
			continue
		default:
			if err := os.Remove(f.full); err != nil {
				note(err)
				continue
			}
			rep.OrphanFiles++
		}
		dirs[filepath.Dir(f.full)] = true
	}
	for _, d := range disks {
		for dir := range dirs {
			if dir != d.Path && strings.HasPrefix(dir, d.Path+string(filepath.Separator)) {
				os.Remove(dir) // only succeeds when empty
			}
		}
	}

	for _, e := range entries {
		if e.DiskID == "" || !present[e.DiskID] || e.RelPath == "" {
			continue
		}
		if e.State != StateOriginal && e.State != StateAV1 {
			continue
		}
		path, err := j.pool.Path(e.DiskID, e.RelPath)
		if err != nil {
			continue
		}
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err := j.store.Delete(e.Key); err != nil && !errors.Is(err, ErrNotFound) {
			note(err)
			continue
		}
		rep.MissingEntries++
	}
	return rep, firstErr
}

// Run frees every disk every 5 minutes and reconciles at start and then every hour, until ctx ends.
func (j *Janitor) Run(ctx context.Context) {
	j.reconcile(ctx)
	free := time.NewTicker(janitorFreeEvery)
	recon := time.NewTicker(janitorReconEvery)
	defer free.Stop()
	defer recon.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-free.C:
			for _, d := range j.pool.Disks() {
				if _, err := j.Free(d.ID); err != nil {
					diagnostics.Log(ctx, slog.LevelWarn, "library.free_failed", "disk", d.ID, "error", err.Error())
				}
			}
		case <-recon.C:
			j.reconcile(ctx)
		}
	}
}

func (j *Janitor) reconcile(ctx context.Context) {
	rep, err := j.Reconcile()
	if err != nil {
		diagnostics.Log(ctx, slog.LevelWarn, "library.reconcile_failed", "error", err.Error())
	}
	if rep != (ReconcileReport{}) {
		diagnostics.Log(ctx, slog.LevelInfo, "library.reconciled", "orphan_files", rep.OrphanFiles,
			"missing_entries", rep.MissingEntries, "temp_files", rep.TempFiles)
	}
}
