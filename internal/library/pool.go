package library

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
)

// markerName is the file that makes a sub-directory of the pool root a library disk.
const markerName = ".gazes-library"

var (
	ErrNoSpace    = errors.New("library: no disk has enough free space")
	ErrDiskAbsent = errors.New("library: disk absent")
)

// Disk is one marked directory (usually a mount point) under the pool root.
type Disk struct {
	ID, Label, Path         string
	Capacity, Free, Reserve int64
	Present                 bool
}

// StatFS reports the capacity and free bytes of the filesystem holding path.
type StatFS func(path string) (capacity, free int64, err error)

// SysStatFS is the StatFS backed by syscall.Statfs.
func SysStatFS(path string) (int64, int64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, err
	}
	return int64(st.Blocks) * int64(st.Bsize), int64(st.Bavail) * int64(st.Bsize), nil
}

// Reservation is space set aside on a disk for a pending download or encode.
type Reservation struct {
	DiskID, Path string
	Size         int64
}

// Pool tracks the disks under root and the space reserved on each.
type Pool struct {
	root           string
	store          *Store
	statfs         StatFS
	reservePercent int
	reserveBytes   int64

	mu       sync.Mutex
	disks    map[string]Disk
	reserved map[string]int64
}

func NewPool(root string, store *Store, statfs StatFS, reservePercent int, reserveBytes int64) *Pool {
	return &Pool{root: root, store: store, statfs: statfs, reservePercent: reservePercent, reserveBytes: reserveBytes,
		disks: map[string]Disk{}, reserved: map[string]int64{}}
}

func (p *Pool) reserveFor(capacity int64) int64 {
	r := capacity / 100 * int64(p.reservePercent)
	r += capacity % 100 * int64(p.reservePercent) / 100
	if p.reserveBytes > r {
		r = p.reserveBytes
	}
	return r
}

type markerFile struct {
	DiskID string `json:"disk_id"`
}

func readMarker(dir string) (string, error) {
	b, err := os.ReadFile(filepath.Join(dir, markerName))
	if err != nil {
		return "", err
	}
	var m markerFile
	if err := json.Unmarshal(b, &m); err != nil {
		return "", err
	}
	if m.DiskID == "" {
		return "", errors.New("library: marker without disk_id")
	}
	return m.DiskID, nil
}

func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// EnsureMarker returns the disk id stored in dir's marker, creating the marker with a fresh UUID v4
// if it is missing. An existing but unreadable marker is an error and is never overwritten.
func EnsureMarker(dir string) (string, error) {
	id, err := readMarker(dir)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	id, err = newUUID()
	if err != nil {
		return "", err
	}
	b, _ := json.Marshal(markerFile{DiskID: id})
	f, err := os.OpenFile(filepath.Join(dir, markerName), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) { // lost a race with another creator
			return readMarker(dir)
		}
		return "", err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return "", err
	}
	return id, f.Close()
}

// Scan rediscovers the disks (direct sub-directories of root holding a marker). Entries of vanished
// disks become UNAVAILABLE (remembering their state); entries of returned disks get it back.
// A disk whose marker or filesystem cannot be read is skipped; that only surfaces as an error when
// no disk could be scanned at all.
func (p *Pool) Scan() (added, removed []Disk, err error) {
	ents, err := os.ReadDir(p.root)
	if err != nil {
		return nil, nil, err
	}
	found := map[string]Disk{}
	var firstErr error
	for _, de := range ents {
		dir := filepath.Join(p.root, de.Name())
		if fi, serr := os.Stat(dir); serr != nil || !fi.IsDir() {
			continue
		}
		id, merr := readMarker(dir)
		if merr != nil {
			if !errors.Is(merr, os.ErrNotExist) && firstErr == nil {
				firstErr = fmt.Errorf("library: disk %s: %w", de.Name(), merr)
			}
			continue
		}
		capacity, free, serr := p.statfs(dir)
		if serr != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("library: disk %s: %w", de.Name(), serr)
			}
			continue
		}
		if _, dup := found[id]; dup {
			continue
		}
		found[id] = Disk{ID: id, Label: de.Name(), Path: dir, Capacity: capacity, Free: free, Reserve: p.reserveFor(capacity), Present: true}
	}

	p.mu.Lock()
	for id, d := range found {
		if _, ok := p.disks[id]; !ok {
			added = append(added, d)
		}
	}
	for id, d := range p.disks {
		if _, ok := found[id]; !ok {
			d.Present = false
			removed = append(removed, d)
		}
	}
	p.disks = found
	p.mu.Unlock()
	sortDisks(added)
	sortDisks(removed)

	if len(found) == 0 && firstErr != nil {
		return added, removed, firstErr
	}
	return added, removed, p.syncEntries(found)
}

func sortDisks(d []Disk) { sort.Slice(d, func(i, j int) bool { return d[i].ID < d[j].ID }) }

