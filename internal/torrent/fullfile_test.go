package torrent

import (
	"testing"

	anacrolix "github.com/anacrolix/torrent"
)

type fakePieces struct {
	prio map[int]anacrolix.PiecePriority
}

func (f *fakePieces) SetPriority(i int, p anacrolix.PiecePriority) { f.prio[i] = p }

func newFakeScheduler() (*pieceScheduler, *fakePieces) {
	fp := &fakePieces{prio: map[int]anacrolix.PiecePriority{}}
	return &pieceScheduler{
		pieces:  fp,
		windows: map[*SequentialFileReader]map[int]anacrolix.PiecePriority{},
		applied: map[int]anacrolix.PiecePriority{},
	}, fp
}

func TestSchedulerFloorSurvivesWindowRelease(t *testing.T) {
	s, fp := newFakeScheduler()
	s.setFloor(10, 20, true)
	r := &SequentialFileReader{}
	s.windows[r] = map[int]anacrolix.PiecePriority{12: anacrolix.PiecePriorityHigh, 13: anacrolix.PiecePriorityHigh}
	s.apply()
	if fp.prio[12] != anacrolix.PiecePriorityHigh {
		t.Fatalf("window should win over floor, got %v", fp.prio[12])
	}
	s.release(r)
	for _, p := range []int{12, 13} {
		if fp.prio[p] != anacrolix.PiecePriorityNormal {
			t.Fatalf("piece %d = %v, want Normal", p, fp.prio[p])
		}
	}
}

func TestSchedulerFloorReleaseOnlyAffectsItsRange(t *testing.T) {
	s, fp := newFakeScheduler()
	s.setFloor(0, 10, true)
	s.setFloor(10, 20, true)
	s.setFloor(0, 10, false)
	if fp.prio[5] != anacrolix.PiecePriorityNone {
		t.Fatalf("piece 5 = %v, want None", fp.prio[5])
	}
	if fp.prio[15] != anacrolix.PiecePriorityNormal {
		t.Fatalf("piece 15 = %v, want Normal", fp.prio[15])
	}
}

func TestTorrentStaysPinnedWhileAnyFileIsPinned(t *testing.T) {
	e := &ClientEngine{}
	e.pinFile("aa", 1)
	e.pinFile("aa", 2)
	e.unpinFile("aa", 1)
	if !e.isPinned("aa") {
		t.Fatal("torrent must stay pinned while file 2 is pinned")
	}
	e.unpinFile("aa", 2)
	if e.isPinned("aa") {
		t.Fatal("torrent must be unpinned once no file is pinned")
	}
}

func TestDropTorrentSkipsPinned(t *testing.T) {
	e := &ClientEngine{torrents: map[string]*anacrolix.Torrent{"aa": nil}}
	e.pinFile("aa", 0)
	e.dropTorrent("aa")
	if _, ok := e.torrents["aa"]; !ok || !e.isPinned("aa") {
		t.Fatal("dropTorrent must be a no-op for a pinned torrent")
	}
	e.unpinFile("aa", 0)
	e.dropTorrent("aa")
	if _, ok := e.torrents["aa"]; ok {
		t.Fatal("unpinned torrent should be dropped")
	}
}
