package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStreamingIsOptIn(t *testing.T) {
	onDarwin(t)
	for _, tc := range []struct {
		name string
		args []string
		want bool
	}{
		{"default waits", nil, false},
		{"explicit streaming", []string{"--stream"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			capture := filepath.Join(dir, "request.json")
			// Stop at the driver boundary, before any remote job or playback.
			for _, name := range []string{"python3", "ffmpeg", "scp"} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n/bin/cat > \"$NARRATE_TEST_CAPTURE\"\nexit 1\n"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("PATH", dir)
			t.Setenv("NARRATE_TEST_CAPTURE", capture)
			t.Setenv("NARRATE_CONFIG", filepath.Join(dir, "missing"))
			t.Setenv("NARRATE_CACHE_DIR", filepath.Join(dir, "cache"))
			t.Setenv("NARRATE_TTS_BACKEND", "pocket")
			t.Setenv("NARRATE_SPARK_URL", "http://spark.test")
			t.Setenv("NARRATE_DGX_SSH_HOST", "user@host")
			args := append([]string{"--verbatim"}, tc.args...)
			args = append(args, "A complete sentence.")
			var stdout, stderr strings.Builder
			if code := Run(args, strings.NewReader(""), &stdout, &stderr); code != ExitRuntime {
				t.Fatalf("exit %d: %s", code, stderr.String())
			}
			data, err := os.ReadFile(capture)
			if err != nil {
				t.Fatalf("driver not reached: %v: %s", err, stderr.String())
			}
			var cfg struct {
				Playback bool `json:"playback"`
			}
			if err := json.Unmarshal(data, &cfg); err != nil {
				t.Fatal(err)
			}
			if cfg.Playback != tc.want {
				t.Fatalf("playback=%v, want %v", cfg.Playback, tc.want)
			}
		})
	}
}

func TestStreamRejectsNonPlaybackModes(t *testing.T) {
	t.Setenv("NARRATE_CONFIG", filepath.Join(t.TempDir(), "missing"))
	for _, args := range [][]string{
		{"--stream", "--script-only", "--verbatim", "Hello."},
		{"--stream", "-o", "out.mp3", "--verbatim", "Hello."},
		{"--stream", "--tts=native", "--verbatim", "Hello."},
	} {
		var stdout, stderr strings.Builder
		if code := Run(args, strings.NewReader(""), &stdout, &stderr); code != ExitUsage {
			t.Fatalf("%v: exit %d: %s", args, code, stderr.String())
		}
	}
}
