package audio

import (
	"encoding/binary"
	"fmt"
	"io"
)

// AiffWriter streams big-endian PCM into AIFF (FORM/COMM/SSND), patching sizes
// at finalize. Used for the macOS-native default output.
//
// Note: the generating session's first draft of this file contained a broken,
// unfinished extendedFloat implementation. Per its own correction note, this
// file ships the clean implementation described there instead.
type AiffWriter struct {
	w         io.WriteSeeker
	spec      ClipSpec
	dataBytes int
}

// extendedFloat encodes v as IEEE 754 80-bit extended (big-endian), the
// format AIFF's COMM chunk uses for sample rate.
func extendedFloat(v float64) [10]byte {
	var out [10]byte
	if v == 0 {
		return out
	}
	sign := uint16(0)
	if v < 0 {
		sign = 0x8000
		v = -v
	}
	exp := 0
	for v >= 2 {
		v /= 2
		exp++
	}
	for v < 1 {
		v *= 2
		exp--
	}
	be := uint16(exp+16383) | sign
	// v is now in [1, 2); the top bit of frac is the explicit integer bit.
	frac := uint64((v - 1) * float64(uint64(1)<<63))
	frac |= 1 << 63
	out[0] = byte(be >> 8)
	out[1] = byte(be)
	binary.BigEndian.PutUint64(out[2:10], frac)
	return out
}

// decodeExtended converts IEEE 754 80-bit extended (big-endian) to float64.
func decodeExtended(b []byte) float64 {
	if len(b) < 10 {
		return 0
	}
	be := uint16(b[0])<<8 | uint16(b[1])
	sign := 1.0
	if be&0x8000 != 0 {
		sign = -1
	}
	exp := int(be&0x7FFF) - 16383
	frac := uint64(b[2])<<56 | uint64(b[3])<<48 | uint64(b[4])<<40 | uint64(b[5])<<32 |
		uint64(b[6])<<24 | uint64(b[7])<<16 | uint64(b[8])<<8 | uint64(b[9])
	// Integer bit is bit 63 of frac; value = frac * 2^(exp-63)
	return sign * float64(frac) * pow2(exp-63)
}

func pow2(e int) float64 {
	r := 1.0
	if e >= 0 {
		for i := 0; i < e; i++ {
			r *= 2
		}
	} else {
		for i := 0; i < -e; i++ {
			r /= 2
		}
	}
	return r
}

// NewAiffWriter writes the FORM header with placeholder sizes.
func NewAiffWriter(w io.WriteSeeker, spec ClipSpec) (*AiffWriter, error) {
	if spec.SampleRate <= 0 || spec.Channels <= 0 {
		return nil, fmt.Errorf("audio: invalid clip spec %+v", spec)
	}
	if spec.Bits == 0 {
		spec.Bits = 16
	}
	if spec.Bits != 16 && spec.Bits != 24 && spec.Bits != 32 {
		return nil, fmt.Errorf("audio: unsupported AIFF bit depth %d", spec.Bits)
	}
	aw := &AiffWriter{w: w, spec: spec}
	if err := aw.writeHeader(0); err != nil {
		return nil, err
	}
	return aw, nil
}

func (aw *AiffWriter) writeHeader(ssndData uint32) error {
	bits := aw.spec.Bits
	blockAlign := aw.spec.Channels * bits / 8
	formLen := uint32(4 + 26 + 16 + ssndData) // "AIFF" + COMM chunk + SSND chunk

	hdr := make([]byte, 0, 12+26+16)
	hdr = append(hdr, "FORM"...)
	hdr = binary.BigEndian.AppendUint32(hdr, formLen)
	hdr = append(hdr, "AIFF"...)

	// COMM chunk: 18 bytes of payload.
	hdr = append(hdr, "COMM"...)
	hdr = binary.BigEndian.AppendUint32(hdr, 18)
	hdr = binary.BigEndian.AppendUint16(hdr, uint16(aw.spec.Channels))
	numFrames := uint32(0)
	if blockAlign > 0 {
		numFrames = ssndData / uint32(blockAlign)
	}
	hdr = binary.BigEndian.AppendUint32(hdr, numFrames)
	hdr = binary.BigEndian.AppendUint16(hdr, uint16(bits))
	ext := extendedFloat(float64(aw.spec.SampleRate))
	hdr = append(hdr, ext[:]...)

	// SSND chunk.
	hdr = append(hdr, "SSND"...)
	hdr = binary.BigEndian.AppendUint32(hdr, 8+ssndData)
	hdr = binary.BigEndian.AppendUint32(hdr, 0) // offset
	hdr = binary.BigEndian.AppendUint32(hdr, 0) // block size

	_, err := aw.w.Write(hdr)
	return err
}

// Write appends PCM sample bytes.
func (aw *AiffWriter) Write(p []byte) (int, error) {
	n, err := aw.w.Write(p)
	aw.dataBytes += n
	return n, err
}

// WriteSilence appends ms milliseconds of silence.
func (aw *AiffWriter) WriteSilence(ms int) error {
	if ms < 0 {
		return fmt.Errorf("audio: negative silence duration")
	}
	frames := aw.spec.SampleRate * ms / 1000
	_, err := aw.Write(make([]byte, frames*aw.spec.Channels*aw.spec.Bits/8))
	return err
}

// Finalize patches sizes.
func (aw *AiffWriter) Finalize() error {
	if _, err := aw.w.Seek(0, io.SeekStart); err != nil {
		return err
	}
	return aw.writeHeader(uint32(aw.dataBytes))
}

// DurationSeconds returns the duration of written audio.
func (aw *AiffWriter) DurationSeconds() float64 {
	bps := float64(aw.spec.SampleRate * aw.spec.Channels * aw.spec.Bits / 8)
	if bps == 0 {
		return 0
	}
	return float64(aw.dataBytes) / bps
}

// ValidateAIFF parses an AIFF header and returns its spec and frame count.
func ValidateAIFF(r io.Reader) (ClipSpec, uint64, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return ClipSpec{}, 0, err
	}
	spec, frames, _, err := parseAIFF(data)
	return spec, frames, err
}
