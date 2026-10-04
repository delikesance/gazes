package metadata

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
)

const (
	ebmlIDSeekHead      = 0x114D9B74
	ebmlIDSeek          = 0x4DBB
	ebmlIDSeekID        = 0x53AB
	ebmlIDSeekPosition  = 0x53AC
	ebmlIDChapters      = 0x1043A770
	ebmlIDEditionEntry  = 0x45B9
	ebmlIDEditionFlagDf = 0x45DB
	ebmlIDEditionFlagOr = 0x45DD
	ebmlIDChapterAtom   = 0xB6
	ebmlIDChapterStart  = 0x91
	ebmlIDChapterEnd    = 0x92
	ebmlIDChapterHidden = 0x98
	ebmlIDChapterEnable = 0x4598
	ebmlIDChapterSegUID = 0x6E67
	ebmlIDChapterDisp   = 0x80
	ebmlIDChapString    = 0x85
	ebmlIDChapLanguage  = 0x437C

	// Chapter reads are bounded: header walk, SeekHead and Chapters together stay small.
	matroskaChaptersReadBudget = 1 << 20
	matroskaChaptersMaxBytes   = 512 << 10
)

// Chapter is one Matroska chapter, in seconds. End is 0 when the file does not give one.
type Chapter struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Title string  `json:"title"`
}

// readMatroskaChapters reads the chapters of a Matroska file without touching any
// cluster: it walks the Segment's children up to the first Cluster and, when Chapters
// is not among them, follows the SeekHead (chapters are often written at the file end).
// Input that is not Matroska or has no usable chapters gives nil, nil; corrupted data
// and budget overruns give an error.
func readMatroskaChapters(ctx context.Context, r io.ReadSeeker, size int64) ([]Chapter, error) {
	var consumed int64
	read := func(p []byte) (int, error) {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		consumed += int64(len(p))
		if consumed > matroskaChaptersReadBudget {
			return 0, errors.New("matroska chapters: read budget exceeded")
		}
		if contextual, ok := r.(interface {
			ReadContext(context.Context, []byte) (int, error)
		}); ok {
			return contextual.ReadContext(ctx, p)
		}
		return r.Read(p)
	}
	full := func(p []byte) error { return readFull(read, p) }

	var position int64
	// header reads exactly one element header, never a payload byte.
	header := func() (id uint64, length int64, unknown bool, err error) {
		var first [1]byte
		if err = full(first[:]); err != nil {
			return
		}
		idLen := ebmlLength(first[0])
		if idLen == 0 || idLen > 4 {
			return 0, 0, false, errNotEBML
		}
		id = uint64(first[0])
		if idLen > 1 {
			rest := make([]byte, idLen-1)
			if err = full(rest); err != nil {
				return
			}
			for _, b := range rest {
				id = id<<8 | uint64(b)
			}
		}
		if err = full(first[:]); err != nil {
			return
		}
		sizeLen := ebmlLength(first[0])
		if sizeLen == 0 {
			return 0, 0, false, errNotEBML
		}
		value := uint64(first[0]) & (0xFF >> sizeLen)
		allOnes := value == 0xFF>>sizeLen
		if sizeLen > 1 {
			rest := make([]byte, sizeLen-1)
			if err = full(rest); err != nil {
				return
			}
			for _, b := range rest {
				value = value<<8 | uint64(b)
				allOnes = allOnes && b == 0xFF
			}
		}
		position += int64(idLen + sizeLen)
		if allOnes || value > 1<<62 {
			return id, 0, true, nil
		}
		return id, int64(value), false, nil
	}
	skip := func(n int64) error {
		position += n
		_, err := r.Seek(position, io.SeekStart)
		return err
	}
	payload := func(n, limit int64) ([]byte, error) {
		if n > limit {
			return nil, fmt.Errorf("matroska chapters: element of %d bytes exceeds %d", n, limit)
		}
		if size > 0 && position+n > size {
			return nil, io.ErrUnexpectedEOF
		}
		buf := make([]byte, n)
		if err := full(buf); err != nil {
			return nil, err
		}
		position += n
		return buf, nil
	}
	notMatroska := func(err error) ([]Chapter, error) {
		if errors.Is(err, errNotEBML) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, nil
		}
		return nil, err
	}

	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	id, n, unknown, err := header()
	if err != nil {
		return notMatroska(err)
	}
	if id != ebmlIDHeader || unknown {
		return nil, nil
	}
	if err := skip(n); err != nil {
		return nil, err
	}
	if id, _, _, err = header(); err != nil {
		return notMatroska(err)
	}
	if id != ebmlIDSegment {
		return nil, nil
	}
	segmentStart := position

	chaptersAt := int64(-1) // Segment-relative position announced by the SeekHead
	for {
		id, n, unknown, err = header()
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, errNotEBML) {
				break // end of the walkable header
			}
			return nil, err
		}
		if id == ebmlIDCluster || unknown {
			break
		}
		switch id {
		case ebmlIDChapters:
			data, err := payload(n, matroskaChaptersMaxBytes)
			if err != nil {
				return nil, err
			}
			return parseChapters(data)
		case ebmlIDSeekHead:
			data, err := payload(n, matroskaChaptersMaxBytes)
			if err != nil {
				return nil, err
			}
			if at := chaptersSeekPosition(data); at >= 0 {
				chaptersAt = at
			}
		default:
			if size > 0 && position+n > size {
				return nil, nil
			}
			if err := skip(n); err != nil {
				return nil, err
			}
		}
	}
	if chaptersAt < 0 {
		return nil, nil
	}
	position = segmentStart + chaptersAt
	if size > 0 && position >= size {
		return nil, nil
	}
	if _, err := r.Seek(position, io.SeekStart); err != nil {
		return nil, err
	}
	id, n, unknown, err = header()
	if err != nil {
		return notMatroska(err)
	}
	if id != ebmlIDChapters || unknown {
		return nil, nil
	}
	data, err := payload(n, matroskaChaptersMaxBytes)
	if err != nil {
		return nil, err
	}
	return parseChapters(data)
}

