package torrent

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestPickEvictionsDropsOldestIdleFirst(t *testing.T) {
	now := time.Now()
	entries := []cacheEntry{
		{infoHash: "recent", bytes: 10, lastUsed: now.Add(-time.Minute)},
		{infoHash: "oldest", bytes: 10, lastUsed: now.Add(-3 * time.Hour)},
		{infoHash: "active", bytes: 10, lastUsed: now.Add(-5 * time.Hour), active: true},
		{infoHash: "older", bytes: 10, lastUsed: now.Add(-2 * time.Hour)},
	}
	got := pickEvictions(entries, 25, 10*time.Minute, now)
	if want := []string{"oldest", "older"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestPickEvictionsNoopUnderCapOrWithoutIdleCandidates(t *testing.T) {
	now := time.Now()
	entries := []cacheEntry{{infoHash: "a", bytes: 10, lastUsed: now}, {infoHash: "b", bytes: 10, lastUsed: now.Add(-time.Hour), active: true}}
	if got := pickEvictions(entries, 100, time.Minute, now); got != nil {
		t.Fatalf("under cap: %v", got)
	}
	if got := pickEvictions(entries, 5, time.Minute, now); got != nil {
		t.Fatalf("nothing idle to evict: %v", got)
	}
	if got := pickEvictions(entries, 0, time.Minute, now); got != nil {
		t.Fatalf("zero cap disables eviction: %v", got)
	}
}

func TestRemoveTorrentDataStaysInsideDataDir(t *testing.T) {
	dir := t.TempDir()
	inside := filepath.Join(dir, "pack")
	outside := filepath.Join(filepath.Dir(dir), "keep-"+filepath.Base(dir))
	for _, path := range []string{inside, outside} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	defer os.RemoveAll(outside)
	for _, name := range []string{"..", "../" + filepath.Base(outside), "a/b", ""} {
		if err := removeTorrentData(dir, name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatal("path outside the data dir must survive")
	}
	if err := removeTorrentData(dir, "pack"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(inside); !os.IsNotExist(err) {
		t.Fatal("torrent payload must be removed")
	}
}

func TestPickEvictionsSkipsPinned(t *testing.T) {
	now := time.Now()
	entries := []cacheEntry{
		{infoHash: "old", bytes: 1000, lastUsed: now.Add(-48 * time.Hour), pinned: true},
		{infoHash: "new", bytes: 10, lastUsed: now.Add(-2 * time.Hour)},
	}
	got := pickEvictions(entries, 5, time.Hour, now)
	if !reflect.DeepEqual(got, []string{"new"}) {
		t.Fatalf("got %v, want only the unpinned candidate", got)
	}
}
