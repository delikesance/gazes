package torrent_test

import (
	"testing"

	"github.com/gazes/gazes/internal/torrent"
)

func TestCalculateStreamingWindows_StartOfFile(t *testing.T) {
	fileOffset := int64(0)
	pieceLen := int64(2 * 1024 * 1024)  // 2MB pieces
	fileLen := int64(500 * 1024 * 1024) // 500MB file (250 pieces)
	readCursor := int64(0)
	beginPiece := 0
	endPiece := 250
	lookaheadCount := 32
	headerCount := 4

	urgent, lookahead, headers := torrent.CalculateStreamingWindows(
		fileOffset,
		pieceLen,
		fileLen,
		readCursor,
		beginPiece,
		endPiece,
		lookaheadCount,
		headerCount,
	)

	// Urgent: [0, 2)
	if urgent.Begin != 0 || urgent.End != 2 {
		t.Errorf("expected urgent [0, 2), got [%d, %d)", urgent.Begin, urgent.End)
	}

	// Lookahead: [2, 32)
	if lookahead.Begin != 2 || lookahead.End != 32 {
		t.Errorf("expected lookahead [2, 32), got [%d, %d)", lookahead.Begin, lookahead.End)
	}

	// Headers: head [0, 4), tail [246, 250)
	if len(headers) != 2 {
		t.Fatalf("expected 2 header ranges, got %d", len(headers))
	}
	if headers[0].Begin != 0 || headers[0].End != 4 {
		t.Errorf("expected head headers [0, 4), got [%d, %d)", headers[0].Begin, headers[0].End)
	}
	if headers[1].Begin != 246 || headers[1].End != 250 {
		t.Errorf("expected tail headers [246, 250), got [%d, %d)", headers[1].Begin, headers[1].End)
	}
}

func TestCalculateStreamingWindows_MidFileSeek(t *testing.T) {
	fileOffset := int64(10 * 1024 * 1024) // 10MB into multi-file torrent (piece 5)
	pieceLen := int64(2 * 1024 * 1024)    // 2MB pieces
	fileLen := int64(1000 * 1024 * 1024)  // 1GB file
	beginPiece := 5
	endPiece := 505

	// Seek to 200MB into the file: absPos = 210MB -> piece 105
	readCursor := int64(200 * 1024 * 1024)
	lookaheadCount := 30
	headerCount := 4

	urgent, lookahead, _ := torrent.CalculateStreamingWindows(
		fileOffset,
		pieceLen,
		fileLen,
		readCursor,
		beginPiece,
		endPiece,
		lookaheadCount,
		headerCount,
	)

	expectedCurPiece := 105
	if urgent.Begin != expectedCurPiece || urgent.End != expectedCurPiece+2 {
		t.Errorf("expected urgent [%d, %d), got [%d, %d)", expectedCurPiece, expectedCurPiece+2, urgent.Begin, urgent.End)
	}
	if lookahead.Begin != expectedCurPiece+2 || lookahead.End != expectedCurPiece+lookaheadCount {
		t.Errorf("expected lookahead [%d, %d), got [%d, %d)", expectedCurPiece+2, expectedCurPiece+lookaheadCount, lookahead.Begin, lookahead.End)
	}
}

func TestCalculateStreamingWindows_EndOfFileBoundary(t *testing.T) {
	fileOffset := int64(0)
	pieceLen := int64(2 * 1024 * 1024)
	fileLen := int64(20 * 1024 * 1024) // 10 pieces [0, 10)
	beginPiece := 0
	endPiece := 10
	readCursor := int64(19 * 1024 * 1024) // At piece 9

	urgent, lookahead, headers := torrent.CalculateStreamingWindows(
		fileOffset,
		pieceLen,
		fileLen,
		readCursor,
		beginPiece,
		endPiece,
		30,
		4,
	)

	if urgent.Begin != 9 || urgent.End != 10 {
		t.Errorf("expected urgent [9, 10), got [%d, %d)", urgent.Begin, urgent.End)
	}
	if lookahead.End > endPiece {
		t.Errorf("lookahead end %d exceeded endPiece %d", lookahead.End, endPiece)
	}
	if headers[0].End != 4 || headers[1].Begin != 6 {
		t.Errorf("unexpected headers: %+v", headers)
	}
}

func BenchmarkCalculateStreamingWindows(b *testing.B) {
	fileOffset := int64(100 * 1024 * 1024)
	pieceLen := int64(2 * 1024 * 1024)
	fileLen := int64(2000 * 1024 * 1024)
	beginPiece := 50
	endPiece := 1050

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cursor := int64((i % 1000) * 1024 * 1024)
		_, _, _ = torrent.CalculateStreamingWindows(fileOffset, pieceLen, fileLen, cursor, beginPiece, endPiece, 32, 4)
	}
}
