package torrent

// PieceRange specifies an interval of piece indices [Begin, End).
type PieceRange struct {
	Begin int
	End   int
}

// CalculateStreamingWindows computes the urgent, lookahead, and header piece ranges
// for a file at a specific read offset.
func CalculateStreamingWindows(
	fileOffset int64,
	pieceLength int64,
	fileLength int64,
	readCursor int64,
	beginPiece int,
	endPiece int,
	lookaheadCount int,
	headerCount int,
) (urgent PieceRange, lookahead PieceRange, headers []PieceRange) {
	if pieceLength <= 0 || endPiece <= beginPiece {
		return PieceRange{beginPiece, beginPiece}, PieceRange{beginPiece, beginPiece}, nil
	}

	if readCursor < 0 {
		readCursor = 0
	}
	if readCursor > fileLength {
		readCursor = fileLength
	}

	absPos := fileOffset + readCursor
	curPiece := int(absPos / pieceLength)

	if curPiece < beginPiece {
		curPiece = beginPiece
	}
	if curPiece >= endPiece {
		curPiece = endPiece - 1
	}

	// 1. Urgent Range (current piece and the next piece; keep startup bandwidth focused)
	urgentBegin := curPiece
	urgentEnd := curPiece + 2
	if urgentEnd > endPiece {
		urgentEnd = endPiece
	}
	urgent = PieceRange{Begin: urgentBegin, End: urgentEnd}

	// 2. Lookahead Range (next N pieces for lookahead buffer)
	lookaheadBegin := urgentEnd
	lookaheadEnd := curPiece + lookaheadCount
	if lookaheadEnd > endPiece {
		lookaheadEnd = endPiece
	}
	if lookaheadBegin < lookaheadEnd {
		lookahead = PieceRange{Begin: lookaheadBegin, End: lookaheadEnd}
	} else {
		lookahead = PieceRange{Begin: lookaheadBegin, End: lookaheadBegin}
	}

	// 3. Header & Tail Ranges (EBML/moov container metadata & cues)
	headEnd := beginPiece + headerCount
	if headEnd > endPiece {
		headEnd = endPiece
	}
	headRange := PieceRange{Begin: beginPiece, End: headEnd}

	tailBegin := endPiece - headerCount
	if tailBegin < beginPiece {
		tailBegin = beginPiece
	}
	tailRange := PieceRange{Begin: tailBegin, End: endPiece}

	headers = []PieceRange{headRange, tailRange}
	return urgent, lookahead, headers
}
