package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenRouterDefaultAllowsNativeFallback(t *testing.T) {
	t.Setenv("NARRATE_CONFIG", filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv("NARRATE_TTS_BACKEND", "")
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("NARRATE_TTS_API_KEY", "")
	t.Setenv("NARRATE_TTS_MODEL", "")
	cfg, _, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TTS.Backend != "openrouter" {
		t.Fatalf("default backend %q", cfg.TTS.Backend)
	}
	if cfg.TTS.APIKey != "" || cfg.TTS.Model != "" {
		t.Fatalf("unexpected implicit TTS config: %+v", cfg.TTS)
	}
	if err := cfg.Validate(false, true); err != nil {
		t.Fatalf("missing OpenRouter config must allow native fallback: %v", err)
	}
	if err := cfg.Validate(false, false); err != nil {
		t.Fatalf("script-only validation failed: %v", err)
	}
	cfg.Rate = 200
	if err := cfg.Validate(false, true); err == nil {
		t.Fatal("OpenRouter silently accepted native rate")
	}
}

func TestOpenRouterTTSConfigAndCredentialPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name, config, openrouterKey, ttsKey string
		wantKey, wantModel, wantBaseURL     string
	}{
		{name: "config values", config: `{"tts":{"model":"vendor/model","api_key":"config-key","base_url":"https://config.example"}}`, wantKey: "config-key", wantModel: "vendor/model", wantBaseURL: "https://config.example"},
		{name: "OpenRouter env replaces configured key", config: `{"tts":{"api_key":"config-key"}}`, openrouterKey: "router-key", wantKey: "router-key"},
		{name: "TTS env replaces OpenRouter env", openrouterKey: "router-key", ttsKey: "tts-key", wantKey: "tts-key"},
		{name: "TTS env replaces configured key", config: `{"tts":{"api_key":"config-key","model":"config-model","base_url":"https://config.example"}}`, ttsKey: "tts-key", wantKey: "tts-key", wantModel: "config-model", wantBaseURL: "https://config.example"},
		{name: "AI OpenRouter key fallback", config: `{"ai":{"provider":"openrouter","api_key":"ai-key"}}`, wantKey: "ai-key"},
		{name: "no OpenAI key leakage", config: `{"ai":{"provider":"openai","api_key":"openai-config-key"}}`, wantKey: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if tc.config != "" {
				if err := os.WriteFile(path, []byte(tc.config), 0600); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("NARRATE_CONFIG", path)
			t.Setenv("NARRATE_AI_PROVIDER", "")
			t.Setenv("OPENROUTER_API_KEY", tc.openrouterKey)
			t.Setenv("OPENAI_API_KEY", "openai-env-key")
			t.Setenv("NARRATE_AI_API_KEY", "")
			t.Setenv("NARRATE_TTS_API_KEY", tc.ttsKey)
			t.Setenv("NARRATE_TTS_MODEL", "")
			t.Setenv("NARRATE_TTS_BASE_URL", "")
			cfg, _, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.TTS.APIKey != tc.wantKey || cfg.TTS.Model != tc.wantModel || cfg.TTS.BaseURL != tc.wantBaseURL {
				t.Fatalf("got TTS key/model/base URL %q/%q/%q; want %q/%q/%q", cfg.TTS.APIKey, cfg.TTS.Model, cfg.TTS.BaseURL, tc.wantKey, tc.wantModel, tc.wantBaseURL)
			}
		})
	}
}

func TestOpenRouterTTSModelAndBaseURLEnvOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"tts":{"model":"config-model","base_url":"https://config.example"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NARRATE_CONFIG", path)
	t.Setenv("NARRATE_TTS_BACKEND", "")
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("NARRATE_TTS_API_KEY", "")
	t.Setenv("NARRATE_TTS_MODEL", "env-model")
	t.Setenv("NARRATE_TTS_BASE_URL", "https://env.example")
	cfg, _, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TTS.Model != "env-model" || cfg.TTS.BaseURL != "https://env.example" {
		t.Fatalf("environment overrides not applied: model=%q base_url=%q", cfg.TTS.Model, cfg.TTS.BaseURL)
	}
	if err := cfg.Validate(false, true); err != nil {
		t.Fatal(err)
	}
}

func TestPocketValidationRemainsUnchanged(t *testing.T) {
	t.Setenv("NARRATE_CONFIG", filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv("NARRATE_TTS_BACKEND", "pocket")
	t.Setenv("NARRATE_SPARK_URL", "")
	t.Setenv("NARRATE_DGX_SSH_HOST", "")
	cfg, _, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(false, true); err == nil {
		t.Fatal("missing DGX config accepted")
	}
	t.Setenv("NARRATE_SPARK_URL", "http://spark.test")
	t.Setenv("NARRATE_DGX_SSH_HOST", "user@host")
	cfg, _, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(false, true); err != nil {
		t.Fatal(err)
	}
	cfg.Rate = 200
	if err := cfg.Validate(false, true); err == nil {
		t.Fatal("Pocket silently accepted native rate")
	}
}

func TestSpeechSpeedConfiguration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"tts":{"speed":0.85}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NARRATE_CONFIG", path)
	t.Setenv("NARRATE_TTS_SPEED", "")
	cfg, _, err := Load()
	if err != nil || cfg.TTS.Speed != 0.85 {
		t.Fatalf("config speed: %v %v", cfg, err)
	}
	t.Setenv("NARRATE_TTS_SPEED", "0.9")
	cfg, _, err = Load()
	if err != nil || cfg.TTS.Speed != 0.9 {
		t.Fatalf("env speed: %v %v", cfg, err)
	}
	for _, value := range []string{"bad", "NaN", "+Inf", "0.49", "2.1", "0"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("NARRATE_TTS_SPEED", value)
			cfg, _, err := Load()
			if err == nil {
				err = cfg.Validate(false, true)
			}
			if err == nil {
				t.Fatal("invalid speed accepted")
			}
		})
	}
}
