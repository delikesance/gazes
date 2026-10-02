package torrent

import (
	"context"
	"io"
	"sync"

	anacrolixTorrent "github.com/anacrolix/torrent"
)

// SequentialFileReader wraps anacrolix torrent.Reader with sliding window lookahead optimizations.
type SequentialFileReader struct {
	reader          anacrolixTorrent.Reader
	ctx             context.Context
	scheduler       *pieceScheduler
	stopRelease     func() bool
	closeOnce       sync.Once
	closeError      error
	file            *anacrolixTorrent.File
	fileInfo        FileInfo
	currentOffset   int64
	lastPieceIndex  int
	lookaheadPieces int
	headerPieces    int
}

// NewSequentialFileReader creates a responsive, readahead-enabled reader for a specific torrent file.
func NewSequentialFileReader(ctx context.Context, scheduler *pieceScheduler, file *anacrolixTorrent.File, info FileInfo, readaheadBytes int64, lookaheadPieces int, headerPieces int) *SequentialFileReader {
	if lookaheadPieces <= 0 {
		lookaheadPieces = 32
	}
	if headerPieces <= 0 {
		headerPieces = 1
	}

	r := file.NewReader()
	r.SetContext(ctx)
	r.SetResponsive()
	if readaheadBytes > 0 {
		r.SetReadahead(readaheadBytes)
	}

	s := &SequentialFileReader{
		reader:          r,
		ctx:             ctx,
		scheduler:       scheduler,
		file:            file,
		fileInfo:        info,
		currentOffset:   0,
		lastPieceIndex:  -1,
		lookaheadPieces: lookaheadPieces,
		headerPieces:    headerPieces,
	}

	// Immediate high-priority lookahead for piece 0
	s.triggerPrioritization(0)
	s.stopRelease = context.AfterFunc(ctx, func() { scheduler.release(s) })

	return s
}

func (s *SequentialFileReader) triggerPrioritization(cursor int64) {
	t := s.file.Torrent()
	if t == nil || t.Info() == nil {
		return
	}

	pieceLen := t.Info().PieceLength
	if pieceLen <= 0 {
		return
	}

	absPos := s.file.Offset() + cursor
	curPiece := int(absPos / pieceLen)

	if curPiece != s.lastPieceIndex {
		s.lastPieceIndex = curPiece
		s.scheduler.update(s, cursor)
	}
}

// Read reads data sequentially from the torrent piece buffer.
func (s *SequentialFileReader) Read(p []byte) (n int, err error) {
	return s.ReadContext(s.ctx, p)
}

// ReadContext bounds metadata probes independently of the stream lifetime.
func (s *SequentialFileReader) ReadContext(ctx context.Context, p []byte) (n int, err error) {
	n, err = s.reader.ReadContext(ctx, p)
	if n > 0 {
		s.currentOffset += int64(n)
		s.triggerPrioritization(s.currentOffset)
	}
	return n, err
}

// Seek moves the read cursor and shifts the responsive piece download window.
func (s *SequentialFileReader) Seek(offset int64, whence int) (int64, error) {
	newPos, err := s.reader.Seek(offset, whence)
	if err == nil {
		s.currentOffset = newPos
		s.lastPieceIndex = -1 // Force reprioritization on seek
		s.triggerPrioritization(newPos)
	}
	return newPos, err
}

// Close closes the underlying torrent reader.
func (s *SequentialFileReader) Close() error {
	s.closeOnce.Do(func() {
		s.stopRelease()
		s.scheduler.release(s)
		s.closeError = s.reader.Close()
	})
	return s.closeError
}

// Verify interface compliance
var _ io.ReadSeekCloser = (*SequentialFileReader)(nil)
