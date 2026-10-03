package cli

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeSpeechSpeedOption(t *testing.T) {
	onDarwin(t)
	dir := t.TempDir()
	t.Setenv("NARRATE_CONFIG", filepath.Join(dir, "missing.json"))
	t.Setenv("NARRATE_TTS_SPEED", "1.25")
	var stdout, stderr strings.Builder
	dest := filepath.Join(dir, "slow.aiff")
	artifacts := filepath.Join(dir, "artifacts")
	code := Run([]string{"--tts=native", "--verbatim", "--speed=0.85", "-o", dest, "--artifacts-dir", artifacts, "Hello from Narrate."}, strings.NewReader(""), &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("exit=%d: %s", code, stderr.String())
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(artifacts, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Speed float64 `json:"speed"`
	}
	if json.Unmarshal(data, &manifest) != nil || manifest.Speed != 0.85 {
		t.Fatalf("flag did not override env: %s", data)
	}
}

func TestSpeechSpeedPreservesPitchAndMeasuresDuration(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg required")
	}
	clip := filepath.Join(t.TempDir(), "tone.aiff")
	err := exec.Command("ffmpeg", "-nostdin", "-v", "error", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=22050:duration=3", "-c:a", "pcm_s16be", clip).Run()
	if err != nil {
		t.Fatal(err)
	}
	spec, _, err := probeAIFF(clip)
	if err != nil {
		t.Fatal(err)
	}
	before := clipDuration(clip, spec)
	if err := adjustSpeechSpeed(context.Background(), clip, 0.85); err != nil {
		t.Fatal(err)
	}
	afterSpec, _, err := probeAIFF(clip)
	if err != nil || afterSpec != spec {
		t.Fatalf("format changed: %v %v", afterSpec, err)
	}
	after := clipDuration(clip, spec)
	if math.Abs(after-before/0.85) > 0.08 {
		t.Fatalf("wrong duration: before=%g after=%g", before, after)
	}
	pcm, err := exec.Command("ffmpeg", "-nostdin", "-v", "error", "-i", clip, "-f", "s16le", "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	// Count positive zero crossings in the interior, avoiding filter edge effects.
	start, end := spec.SampleRate/2, len(pcm)/2-spec.SampleRate/2
	crossings := 0
	for i := start + 1; i < end; i++ {
		prev := int16(binary.LittleEndian.Uint16(pcm[(i-1)*2:]))
		next := int16(binary.LittleEndian.Uint16(pcm[i*2:]))
		if prev <= 0 && next > 0 {
			crossings++
		}
	}
	hz := float64(crossings) * float64(spec.SampleRate) / float64(end-start)
	if math.Abs(hz-440) > 3 {
		t.Fatalf("pitch changed: %g Hz", hz)
	}
}

func TestInvalidSpeechSpeedFailsBeforeGeneration(t *testing.T) {
	t.Setenv("NARRATE_CONFIG", filepath.Join(t.TempDir(), "missing.json"))
	for _, speed := range []string{"0", "0.49", "2.01", "NaN", "Inf"} {
		t.Run(speed, func(t *testing.T) {
			var stdout, stderr strings.Builder
			code := Run([]string{"--verbatim", "--speed=" + speed, "Hello."}, strings.NewReader(""), &stdout, &stderr)
			if code != ExitUsage || !strings.Contains(stderr.String(), "between 0.5 and 2") {
				t.Fatalf("exit=%d: %s", code, stderr.String())
			}
		})
	}
}
