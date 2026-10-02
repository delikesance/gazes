// Package cache holds the small on-disk caches that let the torrent engine skip
// work it has already done: resolved .torrent metadata survives restarts, so a
// previously seen source starts without another DHT/tracker metadata round trip.
package cache

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/anacrolix/torrent/metainfo"
)

var infoHashPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

// MetainfoStore keeps one bencoded .torrent file per info hash.
type MetainfoStore struct {
	dir string
}

// NewMetainfoStore creates the directory when needed.
func NewMetainfoStore(dir string) (*MetainfoStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &MetainfoStore{dir: dir}, nil
}

func (s *MetainfoStore) path(infoHash string) (string, bool) {
	infoHash = strings.ToLower(infoHash)
	if !infoHashPattern.MatchString(infoHash) {
		return "", false
	}
	return filepath.Join(s.dir, infoHash+".torrent"), true
}

// Load returns the cached metainfo, or nil when absent or unreadable.
func (s *MetainfoStore) Load(infoHash string) *metainfo.MetaInfo {
	path, ok := s.path(infoHash)
	if !ok {
		return nil
	}
	mi, err := metainfo.LoadFromFile(path)
	if err != nil {
		return nil
	}
	if _, err := mi.UnmarshalInfo(); err != nil {
		_ = os.Remove(path)
		return nil
	}
	now := time.Now()
	_ = os.Chtimes(path, now, now) // keep recently used entries out of Prune
	return mi
}

// Save writes the metainfo atomically; errors are not fatal for streaming.
func (s *MetainfoStore) Save(infoHash string, mi *metainfo.MetaInfo) error {
	path, ok := s.path(infoHash)
	if !ok || mi == nil {
		return nil
	}
	var buf bytes.Buffer
	if err := mi.Write(&buf); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, ".tmp-*")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(buf.Bytes())
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		_ = os.Remove(tmp.Name())
		if werr != nil {
			return werr
		}
		return cerr
	}
	return os.Rename(tmp.Name(), path)
}

// Prune removes entries not used for maxAge.
func (s *MetainfoStore) Prune(maxAge time.Duration) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-maxAge)
	for _, entry := range entries {
		info, err := entry.Info()
		if err == nil && strings.HasSuffix(entry.Name(), ".torrent") && info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(s.dir, entry.Name()))
		}
	}
}
