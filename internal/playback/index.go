// Package playback owns the episode timeline and bounded, on-demand HLS media.
package playback

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
)

var ErrIndex = errors.New("container has no usable bounded seek index")

const indexBudget = 16 << 20

type Point struct {
	Time   float64
	Offset int64
}
type Segment struct {
	Start, End float64
	Offset     int64
}
type SubtitleCue struct { Start, Duration float64 }
type Index struct {
	Origin, Duration float64
	Points           []Point
	Segments         []Segment
	Subtitles        map[int][]SubtitleCue
}

type boundedReader struct {
	ctx  context.Context
	r    io.ReadSeeker
	left int64
	size int64
}

func (r *boundedReader) at(pos int64, n int) ([]byte, error) {
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}
	if n < 0 || int64(n) > r.left || pos < 0 || pos > r.size-int64(n) {
		return nil, ErrIndex
	}
	r.left -= int64(n)
	if _, err := r.r.Seek(pos, io.SeekStart); err != nil {
		return nil, err
	}
	b := make([]byte, n)
	_, err := io.ReadFull(r.r, b)
	return b, err
}

// ReadIndex reads only container metadata. It never discovers keyframes by a
// linear packet scan; missing indexes are an explicit source failure.
func ReadIndex(ctx context.Context, r io.ReadSeeker, size int64) (*Index, error) {
	br := &boundedReader{ctx: ctx, r: r, left: indexBudget, size: size}
	magic, err := br.at(0, 4)
	if err != nil {
		return nil, err
	}
	var idx *Index
	if binary.BigEndian.Uint32(magic) == 0x1a45dfa3 {
		idx, err = readMKV(br)
	} else {
		idx, err = readMP4(br)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrIndex, err)
	}
	if len(idx.Points) == 0 || !finite(idx.Duration) || idx.Duration <= 0 {
		return nil, ErrIndex
	}
	sort.Slice(idx.Points, func(a, b int) bool { return idx.Points[a].Time < idx.Points[b].Time })
	idx.Origin = idx.Points[0].Time
	end := idx.Duration
	idx.Duration -= idx.Origin
	if idx.Duration <= 0 {
		return nil, ErrIndex
	}
	// Real cue/sample timestamps, never an invented fixed-duration grid.
	start := idx.Points[0]
	for _, p := range idx.Points[1:] {
		if !finite(p.Time) || p.Offset < 0 || p.Offset >= size {
			return nil, ErrIndex
		}
		if p.Time >= end {
			break
		}
		if p.Time-start.Time >= 4-0.000001 {
			idx.Segments = append(idx.Segments, Segment{start.Time - idx.Origin, p.Time - idx.Origin, start.Offset})
			start = p
		}
	}
	idx.Segments = append(idx.Segments, Segment{start.Time - idx.Origin, idx.Duration, start.Offset})
	return idx, nil
}
func finite(n float64) bool { return !math.IsNaN(n) && !math.IsInf(n, 0) }

type element struct {
	id        uint64
	data, end int64
}

