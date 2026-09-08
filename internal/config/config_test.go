package config

import (
	"path/filepath"
	"testing"
)

func TestPocketDefaultAndConfiguration(t *testing.T) {
	t.Setenv("NARRATE_CONFIG", filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv("NARRATE_TTS_BACKEND", "")
	t.Setenv("NARRATE_SPARK_URL", "")
	t.Setenv("NARRATE_DGX_SSH_HOST", "")
	cfg, _, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TTS.Backend != "pocket" {
		t.Fatalf("default backend %q", cfg.TTS.Backend)
	}
	if err := cfg.Validate(false, false); err != nil {
		t.Fatalf("script-only should not need DGX: %v", err)
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
