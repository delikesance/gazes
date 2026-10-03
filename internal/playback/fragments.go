package playback

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

type trackClock struct {
	scale uint32
	shift float64
	video bool
}

func atom(typ string, data []byte) []byte {
	out := make([]byte, 8+len(data))
	binary.BigEndian.PutUint32(out, uint32(len(out)))
	copy(out[4:8], typ)
	copy(out[8:], data)
	return out
}

// Remove per-run edit lists and apply their actual timestamp mapping to every
// tfdt instead. All windows then share one init and one transport epoch. The
// one-second transport preroll keeps initial negative decode timestamps legal;
// HLS maps this common epoch to the episode's zero in the VOD playlist.
func normalizeInit(moov []byte) ([]byte, map[uint32]trackClock, error) {
	xs, err := boxes(moov)
	if err != nil {
		return nil, nil, err
	}
	clocks := map[uint32]trackClock{}
	var out []byte
	mvhd := child(moov, "mvhd")
	for _, x := range xs {
		if x.typ != "trak" {
			out = append(out, x.raw...)
			continue
		}
		tkhd := child(x.data, "tkhd")
		pos := 12
		if len(tkhd) > 0 && tkhd[0] == 1 {
			pos = 20
		}
		if len(tkhd) < pos+4 {
			return nil, nil, ErrIndex
		}
		id := binary.BigEndian.Uint32(tkhd[pos:])
		mdia := child(x.data, "mdia")
		scale, _, err := mediaTime(child(mdia, "mdhd"))
		if err != nil {
			return nil, nil, err
		}
		hdlr := child(mdia, "hdlr")
		if len(hdlr) < 12 {
			return nil, nil, ErrIndex
		}
		clocks[id] = trackClock{scale, editShift(x.data, mvhd, scale), string(hdlr[8:12]) == "vide"}
		children, err := boxes(x.data)
		if err != nil {
			return nil, nil, err
		}
		var clean []byte
		for _, c := range children {
			if c.typ != "edts" {
				clean = append(clean, c.raw...)
			}
		}
		out = append(out, atom("trak", clean)...)
	}
	return atom("moov", out), clocks, nil
}

func normalizeMoof(data []byte, clocks map[uint32]trackClock, origin float64) (float64, error) {
	xs, err := boxes(data)
	if err != nil {
		return 0, err
	}
	videoStart := math.Inf(1)
	for _, x := range xs {
		if x.typ != "traf" {
			continue
		}
		hd := child(x.data, "tfhd")
		dt := child(x.data, "tfdt")
		tr := child(x.data, "trun")
		if len(hd) < 8 || len(dt) < 8 || len(tr) < 8 {
			return 0, ErrIndex
		}
		clock, ok := clocks[binary.BigEndian.Uint32(hd[4:])]
		if !ok {
			return 0, ErrIndex
		}
		var raw uint64
		if dt[0] == 1 {
			if len(dt) < 12 {
				return 0, ErrIndex
			}
			raw = binary.BigEndian.Uint64(dt[4:])
		} else {
			raw = uint64(binary.BigEndian.Uint32(dt[4:]))
		}
		flags := uintBE(tr[1:4])
		p := 8
		if flags&1 != 0 {
			p += 4
		}
		if flags&4 != 0 {
			p += 4
		}
		if flags&0x100 != 0 {
			p += 4
		}
		if flags&0x200 != 0 {
			p += 4
		}
		if flags&0x400 != 0 {
			p += 4
		}
		composition := int64(0)
		if flags&0x800 != 0 {
			if len(tr) < p+4 {
				return 0, ErrIndex
			}
			composition = int64(binary.BigEndian.Uint32(tr[p:]))
			if tr[0] == 1 {
				composition = int64(int32(composition))
			}
		}
		if clock.video {
			videoStart = float64(int64(raw)+composition)/float64(clock.scale) + clock.shift - origin
		}
		mapped := math.Round(float64(raw) + (clock.shift-origin+1)*float64(clock.scale))
		if mapped < 0 || mapped > float64(^uint64(0)) {
			return 0, fmt.Errorf("invalid decode timestamp")
		}
		if dt[0] == 1 {
			binary.BigEndian.PutUint64(dt[4:], uint64(mapped))
		} else {
			if mapped > math.MaxUint32 {
				return 0, ErrIndex
			}
			binary.BigEndian.PutUint32(dt[4:], uint32(mapped))
		}
	}
	return videoStart, nil
}

// SplitWindow streams mdat bytes to disk without buffering the whole window.
// Earlier GOPs returned by FFmpeg input seeking are intentionally discarded.
func SplitWindow(src io.Reader, init, media io.Writer, start, end, origin float64) (float64, error) {
	clocks := map[uint32]trackClock{}
	selected := false
	first := math.Inf(1)
	var wrote bool
	for {
		var h [8]byte
		_, err := io.ReadFull(src, h[:])
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}
		size := int64(binary.BigEndian.Uint32(h[:]))
		if size < 8 || size > 1<<31 {
			return 0, ErrIndex
		}
		typ := string(h[4:])
		n := size - 8
		switch typ {
		case "ftyp":
			if _, err = init.Write(h[:]); err != nil {
				return 0, err
			}
			if _, err = io.CopyN(init, src, n); err != nil {
				return 0, err
			}
		case "moov":
			if n > 4<<20 {
				return 0, ErrIndex
			}
			b := make([]byte, n)
			if _, err = io.ReadFull(src, b); err != nil {
				return 0, err
			}
			clean, c, err := normalizeInit(b)
			if err != nil {
				return 0, err
			}
			clocks = c
			if _, err = init.Write(clean); err != nil {
				return 0, err
			}
		case "moof":
			if n > 1<<20 {
				return 0, ErrIndex
			}
			b := make([]byte, n)
			if _, err = io.ReadFull(src, b); err != nil {
				return 0, err
			}
			pts, err := normalizeMoof(b, clocks, origin)
			if err != nil {
				return 0, err
			}
			selected = pts >= start-0.002 && pts < end-0.002
			if selected {
				if first == math.Inf(1) {
					first = pts
				}
				wrote = true
				if _, err = media.Write(h[:]); err != nil {
					return 0, err
				}
				if _, err = media.Write(b); err != nil {
					return 0, err
				}
			}
		case "mdat":
			dst := io.Writer(io.Discard)
			if selected {
				dst = media
				if _, err = dst.Write(h[:]); err != nil {
					return 0, err
				}
			}
			if _, err = io.CopyN(dst, src, n); err != nil {
				return 0, err
			}
		default:
			if _, err = io.CopyN(io.Discard, src, n); err != nil {
				return 0, err
			}
		}
	}
	if !wrote || math.Abs(first-start) > 0.002 {
		return 0, fmt.Errorf("indexed point %.3f is not an output random access point (%.3f)", start, first)
	}
	return first, nil
}
