package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
)

func testMetainfo(t *testing.T) (*metainfo.MetaInfo, string) {
	t.Helper()
	info := metainfo.Info{Name: "episode.mkv", Length: 4, PieceLength: 16384, Pieces: make([]byte, 20)}
	raw, err := bencode.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	mi := &metainfo.MetaInfo{InfoBytes: raw}
	return mi, mi.HashInfoBytes().HexString()
}

func TestMetainfoStoreRoundTrip(t *testing.T) {
	store, err := NewMetainfoStore(filepath.Join(t.TempDir(), "metainfo"))
	if err != nil {
		t.Fatal(err)
	}
	mi, hash := testMetainfo(t)
	if store.Load(hash) != nil {
		t.Fatal("empty store must miss")
	}
	if err := store.Save(hash, mi); err != nil {
		t.Fatal(err)
	}
	got := store.Load(hash)
	if got == nil || got.HashInfoBytes().HexString() != hash {
		t.Fatalf("cached metainfo mismatch: %#v", got)
	}
}

func TestMetainfoStoreRejectsUnsafeNamesAndCorruptFiles(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewMetainfoStore(dir)
	if store.Load("../../etc/passwd") != nil || store.Save("../x", &metainfo.MetaInfo{}) != nil {
		t.Fatal("non-hex info hashes must be ignored")
	}
	hash := "00bb8da13ecd7e3757d700547fde70767c9da603"
	if err := os.WriteFile(filepath.Join(dir, hash+".torrent"), []byte("not bencode"), 0o644); err != nil {
		t.Fatal(err)
	}
	if store.Load(hash) != nil {
		t.Fatal("corrupt entries must miss")
	}
}

func TestMetainfoStorePrunesOldEntries(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewMetainfoStore(dir)
	mi, hash := testMetainfo(t)
	if err := store.Save(hash, mi); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(filepath.Join(dir, hash+".torrent"), old, old); err != nil {
		t.Fatal(err)
	}
	store.Prune(24 * time.Hour)
	if _, err := os.Stat(filepath.Join(dir, hash+".torrent")); !os.IsNotExist(err) {
		t.Fatal("stale entry must be removed")
	}
}
