package cli

import (
	"context"
	"example.com/narrate/internal/config"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Skip audio-mode tests entirely on non-macOS test hosts; script-only and
// discovery tests run everywhere with no TTS or credentials.
func onDarwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("native audio test requires macOS")
	}
}

func TestHelpNoCredentialsNoStdin(t *testing.T) {
	code := Run([]string{"--help"}, strings.NewReader(""), &out{}, &errW{})
	if code != 0 {
		t.Fatalf("help exit %d", code)
	}
}

func TestVersionExit(t *testing.T) {
	if code := Run([]string{"--version"}, strings.NewReader(""), &out{}, &errW{}); code != 0 {
		t.Fatalf("version exit %d", code)
	}
}

func TestVoiceQuestionMarkRejectedOnLinuxWithoutCrash(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("darwin has real say")
	}
	code := Run([]string{"--tts=native", "-v", "?"}, strings.NewReader(""), &out{}, &errW{})
	if code != 1 { // runtime failure: no /usr/bin/say
		t.Fatalf("expected exit 1, got %d", code)
	}
}

func TestScriptOnlyRequiresAIConfig(t *testing.T) {
	t.Setenv("NARRATE_AI_PROVIDER", "openai")
	t.Setenv("NARRATE_AI_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("NARRATE_AI_MODEL", "")
	t.Setenv("NARRATE_CONFIG", filepath.Join(t.TempDir(), "no-config.json"))
	t.Setenv("NARRATE_CACHE_DIR", t.TempDir())
	code := Run([]string{"--script-only", "hello"}, strings.NewReader(""), &out{}, &errW{})
	if code != 2 {
		t.Fatalf("expected usage exit 2 for missing AI config, got %d", code)
	}
}

func TestScriptOnlyRejectsO(t *testing.T) {
	t.Setenv("NARRATE_CONFIG", filepath.Join(t.TempDir(), "c.json"))
	code := Run([]string{"--script-only", "-o", "x.aiff", "hi"}, strings.NewReader(""), &out{}, &errW{})
	if code != 2 {
		t.Fatalf("expected exit 2, got %d", code)
	}
}

func TestScriptOnlyWritesToScriptOutNotStdout(t *testing.T) {
	dir := t.TempDir()
	mockAI := newMockAI(t)
	dest := filepath.Join(dir, "script.txt")
	t.Setenv("NARRATE_AI_PROVIDER", "openai")
	t.Setenv("NARRATE_AI_API_KEY", "test")
	t.Setenv("NARRATE_AI_MODEL", "test-model")
	t.Setenv("NARRATE_AI_BASE_URL", mockAI.URL)
	t.Setenv("NARRATE_CONFIG", filepath.Join(dir, "c.json"))
	t.Setenv("NARRATE_CACHE_DIR", filepath.Join(dir, "cache"))
	code := Run([]string{"--script-only", "--script-out", dest, "The uptime was 99.95% in Q3."}, strings.NewReader(""), &out{}, &errW{})
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("empty script file")
	}
}

func TestVerbatimNoAIRewrite(t *testing.T) {
	dir := t.TempDir()
	// Point at an address that would fail if contacted.
	t.Setenv("NARRATE_AI_BASE_URL", "http://127.0.0.1:1")
	t.Setenv("NARRATE_CONFIG", filepath.Join(dir, "c.json"))
	t.Setenv("NARRATE_CACHE_DIR", dir)
	// No credentials set; verbatim must not need them.
	var so, se strings.Builder
	code := Run([]string{"--verbatim", "--script-only", "plain words"}, strings.NewReader(""), &so, &se)
	if code != 0 {
		t.Fatalf("exit %d stderr=%s", code, se.String())
	}
	if !strings.Contains(so.String(), "plain words") {
		t.Fatalf("verbatim output missing: %q", so.String())
	}
}

func TestScriptOnlyRefusesExistingDestWithoutForce(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "s.txt")
	os.WriteFile(dest, []byte("old"), 0o600)
	mockAI := newMockAI(t)
	t.Setenv("NARRATE_AI_PROVIDER", "openai")
	t.Setenv("NARRATE_AI_API_KEY", "k")
	t.Setenv("NARRATE_AI_MODEL", "m")
	t.Setenv("NARRATE_AI_BASE_URL", mockAI.URL)
	t.Setenv("NARRATE_CONFIG", filepath.Join(dir, "c.json"))
	t.Setenv("NARRATE_CACHE_DIR", dir)
	var so, se strings.Builder
	code := Run([]string{"--script-only", "--script-out", dest, "text"}, strings.NewReader(""), &so, &se)
	if code != 1 {
		t.Fatalf("expected runtime failure, got %d (%s)", code, se.String())
	}
	data, _ := os.ReadFile(dest)
	if string(data) != "old" {
		t.Fatal("existing destination was modified")
	}
}

