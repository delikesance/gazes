package torrent

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// cacheEntry describes one resident torrent for eviction decisions.
type cacheEntry struct {
	infoHash string
	bytes    int64
	lastUsed time.Time
	active   bool // a reader still holds a priority window
}

// pickEvictions returns the least recently used idle torrents to drop until the
// resident total fits in max. Active or recently used torrents are never chosen,
// so a cap smaller than the active set is tolerated rather than enforced.
func pickEvictions(entries []cacheEntry, max int64, idle time.Duration, now time.Time) []string {
	var total int64
	candidates := make([]cacheEntry, 0, len(entries))
	for _, entry := range entries {
		total += entry.bytes
		if !entry.active && entry.bytes > 0 && now.Sub(entry.lastUsed) >= idle {
			candidates = append(candidates, entry)
		}
	}
	if max <= 0 || total <= max {
		return nil
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].lastUsed.Before(candidates[j].lastUsed) })
	var drop []string
	for _, entry := range candidates {
		if total <= max {
			break
		}
		drop = append(drop, entry.infoHash)
		total -= entry.bytes
	}
	return drop
}

// removeTorrentData deletes a torrent's payload under dataDir. The name comes
// from untrusted metainfo, so anything that is not a plain child of dataDir is refused.
func removeTorrentData(dataDir, name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return nil
	}
	target := filepath.Join(dataDir, name)
	if rel, err := filepath.Rel(dataDir, target); err != nil || strings.HasPrefix(rel, "..") {
		return nil
	}
	return os.RemoveAll(target)
}