var errNotEBML = errors.New("not an EBML element")

func readFull(read func([]byte) (int, error), p []byte) error {
	_, err := io.ReadFull(readerFunc(read), p)
	return err
}

type ebmlChild struct {
	id   uint64
	data []byte
}

// ebmlChildren splits a master element's payload into its direct children.
func ebmlChildren(b []byte) ([]ebmlChild, error) {
	var out []ebmlChild
	for len(b) > 0 {
		idLen := ebmlLength(b[0])
		if idLen == 0 || idLen > 4 || len(b) < idLen+1 {
			return nil, errors.New("matroska chapters: bad element id")
		}
		var id uint64
		for _, c := range b[:idLen] {
			id = id<<8 | uint64(c)
		}
		sizeLen := ebmlLength(b[idLen])
		if sizeLen == 0 || len(b) < idLen+sizeLen {
			return nil, errors.New("matroska chapters: bad element size")
		}
		value := uint64(b[idLen]) & (0xFF >> sizeLen)
		for _, c := range b[idLen+1 : idLen+sizeLen] {
			value = value<<8 | uint64(c)
		}
		start := idLen + sizeLen
		if value > uint64(len(b)-start) {
			return nil, errors.New("matroska chapters: element overruns its parent")
		}
		end := start + int(value)
		out = append(out, ebmlChild{id: id, data: b[start:end]})
		b = b[end:]
	}
	return out, nil
}

func ebmlUintValue(b []byte) uint64 {
	var v uint64
	for _, c := range b {
		v = v<<8 | uint64(c)
	}
	return v
}