func TestAmbiguousInput(t *testing.T) {
	t.Setenv("NARRATE_CONFIG", filepath.Join(t.TempDir(), "c.json"))
	code := Run([]string{"-f", "x.txt", "also", "text"}, strings.NewReader(""), &out{}, &errW{})
	if code != 2 {
		t.Fatalf("expected 2, got %d", code)
	}
}

func TestUnsupportedSayFlagsRejected(t *testing.T) {
	t.Setenv("NARRATE_CONFIG", filepath.Join(t.TempDir(), "c.json"))
	for _, bad := range [][]string{
		{"-n", "host"},
		{"-a", "dev"},
		{"-i"},
	} {
		if code := Run(bad, strings.NewReader(""), &out{}, &errW{}); code != 2 {
			t.Errorf("flag %v: expected exit 2, got %d", bad, code)
		}
	}
}

func TestNativeAudioSmokeIfSayInstalled(t *testing.T) {
	onDarwin(t)
	if _, err := exec.LookPath("/usr/bin/say"); err != nil {
		t.Skip("no /usr/bin/say")
	}
	dir := t.TempDir()
	t.Setenv("NARRATE_CONFIG", filepath.Join(dir, "c.json"))
	t.Setenv("NARRATE_CACHE_DIR", dir)
	outAiff := filepath.Join(dir, "out.aiff")
	var so, se strings.Builder
	code := Run([]string{"--tts=native", "--verbatim", "-o", outAiff, "--force", "Two sentences. For the smoke test."}, strings.NewReader(""), &so, &se)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, se.String())
	}
	data, err := os.ReadFile(outAiff)
	if err != nil || len(data) < 100 {
		t.Fatalf("bad aiff: %v", err)
	}
	if string(data[0:4]) != "FORM" {
		t.Fatal("not AIFF")
	}
}

// --- helpers ---

type out struct{}

func (out) Write(p []byte) (int, error) { return len(p), nil }

type errW struct{ buf strings.Builder }

func (e *errW) Write(p []byte) (int, error) { return e.buf.Write(p) }
func (e *errW) String() string              { return e.buf.String() }

func TestNativeExports(t *testing.T) {
	onDarwin(t)
	for _, ext := range []string{"aiff", "wav"} {
		t.Run(ext, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("NARRATE_CONFIG", filepath.Join(dir, "missing"))
			dest := filepath.Join(dir, "audio."+ext)
			script := filepath.Join(dir, "script.txt")
			source := "First paragraph.\n\nSecond paragraph."
			var so, se strings.Builder
			code := Run([]string{"--tts=native", "--verbatim", "-o", dest, "--script-out", script, "--artifacts-dir", filepath.Join(dir, "artifacts"), source}, strings.NewReader(""), &so, &se)
			if code != 0 {
				t.Fatalf("exit %d: %s", code, se.String())
			}
			data, err := os.ReadFile(script)
			if err != nil || !strings.Contains(string(data), "Second paragraph.") {
				t.Fatalf("script: %s %v", data, err)
			}
			transcript, err := os.ReadFile(filepath.Join(dir, "artifacts", "transcript.md"))
			if err != nil || !strings.Contains(string(transcript), "## Source\n\n"+source) {
				t.Fatalf("source missing: %s %v", transcript, err)
			}
			if out, err := exec.Command("/usr/bin/afinfo", dest).CombinedOutput(); err != nil {
				t.Fatalf("afinfo: %s %v", out, err)
			}
		})
	}
}

func TestFailedSynthesisPreservesOutput(t *testing.T) {
	onDarwin(t)
	dir := t.TempDir()
	t.Setenv("NARRATE_CONFIG", filepath.Join(dir, "missing"))
	dest := filepath.Join(dir, "audio.aiff")
	if err := os.WriteFile(dest, []byte("previous recording"), 0600); err != nil {
		t.Fatal(err)
	}
	var so, se strings.Builder
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := runAudio(ctx, &so, &se, "Hello.", "Hello.", options{cfg: &config.Config{}, outputFile: dest, force: true})
	if err == nil {
		t.Fatal("canceled synthesis unexpectedly succeeded")
	}

	data, err := os.ReadFile(dest)
	if err != nil || string(data) != "previous recording" {
		t.Fatalf("output destroyed: %q %v", data, err)
	}
}

func TestUnsupportedDurationRejected(t *testing.T) {
	var so, se strings.Builder
	if code := Run([]string{"--minutes=2", "--verbatim", "--script-only", "Hello."}, strings.NewReader(""), &so, &se); code != ExitUsage {
		t.Fatalf("exit %d: %s", code, se.String())
	}
}
