package audio

import (
	"bytes"
	"testing"
)

// countSeeker is a minimal WriteSeeker over a buffer.
type countSeeker struct{ buf *bytes.Buffer }

func (c *countSeeker) Write(p []byte) (int, error) { return c.buf.Write(p) }

func (c *countSeeker) Seek(offset int64, whence int) (int64, error) {
	b := c.buf.Bytes()
	switch whence {
	case 0:
		c.buf.Reset()
		c.buf.Write(b[:offset])
	case 1:
		// relative: not used by these writers beyond offset 0
	}
	return int64(len(b)), nil
}

func TestWavRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	w, err := NewWavWriter(&countSeeker{buf: &buf}, ClipSpec{SampleRate: 22050, Channels: 1, Bits: 16})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.WriteSilence(650); err != nil {
		t.Fatal(err)
	}
	if err := w.Finalize(); err != nil {
		t.Fatal(err)
	}
	spec, frames, err := ValidateClip(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if spec.SampleRate != 22050 || frames == 0 {
		t.Fatalf("spec=%+v frames=%d", spec, frames)
	}
	d := w.DurationSeconds()
	if d < 0.6 || d > 0.7 {
		t.Fatalf("duration %f", d)
	}
}

func TestAiffRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	w, err := NewAiffWriter(&countSeeker{buf: &buf}, ClipSpec{SampleRate: 44100, Channels: 1, Bits: 16})
	if err != nil {
		t.Fatal(err)
	}
	w.WriteSilence(100)
	if err := w.Finalize(); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	if string(b[0:4]) != "FORM" || string(b[8:12]) != "AIFF" || string(b[12:16]) != "COMM" {
		t.Fatalf("bad AIFF header: %q", b[:16])
	}
	if w.DurationSeconds() < 0.09 || w.DurationSeconds() > 0.11 {
		t.Fatalf("duration %f", w.DurationSeconds())
	}
}

func TestValidateRejectsEmpty(t *testing.T) {
	if _, _, err := ValidateClip(bytes.NewReader(make([]byte, 44))); err == nil {
		t.Fatal("expected empty-payload rejection")
	}
}
