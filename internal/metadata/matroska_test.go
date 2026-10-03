package metadata

import (
	"bytes"
	"context"
	"io"
	"testing"
)

func ebmlElement(id []byte, payload []byte) []byte {
	// 8-byte size, like muxers that reserve room for large payloads.
	size := make([]byte, 8)
	size[0] = 0x01
	n := uint64(len(payload))
	for i := 7; i >= 1; i-- {
		size[i] = byte(n)
		n >>= 8
	}
	out := append([]byte{}, id...)
	out = append(out, size...)
	return append(out, payload...)
}

var (
	idEBML        = []byte{0x1A, 0x45, 0xDF, 0xA3}
	idSegment     = []byte{0x18, 0x53, 0x80, 0x67}
	idTracks      = []byte{0x16, 0x54, 0xAE, 0x6B}
	idAttachments = []byte{0x19, 0x41, 0xA4, 0x69}
	idCluster     = []byte{0x1F, 0x43, 0xB6, 0x75}
	idTags        = []byte{0x12, 0x54, 0xC3, 0x67}
)

func syntheticMKV(attachment int) (file []byte, withoutAttachment []byte) {
	segmentHeader := append(append([]byte{}, idSegment...), 0x01, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF) // unknown size
	head := append(ebmlElement(idEBML, []byte("webm")), segmentHeader...)
	head = append(head, ebmlElement(idTracks, []byte("tracks"))...)
	tail := append(ebmlElement(idTags, []byte("tags")), ebmlElement(idCluster, bytes.Repeat([]byte{7}, 64))...)
	attachments := ebmlElement(idAttachments, bytes.Repeat([]byte{9}, attachment))
	file = append(append(append([]byte{}, head...), attachments...), tail...)
	withoutAttachment = append(append(append([]byte{}, head...), 0xEC, 0x80), tail...)
	return file, withoutAttachment
}

func TestSkipMatroskaAttachmentsDropsFontPayload(t *testing.T) {
	file, want := syntheticMKV(3 << 20)
	got, err := io.ReadAll(skipMatroskaAttachments(context.Background(), bytes.NewReader(file), int64(len(file))))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("virtual stream = %d bytes, want %d bytes with the attachment replaced by an empty Void", len(got), len(want))
	}
}

func TestSkipMatroskaAttachmentsLeavesOtherInputUntouched(t *testing.T) {
	noAttachment := append(ebmlElement(idEBML, []byte("webm")), idSegment...)
	noAttachment = append(noAttachment, 0x01, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF)
	noAttachment = append(noAttachment, ebmlElement(idTracks, []byte("tracks"))...)
	noAttachment = append(noAttachment, ebmlElement(idCluster, []byte("data"))...)
	for name, input := range map[string][]byte{
		"no attachments": noAttachment,
		"not matroska":   []byte("\x00\x00\x00\x18ftypmp42 plain mp4 bytes"),
		"empty":          nil,
	} {
		got, err := io.ReadAll(skipMatroskaAttachments(context.Background(), bytes.NewReader(input), int64(len(input))))
		if err != nil || !bytes.Equal(got, input) {
			t.Fatalf("%s: stream changed (err=%v)", name, err)
		}
	}
}

func TestSkipMatroskaAttachmentsIgnoresAttachmentBeyondFile(t *testing.T) {
	file, _ := syntheticMKV(1 << 20)
	truncated := file[:200] // attachment claims more bytes than the file holds
	got, _ := io.ReadAll(skipMatroskaAttachments(context.Background(), bytes.NewReader(truncated), 200))
	if !bytes.Equal(got, truncated) {
		t.Fatal("a truncated attachment must not be rewritten")
	}
}

type contextOnlyReader struct{ *bytes.Reader }

func (r contextOnlyReader) ReadContext(_ context.Context, p []byte) (int, error) { return r.Read(p) }

func TestSkipMatroskaAttachmentsKeepsContextReads(t *testing.T) {
	file, _ := syntheticMKV(1 << 20)
	wrapped := skipMatroskaAttachments(context.Background(), contextOnlyReader{bytes.NewReader(file)}, int64(len(file)))
	if _, ok := wrapped.(interface {
		ReadContext(context.Context, []byte) (int, error)
	}); !ok {
		t.Fatal("wrapped reader must keep ReadContext so a stalled swarm cannot outlive the probe timeout")
	}
}
