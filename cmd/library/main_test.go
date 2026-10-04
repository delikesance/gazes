package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gazes/gazes/internal/library"
)

type env struct {
	index, pool string
	store       *library.Store
}

func setup(t *testing.T) env {
	t.Helper()
	e := env{index: t.TempDir(), pool: t.TempDir()}
	t.Setenv("LIBRARY_INDEX_DIR", e.index)
	t.Setenv("LIBRARY_POOL_DIR", e.pool)
	t.Setenv("LIBRARY_RESERVE_PERCENT", "0")
	t.Setenv("LIBRARY_RESERVE_BYTES", "0")
	s, err := library.OpenStore(e.index)
	if err != nil {
		t.Fatal(err)
	}
	e.store = s
	t.Cleanup(func() { s.Close() })
	return e
}

func (e env) disk(t *testing.T, label string) string {
	t.Helper()
	dir := filepath.Join(e.pool, label)
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	id, err := library.EnsureMarker(dir)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (e env) add(t *testing.T, k library.Key, st library.State, disk string, size, orig int64) {
	t.Helper()
	err := e.store.Create(library.Entry{Key: k, State: st, DiskID: disk, RelPath: "1/" + k.String() + ".mkv", SizeBytes: size, OriginalSizeBytes: orig})
	if err != nil {
		t.Fatal(err)
	}
}

func runCmd(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := run(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestStatusReportsCountsDisksAndSavings(t *testing.T) {
	e := setup(t)
	id := e.disk(t, "d1")
	e.add(t, library.Key{SeasonID: 1, Episode: 1, Lang: "vf"}, library.StateAV1, id, 300, 1000)
	e.add(t, library.Key{SeasonID: 1, Episode: 2, Lang: "vf"}, library.StateAV1, id, 200, 500)
	e.add(t, library.Key{SeasonID: 1, Episode: 3, Lang: "vf"}, library.StateOriginal, id, 900, 900)
	e.store.Close()

	code, out, errs := runCmd("status", "--json")
	if code != 0 {
		t.Fatalf("code %d stderr %s", code, errs)
	}
	var st struct {
		Counts map[string]int `json:"counts"`
		Disks  []struct {
			Label   string `json:"label"`
			DiskID  string `json:"disk_id"`
			Present bool   `json:"present"`
		} `json:"disks"`
		SavedBytes int64 `json:"saved_bytes"`
		Pending    int   `json:"pending_encodes"`
	}
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if st.Counts["AV1"] != 2 || st.Counts["ORIGINAL"] != 1 {
		t.Fatalf("counts %v", st.Counts)
	}
	if st.SavedBytes != (1000+500)-(300+200) {
		t.Fatalf("saved %d", st.SavedBytes)
	}
	if st.Pending != 1 {
		t.Fatalf("pending %d", st.Pending)
	}
	if len(st.Disks) != 1 || st.Disks[0].Label != "d1" || st.Disks[0].DiskID != id || !st.Disks[0].Present {
		t.Fatalf("disks %+v", st.Disks)
	}

	code, out, _ = runCmd("status")
	if code != 0 || !strings.Contains(out, "d1") || !strings.Contains(out, "AV1") {
		t.Fatalf("text status %d: %s", code, out)
	}
}

func TestListFiltersByState(t *testing.T) {
	e := setup(t)
	e.add(t, library.Key{SeasonID: 1, Episode: 1, Lang: "vf"}, library.StateAV1, "", 1, 2)
	e.add(t, library.Key{SeasonID: 2, Episode: 1, Lang: "vostfr"}, library.StateOriginal, "", 1, 1)
	e.store.Close()

	code, out, errs := runCmd("list", "--state", "AV1", "--json")
	if code != 0 {
		t.Fatalf("code %d %s", code, errs)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil || len(rows) != 1 || rows[0]["state"] != "AV1" {
		t.Fatalf("%v %s", err, out)
	}
	_, out, _ = runCmd("list", "--season", "2", "--json")
	rows = nil
	if err := json.Unmarshal([]byte(out), &rows); err != nil || len(rows) != 1 || rows[0]["lang"] != "vostfr" {
		t.Fatalf("%v %s", err, out)
	}
	if code, _, _ := runCmd("list", "--state", "BOGUS"); code != 1 {
		t.Fatalf("bogus state accepted")
	}
}

func TestDeleteRefusesEncodingAndRemovesFile(t *testing.T) {
	e := setup(t)
	id := e.disk(t, "d1")
	enc := library.Key{SeasonID: 1, Episode: 1, Lang: "vf"}
	ok := library.Key{SeasonID: 1, Episode: 2, Lang: "vf"}
	e.add(t, enc, library.StateEncoding, id, 1, 1)
	e.add(t, ok, library.StateAV1, id, 1, 2)
	file := filepath.Join(e.pool, "d1", "1", ok.String()+".mkv")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.store.Close()

	if code, _, errs := runCmd("delete", "1", "1", "vf"); code != 1 || errs == "" {
		t.Fatalf("ENCODING delete: code %d stderr %q", code, errs)
	}
	if code, _, errs := runCmd("delete", "1", "2", "vf"); code != 0 {
		t.Fatalf("delete: %d %s", code, errs)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("file still there: %v", err)
	}
	s, err := library.OpenStore(e.index)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Get(ok); err == nil {
		t.Fatal("entry not deleted")
	}
	if _, err := s.Get(enc); err != nil {
		t.Fatal("encoding entry removed")
	}
}

func TestDeleteRefusesWhenDiskAbsent(t *testing.T) {
	e := setup(t)
	k := library.Key{SeasonID: 1, Episode: 1, Lang: "vf"}
	e.add(t, k, library.StateAV1, "ghost-disk", 1, 2)
	e.store.Close()
	if code, _, errs := runCmd("delete", "1", "1", "vf"); code != 1 || !strings.Contains(errs, "absent") {
		t.Fatalf("code %d stderr %q", code, errs)
	}
	s, _ := library.OpenStore(e.index)
	defer s.Close()
	if _, err := s.Get(k); err != nil {
		t.Fatal("entry deleted despite absent disk")
	}
}
