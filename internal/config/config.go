// Package config resolves configuration with precedence:
// flags > environment > user config > defaults.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Config is the resolved runtime configuration. Credentials live only here,
// in memory, and are never persisted or logged.
type Config struct {
	AI             AIConfig  `json:"ai"`
	TTS            TTSConfig `json:"tts"`
	CacheDir       string    `json:"cache_dir"`
	ParagraphGapMs int       `json:"paragraph_gap_ms"`
	Rate           int       `json:"-"` // flag-only
}

type AIConfig struct {
	Provider string `json:"provider,omitempty"`
	APIKey   string `json:"api_key,omitempty"`
	BaseURL  string `json:"base_url,omitempty"`
	Model    string `json:"model,omitempty"`
}

type TTSConfig struct {
	Backend  string `json:"backend,omitempty"` // native | pocket
	Voice    string `json:"voice,omitempty"`
	SSHUID   int    `json:"ssh_uid,omitempty"`
	SparkURL string `json:"spark_url,omitempty"`
	SSHHost  string `json:"ssh_host,omitempty"`
}

// Source describes where configuration came from (for diagnostics).
type Source struct{ ConfigFile string }

// Load reads the user config file (no secrets required to exist) and applies
// environment overrides. Flags are applied by the CLI on top of this.
func Load() (*Config, Source, error) {
	cfg := &Config{
		TTS:            TTSConfig{Backend: "pocket", SSHUID: 1000},
		ParagraphGapMs: 650,
	}
	src := Source{}
	path := configPath()
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, src, fmt.Errorf("config: parsing %s: %w", path, err)
		}
		src.ConfigFile = path
	} else if !os.IsNotExist(err) {
		return nil, src, fmt.Errorf("config: reading %s: %w", path, err)
	}

	if v := os.Getenv("NARRATE_AI_PROVIDER"); v != "" {
		cfg.AI.Provider = v
	}
	if cfg.AI.Provider == "" {
		cfg.AI.Provider = "openrouter"
	}
	keyEnv := "OPENROUTER_API_KEY"
	if cfg.AI.Provider == "openai" {
		keyEnv = "OPENAI_API_KEY"
	}
	if v := os.Getenv(keyEnv); v != "" {
		cfg.AI.APIKey = v
	}
	if v := os.Getenv("NARRATE_AI_API_KEY"); v != "" {
		cfg.AI.APIKey = v
	}

	if v := os.Getenv("NARRATE_AI_MODEL"); v != "" {
		cfg.AI.Model = v
	}
	if v := os.Getenv("NARRATE_AI_BASE_URL"); v != "" {
		cfg.AI.BaseURL = v
	}
	if v := os.Getenv("NARRATE_TTS_BACKEND"); v != "" {
		cfg.TTS.Backend = v
	}
	if v := os.Getenv("NARRATE_TTS_VOICE"); v != "" {
		cfg.TTS.Voice = v
	}
	if v := os.Getenv("NARRATE_SPARK_URL"); v != "" {
		cfg.TTS.SparkURL = v
	}
	if v := os.Getenv("NARRATE_DGX_SSH_HOST"); v != "" {
		cfg.TTS.SSHHost = v
	}
	if v := os.Getenv("NARRATE_CACHE_DIR"); v != "" {
		cfg.CacheDir = v
	}
	if cfg.CacheDir == "" {
		userCache, err := os.UserCacheDir()
		if err == nil {
			cfg.CacheDir = filepath.Join(userCache, "narrate")
		} else {
			cfg.CacheDir = filepath.Join(os.TempDir(), "narrate-cache")
		}
	}
	return cfg, src, nil
}

func configPath() string {
	if v := os.Getenv("NARRATE_CONFIG"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "narrate-config.json"
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(home, "narrate", "config.json")
	}
	return filepath.Join(home, ".config", "narrate", "config.json")
}

// Validate checks configuration without billable calls. needAI indicates the
// mode requires the rewrite provider (script/audio without --verbatim).
// needAudio indicates the mode will actually synthesize/play audio; it is
// false for --script-only, which must work without a TTS backend at all.
func (c *Config) Validate(needAI, needAudio bool) error {
	if needAI {
		if c.AI.Provider != "openrouter" && c.AI.Provider != "openai" {
			return fmt.Errorf("ai.provider must be openrouter or openai")
		}
		if strings.TrimSpace(c.AI.APIKey) == "" {
			return fmt.Errorf("no AI credential configured: set OPENROUTER_API_KEY (OpenRouter), OPENAI_API_KEY (OpenAI), or NARRATE_AI_API_KEY; or add {\"ai\":{\"api_key\":\"...\"}} to %s (config help, and voice listing never make billable calls)", configPath())
		}
		if strings.TrimSpace(c.AI.Model) == "" {
			return fmt.Errorf("no AI model configured: set NARRATE_AI_MODEL or add {\"ai\":{\"model\":\"...\"}} to %s (a default model is intentionally not assumed)", configPath())
		}
	}
	if !needAudio {
		return nil
	}
	if c.ParagraphGapMs < 0 || c.ParagraphGapMs > 60000 {
		return fmt.Errorf("paragraph_gap_ms must be between 0 and 60000")
	}
	switch c.TTS.Backend {
	case "native":
		if runtime.GOOS != "darwin" {
			return fmt.Errorf("tts backend \"native\" requires macOS /usr/bin/say; this platform (%s) is not supported for audio, use --script-only", runtime.GOOS)
		}
	case "pocket", "":
		if c.TTS.SparkURL == "" || c.TTS.SSHHost == "" {
			return fmt.Errorf("Pocket TTS needs NARRATE_SPARK_URL and NARRATE_DGX_SSH_HOST (or tts.spark_url and tts.ssh_host in config); use --tts=native for macOS say")
		}
		if c.Rate != 0 {
			return fmt.Errorf("--rate is only supported with --tts=native")
		}

	default:
		return fmt.Errorf("unknown tts backend %q (want pocket or native)", c.TTS.Backend)
	}
	return nil
}
