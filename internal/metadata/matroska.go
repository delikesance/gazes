package metadata

import (
	"bytes"
	"context"
	"io"
)

const (
	ebmlIDHeader      = 0x1A45DFA3
	ebmlIDSegment     = 0x18538067
	ebmlIDCluster     = 0x1F43B675
	ebmlIDAttachments = 0x1941A469

	// Top-level elements copied verbatim before the attachments (SeekHead, Info, Tracks…)
	// are tiny; anything bigger than this means the file is not worth rewriting.
	matroskaMaxHeaderBytes = 8 << 20
)

// emptyEBMLVoid is a zero-length Void element: demuxers skip it.
var emptyEBMLVoid = []byte{0xEC, 0x80}

// probeStream is a probe input that keeps the context-aware read of a torrent reader.
type probeStream struct {
	prefix *bytes.Reader
	inner  io.Reader
}

func (s *probeStream) Read(p []byte) (int, error) { return s.ReadContext(context.Background(), p) }

func (s *probeStream) ReadContext(ctx context.Context, p []byte) (int, error) {
	if s.prefix.Len() > 0 {
		return s.prefix.Read(p)
	}
	if contextual, ok := s.inner.(interface {
		ReadContext(context.Context, []byte) (int, error)
	}); ok {
		return contextual.ReadContext(ctx, p)
	}
	return s.inner.Read(p)
}

// skipMatroskaAttachments lets FFprobe see the tracks of MKVs that embed megabytes of
// fonts before their first cluster. Its Matroska demuxer reads every header element
// before reporting any stream, so a fixed probe window never reaches the tracks of such
// fansub releases (duration, audio and subtitle lists all come back empty). The returned
// stream replaces the Attachments payload with an empty Void element and seeks the
// source past it. Input that is not a seekable Matroska file with attachments, or that
// cannot be parsed, is returned unchanged and rewound.
func skipMatroskaAttachments(ctx context.Context, r io.Reader, totalBytes int64) io.Reader {
	source, ok := r.(io.ReadSeeker)
	if !ok {
		return r
	}
	read := func(p []byte) (int, error) {
		if contextual, ok := r.(interface {
			ReadContext(context.Context, []byte) (int, error)
		}); ok {
			return contextual.ReadContext(ctx, p)
		}
		return source.Read(p)
	}
	unchanged := func() io.Reader {
		_, _ = source.Seek(0, io.SeekStart)
		return r
	}

	var prefix bytes.Buffer
	var position int64
	var lastHeader int // encoded length of the header readHeader consumed last
	readHeader := func() (id uint64, size int64, unknown bool, ok bool) {
		var header [12]byte
		n, err := io.ReadAtLeast(readerFunc(read), header[:], 2)
		if err != nil && n < 2 {
			return 0, 0, false, false
		}
		idLen := ebmlLength(header[0])
		if idLen == 0 || idLen > 4 || n < idLen+1 {
			return 0, 0, false, false
		}
		for _, b := range header[:idLen] {
			id = id<<8 | uint64(b)
		}
		sizeLen := ebmlLength(header[idLen])
		if sizeLen == 0 || n < idLen+sizeLen {
			return 0, 0, false, false
		}
		value := uint64(header[idLen]) & (0xFF >> sizeLen)
		allOnes := value == 0xFF>>sizeLen
		for _, b := range header[idLen+1 : idLen+sizeLen] {
			value = value<<8 | uint64(b)
			allOnes = allOnes && b == 0xFF
		}
		used := idLen + sizeLen
		// Hand back the bytes read past this header.
		if n > used {
			if _, err := source.Seek(int64(used-n), io.SeekCurrent); err != nil {
				return 0, 0, false, false
			}
		}
		prefix.Write(header[:used])
		lastHeader = used
		position += int64(used)
		if value > 1<<62 {
			return id, 0, true, true
		}
		return id, int64(value), allOnes, true
	}
	copyPayload := func(size int64) bool {
		if size > matroskaMaxHeaderBytes || position+size > matroskaMaxHeaderBytes {
			return false
		}
		n, err := io.CopyN(&prefix, readerFunc(read), size)
		position += n
		return err == nil
	}

	id, size, unknown, ok := readHeader()
	if !ok || id != ebmlIDHeader || unknown || !copyPayload(size) {
		return unchanged()
	}
	if id, _, _, ok = readHeader(); !ok || id != ebmlIDSegment {
		return unchanged() // the Segment's own size is irrelevant for a piped probe
	}
	for {
		id, size, unknown, ok = readHeader()
		if !ok || unknown || id == ebmlIDCluster {
			return unchanged()
		}
		if id == ebmlIDAttachments {
			after := position + size
			if totalBytes > 0 && after > totalBytes {
				return unchanged()
			}
			// readHeader already appended the Attachments header; drop it for the Void.
			prefix.Truncate(prefix.Len() - lastHeader)
			if _, err := source.Seek(after, io.SeekStart); err != nil {
				return unchanged()
			}
			prefix.Write(emptyEBMLVoid)
			return &probeStream{prefix: bytes.NewReader(prefix.Bytes()), inner: r}
		}
		if !copyPayload(size) {
			return unchanged()
		}
	}
}

// readerFunc adapts a read function to io.Reader.
type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }

// ebmlLength is the byte length of an EBML variable-size integer from its first byte.
func ebmlLength(first byte) int {
	for n := 1; n <= 8; n++ {
		if first&(0x80>>(n-1)) != 0 {
			return n
		}
	}
	return 0
}
