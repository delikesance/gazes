package torrent

import (
	"context"
	"fmt"
	"strings"

	anacrolixTorrent "github.com/anacrolix/torrent"
)

// pinFile marks a file as held by a library download; the torrent stays pinned
// (never evicted) while at least one of its files is.
func (e *ClientEngine) pinFile(infoHash string, fileIndex int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.pinned == nil {
		e.pinned = make(map[string]map[int]bool)
	}
	if e.pinned[infoHash] == nil {
		e.pinned[infoHash] = make(map[int]bool)
	}
	e.pinned[infoHash][fileIndex] = true
}

func (e *ClientEngine) unpinFile(infoHash string, fileIndex int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.pinned[infoHash], fileIndex)
	if len(e.pinned[infoHash]) == 0 {
		delete(e.pinned, infoHash)
	}
}

func (e *ClientEngine) isPinned(infoHash string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.isPinnedLocked(infoHash)
}

// readyFile resolves a file of a torrent whose metadata is already available.
func (e *ClientEngine) readyFile(infoHash string, fileIndex int) (*anacrolixTorrent.Torrent, *anacrolixTorrent.File, error) {
	t, ok := e.getTorrent(infoHash)
	if !ok {
		return nil, nil, fmt.Errorf("torrent with infohash %s not found", infoHash)
	}
	select {
	case <-t.GotInfo():
	default:
		return nil, nil, fmt.Errorf("torrent %s metadata not ready", infoHash)
	}
	files := t.Files()
	if fileIndex < 0 || fileIndex >= len(files) {
		return nil, nil, fmt.Errorf("file index %d out of range (total %d)", fileIndex, len(files))
	}
	return t, files[fileIndex], nil
}

func (e *ClientEngine) schedulerFor(infoHash string, t *anacrolixTorrent.Torrent) *pieceScheduler {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.schedulers == nil {
		e.schedulers = make(map[string]*pieceScheduler)
	}
	s := e.schedulers[infoHash]
	if s == nil {
		s = newPieceScheduler(t)
		e.schedulers[infoHash] = s
	}
	return s
}

// DownloadFile downloads a whole file at PiecePriorityNormal, independent of
// reader windows, and pins its torrent against eviction until ReleaseFile.
func (e *ClientEngine) DownloadFile(infoHash string, fileIndex int) error {
	infoHash = strings.ToLower(infoHash)
	t, f, err := e.readyFile(infoHash, fileIndex)
	if err != nil {
		return err
	}
	e.touch(infoHash)
	e.pinFile(infoHash, fileIndex)
	e.schedulerFor(infoHash, t).setFloor(f.BeginPieceIndex(), f.EndPieceIndex(), true)
	return nil
}

// ReleaseFile removes the download floor and the pin of this file only.
func (e *ClientEngine) ReleaseFile(infoHash string, fileIndex int) {
	infoHash = strings.ToLower(infoHash)
	e.unpinFile(infoHash, fileIndex)
	e.touch(infoHash)
	_, f, err := e.readyFile(infoHash, fileIndex)
	if err != nil {
		return
	}
	e.mu.RLock()
	s := e.schedulers[infoHash]
	e.mu.RUnlock()
	if s != nil {
		s.setFloor(f.BeginPieceIndex(), f.EndPieceIndex(), false)
	}
}

// FileProgress reports the verified bytes and total length of a file.
func (e *ClientEngine) FileProgress(infoHash string, fileIndex int) (completed, length int64, err error) {
	_, f, err := e.readyFile(strings.ToLower(infoHash), fileIndex)
	if err != nil {
		return 0, 0, err
	}
	return f.BytesCompleted(), f.Length(), nil
}

// VerifyFile rehashes every piece of the file and fails if one is rejected.
func (e *ClientEngine) VerifyFile(ctx context.Context, infoHash string, fileIndex int) error {
	t, f, err := e.readyFile(strings.ToLower(infoHash), fileIndex)
	if err != nil {
		return err
	}
	for i := f.BeginPieceIndex(); i < f.EndPieceIndex(); i++ {
		p := t.Piece(i)
		if err := p.VerifyDataContext(ctx); err != nil {
			return err
		}
		if !p.State().Complete {
			return fmt.Errorf("piece %d of file %d rejected by verification", i, fileIndex)
		}
	}
	return nil
}