func vint(b []byte, id bool) (uint64, int, error) {
	if len(b) == 0 || b[0] == 0 {
		return 0, 0, ErrIndex
	}
	n := 1
	mask := byte(0x80)
	for b[0]&mask == 0 {
		n++
		mask >>= 1
	}
	if n > 8 || len(b) < n {
		return 0, 0, ErrIndex
	}
	v := uint64(b[0])
	if !id {
		v &= uint64(mask - 1)
	}
	for _, x := range b[1:n] {
		v = v<<8 | uint64(x)
	}
	return v, n, nil
}
func (r *boundedReader) ebml(pos, limit int64) (element, error) {
	n := int(min(int64(12), limit-pos))
	b, err := r.at(pos, n)
	if err != nil {
		return element{}, err
	}
	id, a, err := vint(b, true)
	if err != nil {
		return element{}, err
	}
	sz, c, err := vint(b[a:], false)
	if err != nil {
		return element{}, err
	}
	data := pos + int64(a+c)
	end := limit
	if sz != (uint64(1)<<uint(c*7))-1 {
		if sz > uint64(limit-data) {
			return element{}, ErrIndex
		}
		end = data + int64(sz)
	}
	return element{id, data, end}, nil
}
func uintBE(b []byte) uint64 {
	var v uint64
	for _, x := range b {
		v = v<<8 | uint64(x)
	}
	return v
}
func walkEBML(b []byte, fn func(uint64, []byte) error) error {
	for len(b) > 0 {
		id, n, err := vint(b, true)
		if err != nil {
			return err
		}
		sz, m, err := vint(b[n:], false)
		if err != nil || sz > uint64(len(b)-n-m) {
			return ErrIndex
		}
		if err = fn(id, b[n+m:n+m+int(sz)]); err != nil {
			return err
		}
		b = b[n+m+int(sz):]
	}
	return nil
}
func readMKV(r *boundedReader) (*Index, error) {
	pos := int64(0)
	var root element
	for pos < r.size {
		e, err := r.ebml(pos, r.size)
		if err != nil {
			return nil, err
		}
		if e.id == 0x18538067 {
			root = e
			break
		}
		pos = e.end
	}
	if root.id == 0 {
		return nil, ErrIndex
	}
	refs := map[uint64]int64{}
	var info, tracks, cues []byte
	pos = root.data
	for pos < root.end {
		e, err := r.ebml(pos, root.end)
		if err != nil {
			return nil, err
		}
		if e.id == 0x1f43b675 {
			break
		} // Never walk payload clusters to find an index.
		if e.id == 0x114d9b74 || e.id == 0x1549a966 || e.id == 0x1654ae6b || e.id == 0x1c53bb6b {
			b, err := r.at(e.data, int(e.end-e.data))
			if err != nil {
				return nil, err
			}
			switch e.id {
			case 0x1549a966:
				info = b
			case 0x1654ae6b:
				tracks = b
			case 0x1c53bb6b:
				cues = b
			case 0x114d9b74:
				if err := walkEBML(b, func(id uint64, data []byte) error {
					if id != 0x4dbb {
						return nil
					}
					var target, offset uint64
					err := walkEBML(data, func(id uint64, v []byte) error {
						if len(v) > 8 {
							return ErrIndex
						}
						if id == 0x53ab {
							target = uintBE(v)
						}
						if id == 0x53ac {
							offset = uintBE(v)
						}
						return nil
					})
					if offset > uint64(root.end-root.data) {
						return ErrIndex
					}
					refs[target] = root.data + int64(offset)
					return err
				}); err != nil {
					return nil, err
				}
			}
		}
		pos = e.end
	}
	for id, dst := range map[uint64]*[]byte{0x1549a966: &info, 0x1654ae6b: &tracks, 0x1c53bb6b: &cues} {
		if len(*dst) > 0 {
			continue
		}
		p, ok := refs[id]
		if !ok {
			return nil, ErrIndex
		}
		e, err := r.ebml(p, root.end)
		if err != nil || e.id != id {
			return nil, ErrIndex
		}
		*dst, err = r.at(e.data, int(e.end-e.data))
		if err != nil {
			return nil, err
		}
	}
	scale := float64(1000000)
	duration := float64(0)
	if err := walkEBML(info, func(id uint64, b []byte) error {
		switch id {
		case 0x2ad7b1:
			scale = float64(uintBE(b))
		case 0x4489:
			if len(b) == 4 {
				duration = float64(math.Float32frombits(binary.BigEndian.Uint32(b)))
			} else if len(b) == 8 {
				duration = math.Float64frombits(binary.BigEndian.Uint64(b))
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	var video uint64
	subtitleTracks := map[uint64]int{}
	if err := walkEBML(tracks, func(id uint64, b []byte) error {
		if id != 0xae {
			return nil
		}
		var num, typ uint64
		err := walkEBML(b, func(id uint64, v []byte) error {
			if id == 0xd7 {
				num = uintBE(v)
			}
			if id == 0x83 {
				typ = uintBE(v)
			}
			return nil
		})
		if typ == 1 && video == 0 {
			video = num
		}
		if typ == 17 { subtitleTracks[num] = len(subtitleTracks) }
		return err
	}); err != nil {
		return nil, err
	}
	idx := &Index{Duration: duration * scale / 1e9, Subtitles: map[int][]SubtitleCue{}}
	if err := walkEBML(cues, func(id uint64, b []byte) error {
		if id != 0xbb {
			return nil
		}
		var ts uint64
		var points []Point
		type cue struct { track int; duration float64 }
		var subtitles []cue
		err := walkEBML(b, func(id uint64, v []byte) error {
			if id == 0xb3 {
				ts = uintBE(v)
			}
			if id != 0xb7 {
				return nil
			}
			var track, offset, cueDuration uint64
			err := walkEBML(v, func(id uint64, v []byte) error {
				if id == 0xf7 {
					track = uintBE(v)
				}
				if id == 0xf1 {
					offset = uintBE(v)
				}
				if id == 0xb2 { cueDuration = uintBE(v) }
				return nil
			})
			if track == video && track != 0 {
				if offset >= uint64(root.end-root.data) {
					return ErrIndex
				}
				points = append(points, Point{Offset: root.data + int64(offset)})
			}
			if ordinal, ok := subtitleTracks[track]; ok { subtitles = append(subtitles, cue{ordinal, float64(cueDuration)*scale/1e9}) }
			return err
		})
		for _, p := range points {
			p.Time = float64(ts) * scale / 1e9
			idx.Points = append(idx.Points, p)
		}
		for _, cue := range subtitles { idx.Subtitles[cue.track] = append(idx.Subtitles[cue.track], SubtitleCue{float64(ts)*scale/1e9, cue.duration}) }
		return err
	}); err != nil {
		return nil, err
	}
	return idx, nil
}

type box struct {
	typ  string
	data []byte
	raw  []byte
}

func boxes(b []byte) ([]box, error) {
	var out []box
	for len(b) > 0 {
		if len(b) < 8 {
			return nil, ErrIndex
		}
		n := uint64(binary.BigEndian.Uint32(b))
		head := 8
		if n == 1 {
			if len(b) < 16 {
				return nil, ErrIndex
			}
			n = binary.BigEndian.Uint64(b[8:])
			head = 16
		}
		if n == 0 {
			n = uint64(len(b))
		}
		if n < uint64(head) || n > uint64(len(b)) {
			return nil, ErrIndex
		}
		out = append(out, box{string(b[4:8]), b[head:int(n)], b[:int(n)]})
		b = b[int(n):]
	}
	return out, nil
}
func child(b []byte, typ string) []byte {
	xs, err := boxes(b)
	if err != nil {
		return nil
	}
	for _, x := range xs {
		if x.typ == typ {
			return x.data
		}
	}
	return nil
}
func readMP4(r *boundedReader) (*Index, error) {
	var moov []byte
	for pos := int64(0); pos < r.size; {
		h, err := r.at(pos, int(min(int64(16), r.size-pos)))
		if err != nil || len(h) < 8 {
			return nil, ErrIndex
		}
		n := int64(binary.BigEndian.Uint32(h))
		head := int64(8)
		if n == 1 {
			if len(h) < 16 {
				return nil, ErrIndex
			}
			n = int64(binary.BigEndian.Uint64(h[8:]))
			head = 16
		}
		if n == 0 {
			n = r.size - pos
		}
		if n < head || n > r.size-pos {
			return nil, ErrIndex
		}
		if string(h[4:8]) == "moov" {
			moov, err = r.at(pos+head, int(n-head))
			if err != nil {
				return nil, err
			}
			break
		}
		pos += n
	}
	tracks, err := boxes(moov)
	if err != nil {
		return nil, err
	}
	for _, track := range tracks {
		if track.typ != "trak" {
			continue
		}
		mdia := child(track.data, "mdia")
		hdlr := child(mdia, "hdlr")
		if len(hdlr) < 12 || string(hdlr[8:12]) != "vide" {
			continue
		}
		mdhd := child(mdia, "mdhd")
		scale, duration, err := mediaTime(mdhd)
		if err != nil {
			return nil, err
		}
		table := child(child(mdia, "minf"), "stbl")
		stts := child(table, "stts")
		ctts := child(table, "ctts")
		stss := child(table, "stss")
		offsetFor, err := sampleOffsets(table, r.size)
		if err != nil {
			return nil, err
		}
		if len(stts) < 8 {
			return nil, ErrIndex
		}
		entries := int(binary.BigEndian.Uint32(stts[4:]))
		if entries > (len(stts)-8)/8 {
			return nil, ErrIndex
		}
		var syncs []uint32
		if len(stss) >= 8 {
			count := int(binary.BigEndian.Uint32(stss[4:]))
			if count > (len(stss)-8)/4 {
				return nil, ErrIndex
			}
			for n := 0; n < count; n++ {
				syncs = append(syncs, binary.BigEndian.Uint32(stss[8+n*4:]))
			}
		}
		var offsets []struct {
			count  uint32
			offset int64
		}
		if len(ctts) >= 8 {
			count := int(binary.BigEndian.Uint32(ctts[4:]))
			if count > (len(ctts)-8)/8 {
				return nil, ErrIndex
			}
			for n := 0; n < count; n++ {
				v := binary.BigEndian.Uint32(ctts[12+n*8:])
				o := int64(v)
				if ctts[0] == 1 {
					o = int64(int32(v))
				}
				offsets = append(offsets, struct {
					count  uint32
					offset int64
				}{binary.BigEndian.Uint32(ctts[8+n*8:]), o})
			}
		}
		shift := editShift(track.data, child(moov, "mvhd"), scale)
		idx := &Index{Duration: float64(duration)/float64(scale) + shift}
		var sample uint32 = 1
		var dts uint64
		ci := 0
		var cn uint32
		si := 0
		for n := 0; n < entries; n++ {
			count := binary.BigEndian.Uint32(stts[8+n*8:])
			delta := binary.BigEndian.Uint32(stts[12+n*8:])
			if count > 10000000 || uint64(sample)+uint64(count) > 10000001 || delta == 0 {
				return nil, ErrIndex
			}
			for k := uint32(0); k < count; k++ {
				if sample%1024 == 0 {
					if err := r.ctx.Err(); err != nil {
						return nil, err
					}
				}
				offset, err := offsetFor(sample)
				if err != nil {
					return nil, err
				}
				var composition int64
				if ci < len(offsets) {
					composition = offsets[ci].offset
					cn++
					if cn == offsets[ci].count {
						ci++
						cn = 0
					}
				}
				if len(stss) == 0 || (si < len(syncs) && sample == syncs[si]) {
					if len(idx.Points) > 200000 {
						return nil, ErrIndex
					}
					idx.Points = append(idx.Points, Point{Time: float64(int64(dts)+composition)/float64(scale) + shift, Offset: offset})
					si++
				}
				sample++
				dts += uint64(delta)
			}
		}
		return idx, nil
	}
	return nil, ErrIndex
}

// MP4 sample-to-chunk and size tables provide byte offsets without touching mdat.
func sampleOffsets(table []byte, size int64) (func(uint32) (int64, error), error) {
	stsc, stsz := child(table, "stsc"), child(table, "stsz")
	chunks := child(table, "stco")
	wide := false
	if len(chunks) == 0 {
		chunks = child(table, "co64")
		wide = true
	}
	if len(stsc) < 8 || len(stsz) < 12 || len(chunks) < 8 {
		return nil, ErrIndex
	}
	rows := int(binary.BigEndian.Uint32(stsc[4:]))
	count := int(binary.BigEndian.Uint32(chunks[4:]))
	width := 4
	if wide {
		width = 8
	}
	if rows == 0 || count == 0 || rows > (len(stsc)-8)/12 || count > (len(chunks)-8)/width {
		return nil, ErrIndex
	}
	samples := binary.BigEndian.Uint32(stsz[8:])
	fixed := binary.BigEndian.Uint32(stsz[4:])
	if fixed == 0 && uint64(samples) > uint64((len(stsz)-12)/4) {
		return nil, ErrIndex
	}
	for n := 0; n < rows; n++ {
		first := binary.BigEndian.Uint32(stsc[8+n*12:])
		if first == 0 || (n == 0 && first != 1) || binary.BigEndian.Uint32(stsc[12+n*12:]) == 0 {
			return nil, ErrIndex
		}
		if n > 0 && first <= binary.BigEndian.Uint32(stsc[8+(n-1)*12:]) {
			return nil, ErrIndex
		}
	}
	chunk, row, inChunk := 0, 0, uint32(0)
	offset := int64(0)
	return func(sample uint32) (int64, error) {
		if sample == 0 || sample > samples || chunk >= count {
			return 0, ErrIndex
		}
		if inChunk == 0 {
			for row+1 < rows && uint32(chunk+1) >= binary.BigEndian.Uint32(stsc[8+(row+1)*12:]) {
				row++
			}
			p := 8 + chunk*width
			var raw uint64
			if wide {
				raw = binary.BigEndian.Uint64(chunks[p:])
			} else {
				raw = uint64(binary.BigEndian.Uint32(chunks[p:]))
			}
			if raw >= uint64(size) {
				return 0, ErrIndex
			}
			offset = int64(raw)
		}
		sampleSize := fixed
		if fixed == 0 {
			sampleSize = binary.BigEndian.Uint32(stsz[12+int(sample-1)*4:])
		}
		if sampleSize == 0 || int64(sampleSize) > size-offset {
			return 0, ErrIndex
		}
		result := offset
		offset += int64(sampleSize)
		inChunk++
		if inChunk == binary.BigEndian.Uint32(stsc[12+row*12:]) {
			chunk++
			inChunk = 0
		}
		return result, nil
	}, nil
}
func mediaTime(b []byte) (uint32, uint64, error) {
	if len(b) < 20 {
		return 0, 0, ErrIndex
	}
	if b[0] == 1 {
		if len(b) < 32 {
			return 0, 0, ErrIndex
		}
		s := binary.BigEndian.Uint32(b[20:])
		if s == 0 {
			return 0, 0, ErrIndex
		}
		return s, binary.BigEndian.Uint64(b[24:]), nil
	}
	s := binary.BigEndian.Uint32(b[12:])
	if s == 0 {
		return 0, 0, ErrIndex
	}
	return s, uint64(binary.BigEndian.Uint32(b[16:])), nil
}
func editShift(trak, mvhd []byte, scale uint32) float64 {
	movieScale, _, err := mediaTime(mvhd)
	if err != nil {
		return 0
	}
	elst := child(child(trak, "edts"), "elst")
	if len(elst) < 8 {
		return 0
	}
	count := int(binary.BigEndian.Uint32(elst[4:]))
	pos := 8
	var shift float64
	for n := 0; n < count; n++ {
		var duration uint64
		var media int64
		if elst[0] == 1 {
			if len(elst) < pos+20 {
				return 0
			}
			duration = binary.BigEndian.Uint64(elst[pos:])
			media = int64(binary.BigEndian.Uint64(elst[pos+8:]))
			pos += 20
		} else {
			if len(elst) < pos+12 {
				return 0
			}
			duration = uint64(binary.BigEndian.Uint32(elst[pos:]))
			media = int64(int32(binary.BigEndian.Uint32(elst[pos+4:])))
			pos += 12
		}
		if media == -1 {
			shift += float64(duration) / float64(movieScale)
		} else {
			return shift - float64(media)/float64(scale)
		}
	}
	return shift
}
