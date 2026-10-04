package torrent

import (
	"context"
	"time"

	anacrolixTorrent "github.com/anacrolix/torrent"
)

const fileVerifyTimeout = 45 * time.Second

// samplePieces picks the first, middle and last piece of [begin,end): enough to notice a payload
// that was deleted or zeroed behind the completion database without hashing the whole file.
func samplePieces(begin, end int) []int {
	if end <= begin {
		return nil
	}
	out := []int{begin}
	for _, p := range []int{begin + (end-begin)/2, end - 1} {
		if p != out[len(out)-1] {
			out = append(out, p)
		}
	}
	return out
}

// verifyFilePieces rehashes sampled pieces the completion database calls complete. When one fails,
// the payload cannot be trusted, so every complete piece of the file is rehashed and the failed
// ones are downloaded again instead of being served as zeros.
func verifyFilePieces(ctx context.Context, t *anacrolixTorrent.Torrent, f *anacrolixTorrent.File) (rejected int, err error) {
	begin, end := f.BeginPieceIndex(), f.EndPieceIndex()
	check := func(pieces []int) (bad int, err error) {
		for _, i := range pieces {
			p := t.Piece(i)
			if !p.State().Complete {
				continue
			}
			if err := p.VerifyDataContext(ctx); err != nil {
				return bad, err
			}
			if !p.State().Complete {
				bad++
			}
		}
		return bad, nil
	}
	bad, err := check(samplePieces(begin, end))
	if err != nil || bad == 0 {
		return bad, err
	}
	all := make([]int, 0, end-begin)
	for i := begin; i < end; i++ {
		all = append(all, i)
	}
	more, err := check(all)
	return bad + more, err
}
