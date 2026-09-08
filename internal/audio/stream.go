package audio

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// StreamWriter assembles clips and silence into one container.
type StreamWriter interface {
	CopyClip(path string) error
	WriteSilence(ms int) error
	Finalize() error
}

// wavStreamWriter adapts WavWriter.
type wavStreamWriter struct{ w *WavWriter }

func NewWavStreamWriter(ws io.WriteSeeker, spec ClipSpec) (StreamWriter, error) {
	w, err := NewWavWriter(ws, spec)
	if err != nil {
		return nil, err
	}
	return wavStreamWriter{w}, nil
}

func (s wavStreamWriter) CopyClip(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return convertAndWrite(s.w, data, false)
}
func (s wavStreamWriter) WriteSilence(ms int) error { return s.w.WriteSilence(ms) }
func (s wavStreamWriter) Finalize() error           { return s.w.Finalize() }

// aiffStreamWriter adapts AiffWriter.
type aiffStreamWriter struct{ w *AiffWriter }

func NewAiffStreamWriter(ws io.WriteSeeker, spec ClipSpec) (StreamWriter, error) {
	w, err := NewAiffWriter(ws, spec)
	if err != nil {
		return nil, err
	}
	return aiffStreamWriter{w}, nil
}

func (s aiffStreamWriter) CopyClip(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return convertAndWrite(s.w, data, true)
}
func (s aiffStreamWriter) WriteSilence(ms int) error { return s.w.WriteSilence(ms) }
func (s aiffStreamWriter) Finalize() error           { return s.w.Finalize() }

// convertAndWrite strips a clip's container header and appends raw PCM,
// byte-swapping WAV (little-endian) samples into AIFF (big-endian) when needed.
func convertAndWrite(w io.Writer, clip []byte, toBigEndian bool) error {
	spec, _, payload, err := parseAIFF(clip)
	if err != nil {
		return err
	}
	if !toBigEndian {
		payload = append([]byte(nil), payload...)
		width := spec.Bits / 8
		for i := 0; i < len(payload); i += width {
			for j := 0; j < width/2; j++ {
				payload[i+j], payload[i+width-1-j] = payload[i+width-1-j], payload[i+j]
			}
		}
	}
	_, err = w.Write(payload)
	return err
}

// parseAIFF walks chunks rather than assuming a fixed native header layout.
func parseAIFF(data []byte) (ClipSpec, uint64, []byte, error) {
	var spec ClipSpec
	var frames uint64
	var payload []byte
	fail := func() (ClipSpec, uint64, []byte, error) {
		return ClipSpec{}, 0, nil, fmt.Errorf("audio: invalid or unsupported AIFF clip")
	}
	if len(data) < 12 || string(data[:4]) != "FORM" || string(data[8:12]) != "AIFF" {
		return fail()
	}
	end := uint64(binary.BigEndian.Uint32(data[4:8])) + 8
	if end > uint64(len(data)) || end < 12 {
		return fail()
	}
	for pos := uint64(12); pos+8 <= end; {
		n := uint64(binary.BigEndian.Uint32(data[pos+4 : pos+8]))
		start := pos + 8
		if start+n > end {
			return fail()
		}
		chunk := data[start : start+n]
		switch string(data[pos : pos+4]) {
		case "COMM":
			if n < 18 {
				return fail()
			}
			spec = ClipSpec{Channels: int(binary.BigEndian.Uint16(chunk[:2])), Bits: int(binary.BigEndian.Uint16(chunk[6:8])), SampleRate: int(decodeExtended(chunk[8:18]))}
			frames = uint64(binary.BigEndian.Uint32(chunk[2:6]))
		case "SSND":
			if n < 8 {
				return fail()
			}
			offset := uint64(binary.BigEndian.Uint32(chunk[:4]))
			if offset > n-8 {
				return fail()
			}
			payload = chunk[8+offset:]
		}
		pos = start + n + (n % 2)
	}
	if spec.Channels <= 0 || spec.SampleRate <= 0 || frames == 0 || (spec.Bits != 16 && spec.Bits != 24 && spec.Bits != 32) {
		return fail()
	}
	expected := frames * uint64(spec.Channels) * uint64(spec.Bits/8)
	if expected != uint64(len(payload)) {
		return fail()
	}
	return spec, frames, payload, nil
}

func swap16(b []byte) []byte {
	out := make([]byte, len(b))
	for i := 0; i+1 < len(b); i += 2 {
		out[i], out[i+1] = b[i+1], b[i]
	}
	return out
}
