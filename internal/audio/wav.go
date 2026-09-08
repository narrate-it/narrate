// Package audio implements PCM assembly, WAV/AIFF, and clip handling.
package audio

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// ClipSpec describes a validated PCM clip.
type ClipSpec struct {
	SampleRate int
	Channels   int
	Bits       int // 16 or 32 (float handled as 32-bit PCM int here for simplicity)
}

// WavWriter streams PCM samples into a RIFF/WAVE file using header patching,
// so long recordings never need to be fully in memory.
type WavWriter struct {
	w          io.WriteSeeker
	spec       ClipSpec
	dataBytes  int
	headerSize int64
}

// NewWavWriter writes the 44-byte canonical header with placeholder sizes.
func NewWavWriter(w io.WriteSeeker, spec ClipSpec) (*WavWriter, error) {
	if spec.SampleRate <= 0 || spec.Channels <= 0 {
		return nil, fmt.Errorf("audio: invalid clip spec %+v", spec)
	}
	bits := spec.Bits
	if bits == 0 {
		bits = 16
	}
	spec.Bits = bits
	ww := &WavWriter{w: w, spec: spec}
	if err := ww.writeHeader(0); err != nil {
		return nil, err
	}
	ww.headerSize = 44
	return ww, nil
}

func (ww *WavWriter) writeHeader(dataLen uint32) error {
	h := make([]byte, 44)
	copy(h[0:4], "RIFF")
	binary.LittleEndian.PutUint32(h[4:8], 36+dataLen)
	copy(h[8:12], "WAVE")
	copy(h[12:16], "fmt ")
	binary.LittleEndian.PutUint32(h[16:20], 16)
	binary.LittleEndian.PutUint16(h[20:22], 1) // PCM
	binary.LittleEndian.PutUint16(h[22:24], uint16(ww.spec.Channels))
	binary.LittleEndian.PutUint32(h[24:28], uint32(ww.spec.SampleRate))
	byteRate := ww.spec.SampleRate * ww.spec.Channels * ww.spec.Bits / 8
	binary.LittleEndian.PutUint32(h[28:32], uint32(byteRate))
	blockAlign := ww.spec.Channels * ww.spec.Bits / 8
	binary.LittleEndian.PutUint16(h[32:34], uint16(blockAlign))
	binary.LittleEndian.PutUint16(h[34:36], uint16(ww.spec.Bits))
	copy(h[36:40], "data")
	binary.LittleEndian.PutUint32(h[40:44], dataLen)
	_, err := ww.w.Write(h)
	return err
}

// Write appends PCM samples (interleaved, already in target format bytes).
func (ww *WavWriter) Write(p []byte) (int, error) {
	n, err := ww.w.Write(p)
	ww.dataBytes += n
	return n, err
}

// WriteSilence appends n milliseconds of digital silence.
func (ww *WavWriter) WriteSilence(ms int) error {
	if ms < 0 {
		return fmt.Errorf("audio: negative silence duration")
	}
	frames := ww.spec.SampleRate * ms / 1000
	sil := make([]byte, frames*ww.spec.Channels*ww.spec.Bits/8)
	_, err := ww.Write(sil)
	return err
}

// Finalize patches sizes; safe exactly once.
func (ww *WavWriter) Finalize() error {
	if ww.dataBytes%2 != 0 { // pad to even
		if _, err := ww.w.Write([]byte{0}); err != nil {
			return err
		}
		ww.dataBytes++
	}
	if _, err := ww.w.Seek(0, io.SeekStart); err != nil {
		return err
	}
	return ww.writeHeader(uint32(ww.dataBytes))
}

// DurationSeconds returns total audio duration of what has been written.
func (ww *WavWriter) DurationSeconds() float64 {
	bps := float64(ww.spec.SampleRate * ww.spec.Channels * ww.spec.Bits / 8)
	if bps == 0 {
		return 0
	}
	return float64(ww.dataBytes) / bps
}

// MaxWavDataBytes is the practical 32-bit RIFF limit for the data chunk.
const MaxWavDataBytes = 0xFFFFFFF0 - 44

// ValidateClip performs a header sanity check on a WAV clip read from r.
// Returns the parsed spec and sample-frame count.
func ValidateClip(r io.Reader) (ClipSpec, uint64, error) {
	h := make([]byte, 44)
	if _, err := io.ReadFull(r, h); err != nil {
		return ClipSpec{}, 0, fmt.Errorf("audio: short clip header: %w", err)
	}
	if string(h[0:4]) != "RIFF" || string(h[8:12]) != "WAVE" || string(h[12:16]) != "fmt " {
		return ClipSpec{}, 0, fmt.Errorf("audio: not a RIFF/WAVE clip")
	}
	audioFmt := binary.LittleEndian.Uint16(h[20:22])
	if audioFmt != 1 {
		return ClipSpec{}, 0, fmt.Errorf("audio: unsupported WAV format %d (want PCM)", audioFmt)
	}
	spec := ClipSpec{
		Channels:   int(binary.LittleEndian.Uint16(h[22:24])),
		SampleRate: int(binary.LittleEndian.Uint32(h[24:28])),
		Bits:       int(binary.LittleEndian.Uint16(h[34:36])),
	}
	if spec.Channels <= 0 || spec.SampleRate <= 0 || (spec.Bits != 16 && spec.Bits != 32) {
		return ClipSpec{}, 0, fmt.Errorf("audio: implausible clip spec %+v", spec)
	}
	dataLen := binary.LittleEndian.Uint32(h[40:44])
	if dataLen == 0 {
		return ClipSpec{}, 0, fmt.Errorf("audio: empty clip payload")
	}
	frames := uint64(dataLen) / uint64(spec.Channels*spec.Bits/8)
	return spec, frames, nil
}

// SamplesFinite checks 32-bit float samples; for int16 PCM it's a no-op
// (all int16 values are finite). Present for float pipelines and tests.
func SamplesFinite(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}