// syncEntries flags entries whose disk is gone and restores those whose disk is back.
func (p *Pool) syncEntries(present map[string]Disk) error {
	all, err := p.store.List(Filter{})
	if err != nil {
		return err
	}
	var firstErr error
	for _, e := range all {
		if e.DiskID == "" {
			continue
		}
		_, here := present[e.DiskID]
		var fn func(*Entry) error
		switch {
		case !here && e.State != StateUnavailable:
			fn = func(en *Entry) error {
				if en.State == StateUnavailable {
					return nil
				}
				en.PrevState, en.State = en.State, StateUnavailable
				return nil
			}
		case here && e.State == StateUnavailable:
			fn = func(en *Entry) error {
				if en.State != StateUnavailable || en.PrevState == "" {
					return nil
				}
				en.State, en.PrevState = en.PrevState, ""
				return nil
			}
		default:
			continue
		}
		if _, err := p.store.Update(e.Key, fn); err != nil && !errors.Is(err, ErrNotFound) && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Disks returns the present disks ordered by id.
func (p *Pool) Disks() []Disk {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]Disk, 0, len(p.disks))
	for _, d := range p.disks {
		out = append(out, d)
	}
	sortDisks(out)
	return out
}

// available is free space minus the disk reserve minus space already reserved; callers hold p.mu.
func (p *Pool) available(d Disk) (int64, bool) {
	_, free, err := p.statfs(d.Path)
	if err != nil {
		return 0, false
	}
	return free - d.Reserve - p.reserved[d.ID], true
}

// Reserve sets size bytes aside on the present disk with the most available space.
func (p *Pool) Reserve(size int64) (Reservation, error) {
	if size < 0 {
		return Reservation{}, fmt.Errorf("library: negative reservation %d", size)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	var best Disk
	bestAvail := int64(-1)
	for _, d := range p.disks {
		avail, ok := p.available(d)
		if !ok || avail < size {
			continue
		}
		if avail > bestAvail || (avail == bestAvail && d.ID < best.ID) {
			best, bestAvail = d, avail
		}
	}
	if bestAvail < 0 {
		return Reservation{}, ErrNoSpace
	}
	p.reserved[best.ID] += size
	return Reservation{DiskID: best.ID, Path: best.Path, Size: size}, nil
}

// ReserveOn sets size bytes aside on one specific present disk, or fails with ErrNoSpace / ErrDiskAbsent.
func (p *Pool) ReserveOn(diskID string, size int64) (Reservation, error) {
	if size < 0 {
		return Reservation{}, fmt.Errorf("library: negative reservation %d", size)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	d, ok := p.disks[diskID]
	if !ok {
		return Reservation{}, ErrDiskAbsent
	}
	avail, ok := p.available(d)
	if !ok {
		return Reservation{}, ErrDiskAbsent
	}
	if avail < size {
		return Reservation{}, ErrNoSpace
	}
	p.reserved[diskID] += size
	return Reservation{DiskID: diskID, Path: d.Path, Size: size}, nil
}

// Release gives a reservation back. It is safe to call with a zero Reservation.
func (p *Pool) Release(r Reservation) {
	if r.DiskID == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.reserved[r.DiskID] -= r.Size; p.reserved[r.DiskID] <= 0 {
		delete(p.reserved, r.DiskID)
	}
}

// Grow enlarges r by extra bytes on its own disk, or fails with ErrNoSpace / ErrDiskAbsent.
func (p *Pool) Grow(r *Reservation, extra int64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	d, ok := p.disks[r.DiskID]
	if !ok {
		return ErrDiskAbsent
	}
	avail, ok := p.available(d)
	if !ok {
		return ErrDiskAbsent
	}
	if avail < extra {
		return ErrNoSpace
	}
	p.reserved[r.DiskID] += extra
	r.Size += extra
	return nil
}

// Under returns how many bytes must be freed for the disk to be back above its reserve (0 if fine
// or if the disk is absent).
func (p *Pool) Under(diskID string) int64 {
	p.mu.Lock()
	d, ok := p.disks[diskID]
	p.mu.Unlock()
	if !ok {
		return 0
	}
	_, free, err := p.statfs(d.Path)
	if err != nil || free >= d.Reserve {
		return 0
	}
	return d.Reserve - free
}

// Shortfall returns how many bytes must be freed on the disk so that need bytes can be reserved on it:
// the larger of Under and need minus the space currently available for reservation (0 if fine or if the
// disk is absent).
func (p *Pool) Shortfall(diskID string, need int64) int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	d, ok := p.disks[diskID]
	if !ok {
		return 0
	}
	avail, ok := p.available(d)
	if !ok {
		return 0
	}
	short := need - avail
	if _, free, err := p.statfs(d.Path); err == nil && d.Reserve-free > short {
		short = d.Reserve - free
	}
	if short < 0 {
		return 0
	}
	return short
}

// Path resolves rel inside the disk's directory. It rejects paths that escape the disk.
func (p *Pool) Path(diskID, rel string) (string, error) {
	p.mu.Lock()
	d, ok := p.disks[diskID]
	p.mu.Unlock()
	if !ok {
		return "", ErrDiskAbsent
	}
	full := filepath.Join(d.Path, rel)
	if full != d.Path && !strings.HasPrefix(full, d.Path+string(filepath.Separator)) {
		return "", fmt.Errorf("library: path %q escapes the disk", rel)
	}
	return full, nil
}
