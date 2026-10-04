package torrent

import (
	anacrolix "github.com/anacrolix/torrent"
	"sync"
)

// Each reader owns its priority window. Releasing one viewer must not remove
// another viewer's pieces, while abandoned windows must stop downloading.
type pieceScheduler struct {
	mu      sync.Mutex
	torrent *anacrolix.Torrent
	windows map[*SequentialFileReader]map[int]anacrolix.PiecePriority
	applied map[int]anacrolix.PiecePriority
	floors  map[[2]int]struct{} // [begin,end) piece ranges kept at PiecePriorityNormal
	pieces  pieceSetter         // overrides torrent for piece priorities (tests)
}

// pieceSetter is the part of the torrent the scheduler needs to change priorities.
type pieceSetter interface {
	SetPriority(piece int, priority anacrolix.PiecePriority)
}

type torrentPieces struct{ t *anacrolix.Torrent }

func (p torrentPieces) SetPriority(piece int, priority anacrolix.PiecePriority) {
	p.t.Piece(piece).SetPriority(priority)
}

func (s *pieceScheduler) setter() pieceSetter {
	if s.pieces != nil {
		return s.pieces
	}
	return torrentPieces{s.torrent}
}

// setFloor adds or removes a download floor over [begin,end): those pieces keep at least
// PiecePriorityNormal when no reader window covers them.
func (s *pieceScheduler) setFloor(begin, end int, on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := [2]int{begin, end}
	if on {
		if s.floors == nil {
			s.floors = make(map[[2]int]struct{})
		}
		s.floors[key] = struct{}{}
	} else {
		delete(s.floors, key)
	}
	s.apply()
}

func (s *pieceScheduler) update(reader *SequentialFileReader, cursor int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if reader.ctx.Err() != nil {
		return
	}
	file := reader.file
	urgent, lookahead, headers := CalculateStreamingWindows(file.Offset(), s.torrent.Info().PieceLength, file.Length(), cursor, file.BeginPieceIndex(), file.EndPieceIndex(), reader.lookaheadPieces, reader.headerPieces)
	priorities := make(map[int]anacrolix.PiecePriority)
	assign := func(interval PieceRange, priority anacrolix.PiecePriority) {
		for piece := interval.Begin; piece < interval.End; piece++ {
			if priority > priorities[piece] {
				priorities[piece] = priority
			}
		}
	}
	assign(urgent, anacrolix.PiecePriorityNow)
	assign(lookahead, anacrolix.PiecePriorityHigh)
	for _, header := range headers {
		assign(header, anacrolix.PiecePriorityHigh)
	}
	s.windows[reader] = priorities
	s.apply()
}

func (s *pieceScheduler) release(reader *SequentialFileReader) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.windows, reader)
	s.apply()
}

func (s *pieceScheduler) apply() {
	priorities := make(map[int]anacrolix.PiecePriority)
	for floor := range s.floors {
		for piece := floor[0]; piece < floor[1]; piece++ {
			priorities[piece] = anacrolix.PiecePriorityNormal
		}
	}
	for _, window := range s.windows {
		for piece, priority := range window {
			if priority > priorities[piece] {
				priorities[piece] = priority
			}
		}
	}
	set := s.setter()
	for piece := range s.applied {
		if _, ok := priorities[piece]; !ok {
			set.SetPriority(piece, anacrolix.PiecePriorityNone)
		}
	}
	for piece, priority := range priorities {
		if priority != s.applied[piece] {
			set.SetPriority(piece, priority)
		}
	}
	s.applied = priorities
}

func newPieceScheduler(t *anacrolix.Torrent) *pieceScheduler {
	return &pieceScheduler{torrent: t, windows: make(map[*SequentialFileReader]map[int]anacrolix.PiecePriority), applied: make(map[int]anacrolix.PiecePriority)}
}
