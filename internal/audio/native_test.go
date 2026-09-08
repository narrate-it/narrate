package audio

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestNativeAIFFPaddingAndEndian(t *testing.T) {
	// macOS say inserts FLLR before SSND; only the SSND samples are audio.
	h := &countSeeker{buf: new(bytes.Buffer)}
	w, err := NewAiffWriter(h, ClipSpec{SampleRate: 22050, Channels: 1, Bits: 16})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte{0x12, 0x34, 0x56, 0x78}); err != nil {
		t.Fatal(err)
	}
	if err := w.Finalize(); err != nil {
		t.Fatal(err)
	}
	header := append([]byte(nil), h.buf.Bytes()[:54]...)
	clip := append([]byte(nil), header[:38]...)
	clip = append(clip, []byte{'F', 'L', 'L', 'R', 0, 0, 0, 2, 0, 0}...)
	clip = append(clip, header[38:]...)
	clip = append(clip, 0x12, 0x34, 0x56, 0x78)
	binary.BigEndian.PutUint32(clip[4:8], uint32(len(clip)-8))
	var out bytes.Buffer
	if err := convertAndWrite(&out, clip, false); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out.Bytes(), []byte{0x34, 0x12, 0x78, 0x56}) {
		t.Fatalf("wrong PCM: %x", out.Bytes())
	}
}
