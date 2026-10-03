package cli

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/narrate-it/narrate/internal/config"
)

func TestBackendPriority(t *testing.T) {
	for _, tc := range []struct {
		name      string
		backend   string
		failures  map[string]error
		want      []string
		wantError bool
	}{
		{"first succeeds", "auto", nil, []string{"pocket"}, false},
		{"first unavailable", "auto", map[string]error{"pocket": &backendUnavailable{"offline"}}, []string{"pocket", "openrouter"}, false},
		{"two unavailable", "auto", map[string]error{"pocket": &backendUnavailable{"offline"}, "openrouter": &speechUnavailable{"refused"}}, []string{"pocket", "openrouter", "native"}, false},
		{"local failure stops", "auto", map[string]error{"pocket": errors.New("disk full")}, []string{"pocket"}, true},
		{"explicit single", "openrouter", map[string]error{"openrouter": &speechUnavailable{"refused"}}, []string{"openrouter"}, true},
		{"custom order", "native,openrouter", nil, []string{"native"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{TTS: config.TTSConfig{Backend: tc.backend, Backends: []string{"pocket", "openrouter", "native"}, Speed: 0.85, Voices: map[string]string{"pocket": "michael", "openrouter": "provider-voice", "native": "Samantha"}}}
			var attempted []string
			var stderr strings.Builder
			err := tryBackendChain(context.Background(), &stderr, options{cfg: cfg}, func(_ context.Context, name string, o options) error {
				attempted = append(attempted, name)
				if o.cfg.TTS.Backend != name || o.cfg.TTS.Speed != .85 || o.cfg.TTS.Voice != cfg.TTS.Voices[name] {
					t.Fatalf("attempt lost backend settings: %+v", o.cfg.TTS)
				}
				return tc.failures[name]
			})
			if (err != nil) != tc.wantError || !reflect.DeepEqual(attempted, tc.want) {
				t.Fatalf("attempts=%v err=%v", attempted, err)
			}
		})
	}
}

func TestScriptOnlyIgnoresSpeechBackendConfiguration(t *testing.T) {
	t.Setenv("NARRATE_CONFIG", filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv("NARRATE_TTS_BACKEND", "invalid-speech-backend")
	var stdout, stderr strings.Builder
	code := Run([]string{"--script-only", "--verbatim", "Exact text."}, strings.NewReader(""), &stdout, &stderr)
	if code != ExitOK || strings.TrimSpace(stdout.String()) != "Exact text." {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}

func TestLocalDecoderFailureDoesNotBecomeBackendUnavailability(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ffmpeg"), []byte("#!/bin/sh\necho 'local decoder failure' >&2\nexit 2\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Write([]byte("audio"))
	}))
	t.Cleanup(srv.Close)
	cfg := &config.Config{TTS: config.TTSConfig{Backend: "openrouter", APIKey: "test", Model: "test", Voice: "test", BaseURL: srv.URL}}
	err := synthOpenRouterParagraph(context.Background(), "Hello.", filepath.Join(dir, "clip.aiff"), options{cfg: cfg})
	var unavailable *speechUnavailable
	if err == nil || errors.As(err, &unavailable) {
		t.Fatalf("local failure became unavailable: %v", err)
	}
}

func TestBackendCancellationDoesNotAdvance(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := &config.Config{TTS: config.TTSConfig{Backend: "auto", Backends: []string{"pocket", "native"}}}
	calls := 0
	var stderr strings.Builder
	err := tryBackendChain(ctx, &stderr, options{cfg: cfg}, func(_ context.Context, _ string, _ options) error {
		calls++
		cancel()
		return &backendUnavailable{"interrupted"}
	})
	if !errors.Is(err, context.Canceled) || calls != 1 || stderr.Len() != 0 {
		t.Fatalf("cancellation advanced: %d %v %s", calls, err, stderr.String())
	}
}