// chaptersSeekPosition returns the Segment-relative position of Chapters announced by
// a SeekHead payload, or -1.
func chaptersSeekPosition(seekHead []byte) int64 {
	entries, err := ebmlChildren(seekHead)
	if err != nil {
		return -1
	}
	for _, entry := range entries {
		if entry.id != ebmlIDSeek {
			continue
		}
		fields, err := ebmlChildren(entry.data)
		if err != nil {
			continue
		}
		var target uint64
		position := int64(-1)
		for _, f := range fields {
			switch f.id {
			case ebmlIDSeekID:
				target = ebmlUintValue(f.data)
			case ebmlIDSeekPosition:
				if v := ebmlUintValue(f.data); v <= 1<<62 {
					position = int64(v)
				}
			}
		}
		if target == ebmlIDChapters && position >= 0 {
			return position
		}
	}
	return -1
}

// parseChapters picks the edition and returns its enabled, non-linked top-level atoms.
func parseChapters(data []byte) ([]Chapter, error) {
	children, err := ebmlChildren(data)
	if err != nil {
		return nil, err
	}
	var chosen []ebmlChild
	chosenDefault := false
	for _, child := range children {
		if child.id != ebmlIDEditionEntry {
			continue
		}
		fields, err := ebmlChildren(child.data)
		if err != nil {
			return nil, err
		}
		isDefault, ordered := false, false
		for _, f := range fields {
			switch f.id {
			case ebmlIDEditionFlagDf:
				isDefault = ebmlUintValue(f.data) == 1
			case ebmlIDEditionFlagOr:
				ordered = ebmlUintValue(f.data) == 1
			}
		}
		if ordered {
			continue // ordered chapters are out of scope
		}
		if chosen == nil || (isDefault && !chosenDefault) {
			chosen, chosenDefault = fields, isDefault
		}
	}

	type atom struct {
		start, end int64
		hasEnd     bool
		title      string
	}
	var atoms []atom
	for _, f := range chosen {
		if f.id != ebmlIDChapterAtom {
			continue
		}
		fields, err := ebmlChildren(f.data)
		if err != nil {
			return nil, err
		}
		a := atom{}
		skipAtom := false
		titleEnglish := false
		for _, af := range fields {
			switch af.id {
			case ebmlIDChapterStart:
				a.start = int64(ebmlUintValue(af.data))
			case ebmlIDChapterEnd:
				a.end, a.hasEnd = int64(ebmlUintValue(af.data)), true
			case ebmlIDChapterHidden:
				skipAtom = skipAtom || ebmlUintValue(af.data) == 1
			case ebmlIDChapterEnable:
				skipAtom = skipAtom || ebmlUintValue(af.data) == 0
			case ebmlIDChapterSegUID:
				skipAtom = true
			case ebmlIDChapterDisp:
				title, lang, ok := chapterDisplay(af.data)
				if !ok {
					continue
				}
				english := lang == "eng"
				if a.title == "" || (english && !titleEnglish) {
					a.title, titleEnglish = title, english
				}
			}
		}
		if !skipAtom {
			atoms = append(atoms, a)
		}
	}
	if len(atoms) == 0 {
		return nil, nil
	}
	sort.SliceStable(atoms, func(i, j int) bool { return atoms[i].start < atoms[j].start })
	out := make([]Chapter, len(atoms))
	for i, a := range atoms {
		c := Chapter{Start: float64(a.start) / 1e9, Title: a.title}
		switch {
		case a.hasEnd:
			c.End = float64(a.end) / 1e9
		case i+1 < len(atoms):
			c.End = float64(atoms[i+1].start) / 1e9
		}
		out[i] = c
	}
	return out, nil
}

// chapterDisplay extracts the title and language of a ChapterDisplay. A missing
// language is "eng", the Matroska default.
func chapterDisplay(data []byte) (title, lang string, ok bool) {
	fields, err := ebmlChildren(data)
	if err != nil {
		return "", "", false
	}
	lang = "eng"
	for _, f := range fields {
		switch f.id {
		case ebmlIDChapString:
			title, ok = string(bytes.TrimRight(f.data, "\x00")), true
		case ebmlIDChapLanguage:
			lang = string(bytes.TrimRight(f.data, "\x00"))
		}
	}
	return title, lang, ok
}
