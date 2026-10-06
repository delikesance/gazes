package torrent

import (
	"testing"

	anacrolix "github.com/anacrolix/torrent"
)

func TestActiveReaders(t *testing.T) {
	e := &ClientEngine{schedulers: map[string]*pieceScheduler{}}
	if n := e.ActiveReaders(); n != 0 {
		t.Fatalf("idle engine: %d", n)
	}
	e.schedulers["a"] = &pieceScheduler{windows: map[*SequentialFileReader]map[int]anacrolix.PiecePriority{{}: {}}}
	e.schedulers["b"] = &pieceScheduler{floors: map[[2]int]struct{}{{0, 4}: {}}}
	e.schedulers["c"] = &pieceScheduler{} // finished: no reader, no floor
	if n := e.ActiveReaders(); n != 2 {
		t.Fatalf("one reader + one floor = 2, got %d", n)
	}
}
