package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/narrate-it/narrate/internal/config"
)

func TestOpenRouterMissingConfigFallsBack(t *testing.T) {
	onDarwin(t)
	dir := t.TempDir()
	t.Setenv("NARRATE_CONFIG", filepath.Join(dir, "missing.json"))
	t.Setenv("NARRATE_TTS_BACKEND", "")
	t.Setenv("NARRATE_TTS_MODEL", "")
	t.Setenv("NARRATE_TTS_API_KEY", "")
	t.Setenv("NARRATE_AI_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "")
	var stdout, stderr strings.Builder
	dest := filepath.Join(dir, "fallback.aiff")
	code := Run([]string{"--tts=openrouter,native", "--verbatim", "-o", dest, "Hello."}, strings.NewReader(""), &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	data, err := os.ReadFile(dest)
	if err != nil || len(data) < 100 || string(data[:4]) != "FORM" {
		t.Fatalf("invalid fallback audio: %v", err)
	}
	if !strings.Contains(stderr.String(), "trying native") {
		t.Fatalf("missing fallback notice: %s", stderr.String())
	}
}

func TestOpenRouterSpeechAndFallback(t *testing.T) {
	onDarwin(t)
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg required")
	}
	mp3 := filepath.Join(t.TempDir(), "fixture.mp3")
	if err := exec.Command("ffmpeg", "-nostdin", "-v", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=0.15", mp3).Run(); err != nil {
		t.Fatal(err)
	}
	audio, err := os.ReadFile(mp3)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		status  int
		audio   []byte
		backend string
	}{
		{"success", 200, audio, "openrouter"},
		{"refused", 429, nil, "native"},
		{"invalid audio", 200, []byte("invalid audio"), "native"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != "/audio/speech" || r.Header.Get("Authorization") != "Bearer speech-test" {
					t.Errorf("unexpected speech request")
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["model"] != "tts-test" || body["voice"] != "alloy" || body["input"] != "Hello." {
					t.Errorf("wrong model, voice or source")
				}
				w.Header().Set("Content-Type", "audio/mpeg")
				w.WriteHeader(tc.status)
				w.Write(tc.audio)
			}))
			defer srv.Close()
			t.Setenv("NARRATE_CONFIG", filepath.Join(dir, "missing.json"))
			t.Setenv("NARRATE_TTS_BACKEND", "openrouter,native")
			t.Setenv("NARRATE_TTS_MODEL", "tts-test")
			t.Setenv("NARRATE_TTS_API_KEY", "speech-test")
			t.Setenv("NARRATE_TTS_BASE_URL", srv.URL)
			t.Setenv("NARRATE_TTS_VOICE", "alloy")
			t.Setenv("NARRATE_TTS_SPEED", "0.85")
			var stdout, stderr strings.Builder
			dest := filepath.Join(dir, "result.mp3")
			artifacts := filepath.Join(dir, "artifacts")
			code := Run([]string{"--verbatim", "-o", dest, "--artifacts-dir", artifacts, "Hello."}, strings.NewReader(""), &stdout, &stderr)
			if code != ExitOK || calls != 1 {
				t.Fatalf("exit=%d calls=%d: %s", code, calls, stderr.String())
			}
			if err := exec.Command("ffmpeg", "-nostdin", "-v", "error", "-i", dest, "-f", "null", "-").Run(); err != nil {
				t.Fatalf("invalid output: %v", err)
			}
			data, err := os.ReadFile(filepath.Join(artifacts, "manifest.json"))
			if err != nil {
				t.Fatal(err)
			}
			var manifest struct {
				Backend string  `json:"tts_backend"`
				Speed   float64 `json:"speed"`
			}
			if json.Unmarshal(data, &manifest) != nil || manifest.Backend != tc.backend {
				t.Fatalf("wrong actual backend: %s", data)
			}
			if manifest.Speed != 0.85 {
				t.Fatalf("speed lost on %s: %s", tc.backend, data)
			}
			if strings.Contains(stderr.String(), "trying native") != (tc.backend == "native") {
				t.Fatalf("wrong fallback notice: %s", stderr.String())
			}
			// Existing output must fail before another provider call.
			code = Run([]string{"--verbatim", "-o", dest, "Hello."}, strings.NewReader(""), &stdout, &stderr)
			if code != ExitRuntime || calls != 1 {
				t.Fatalf("overwrite called provider: exit=%d calls=%d", code, calls)
			}
		})
	}
}

func TestOpenRouterCanceledDoesNotFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout, stderr strings.Builder
	err := runPreferredAudio(ctx, &stdout, &stderr, "Hello.", "Hello.", options{cfg: &config.Config{TTS: config.TTSConfig{Backend: "openrouter"}}})
	if err == nil || strings.Contains(stderr.String(), "trying native") {
		t.Fatalf("cancellation triggered fallback: %v %s", err, stderr.String())
	}
}

func TestSpeechChunksPreserveUnicode(t *testing.T) {
	text := strings.Repeat("Hello 世界. ", 1600)
	chunks := speechChunks(text)
	if strings.Join(chunks, "") != text {
		t.Fatal("chunking changed text")
	}
	for _, chunk := range chunks {
		if len([]rune(chunk)) > 4000 {
			t.Fatal("oversized speech request")
		}
	}
}

func TestOpenRouterRewriteFailureReadsSourceNatively(t *testing.T) {
	onDarwin(t)
	dir := t.TempDir()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected speech after rewrite failed")
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	t.Setenv("NARRATE_CONFIG", filepath.Join(dir, "missing.json"))
	t.Setenv("NARRATE_TTS_BACKEND", "openrouter,native")
	t.Setenv("NARRATE_AI_PROVIDER", "openrouter")
	t.Setenv("NARRATE_AI_API_KEY", "test-key")
	t.Setenv("NARRATE_AI_MODEL", "test-model")
	t.Setenv("NARRATE_AI_BASE_URL", srv.URL)
	t.Setenv("NARRATE_CACHE_DIR", filepath.Join(dir, "cache"))
	var stdout, stderr strings.Builder
	dest := filepath.Join(dir, "fallback.aiff")
	code := Run([]string{"-o", dest, "--script-out", filepath.Join(dir, "script.txt"), "Original text."}, strings.NewReader(""), &stdout, &stderr)
	if code != ExitOK || calls != 1 || !strings.Contains(stderr.String(), "using original text") {
		t.Fatalf("exit=%d calls=%d: %s", code, calls, stderr.String())
	}
	data, err := os.ReadFile(filepath.Join(dir, "script.txt"))
	if err != nil || strings.TrimSpace(string(data)) != "Original text." {
		t.Fatalf("source changed: %s %v", data, err)
	}
}
