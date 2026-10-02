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
	for _, window := range s.windows {
		for piece, priority := range window {
			if priority > priorities[piece] {
				priorities[piece] = priority
			}
		}
	}
	for piece := range s.applied {
		if _, ok := priorities[piece]; !ok {
			s.torrent.Piece(piece).SetPriority(anacrolix.PiecePriorityNone)
		}
	}
	for piece, priority := range priorities {
		if priority != s.applied[piece] {
			s.torrent.Piece(piece).SetPriority(priority)
		}
	}
	s.applied = priorities
}
