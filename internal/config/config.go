// Package config resolves configuration with precedence:
// flags > environment > user config > defaults.
package config

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
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
	Backends              []string          `json:"backends,omitempty"`
	Voices                map[string]string `json:"voices,omitempty"`
	Device                string            `json:"device,omitempty"`
	RemoteHostPath        string            `json:"remote_host_path,omitempty"`
	RemotePython          string            `json:"remote_python,omitempty"`
	RemoteOutputSubdir    string            `json:"remote_output_subdir,omitempty"`
	RemoteCacheSubdir     string            `json:"remote_cache_subdir,omitempty"`
	RemoteImage           string            `json:"remote_image,omitempty"`
	ConnectTimeoutSeconds int               `json:"connect_timeout_seconds,omitempty"`

	Speed    float64 `json:"speed,omitempty"`   // pitch-preserving tempo multiplier for native/OpenRouter
	Backend  string  `json:"backend,omitempty"` // native | openrouter | pocket
	Model    string  `json:"model,omitempty"`
	APIKey   string  `json:"api_key,omitempty"`
	BaseURL  string  `json:"base_url,omitempty"`
	Voice    string  `json:"voice,omitempty"`
	SSHUID   int     `json:"ssh_uid,omitempty"`
	SparkURL string  `json:"spark_url,omitempty"`
	SSHHost  string  `json:"ssh_host,omitempty"`
}

// Source describes where configuration came from (for diagnostics).
type Source struct{ ConfigFile string }

// Load reads the user config file (no secrets required to exist) and applies
// environment overrides. Flags are applied by the CLI on top of this.
func Load() (*Config, Source, error) {
	cfg := &Config{
		TTS:            TTSConfig{Backend: "auto", Backends: []string{"pocket", "openrouter", "native"}, Device: "cuda", SSHUID: 1000, Speed: 1, RemoteOutputSubdir: "output", RemoteCacheSubdir: "cache", RemoteImage: "nvcr.io/nvidia/pytorch:26.02-py3", ConnectTimeoutSeconds: 3},
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
	if v := os.Getenv("NARRATE_TTS_SPEED"); v != "" {
		speed, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, src, fmt.Errorf("NARRATE_TTS_SPEED must be a number between 0.5 and 2")
		}
		cfg.TTS.Speed = speed
	}
	if v := os.Getenv("NARRATE_TTS_MODEL"); v != "" {
		cfg.TTS.Model = v
	}
	if v := os.Getenv("OPENROUTER_API_KEY"); v != "" {
		cfg.TTS.APIKey = v
	}
	if v := os.Getenv("NARRATE_TTS_API_KEY"); v != "" {
		cfg.TTS.APIKey = v
	}
	if cfg.TTS.APIKey == "" && cfg.AI.Provider == "openrouter" {
		cfg.TTS.APIKey = cfg.AI.APIKey
	}
	if v := os.Getenv("NARRATE_TTS_BASE_URL"); v != "" {
		cfg.TTS.BaseURL = v
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
	for key, dest := range map[string]*string{
		"NARRATE_REMOTE_SSH_HOST":      &cfg.TTS.SSHHost,
		"NARRATE_TTS_DEVICE":           &cfg.TTS.Device,
		"NARRATE_REMOTE_HOST_PATH":     &cfg.TTS.RemoteHostPath,
		"NARRATE_REMOTE_PYTHON":        &cfg.TTS.RemotePython,
		"NARRATE_REMOTE_OUTPUT_SUBDIR": &cfg.TTS.RemoteOutputSubdir,
		"NARRATE_REMOTE_CACHE_SUBDIR":  &cfg.TTS.RemoteCacheSubdir,
		"NARRATE_REMOTE_IMAGE":         &cfg.TTS.RemoteImage,
	} {
		if v := os.Getenv(key); v != "" {
			*dest = v
		}
	}
	if v := os.Getenv("NARRATE_TTS_BACKENDS"); v != "" {
		cfg.TTS.Backends = strings.Split(v, ",")
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
	if math.IsNaN(c.TTS.Speed) || math.IsInf(c.TTS.Speed, 0) || c.TTS.Speed < 0.5 || c.TTS.Speed > 2 {
		return fmt.Errorf("tts.speed / --speed must be between 0.5 and 2")
	}
	if c.ParagraphGapMs < 0 || c.ParagraphGapMs > 60000 {
		return fmt.Errorf("paragraph_gap_ms must be between 0 and 60000")
	}
	order, err := c.BackendOrder()
	if err != nil {
		return err
	}
	if c.Rate != 0 && (len(order) != 1 || order[0] != "native") {
		return fmt.Errorf("--rate requires a single native backend; use --speed for a backend chain")
	}
	if c.TTS.Device != "cuda" && c.TTS.Device != "cpu" {
		return fmt.Errorf("tts.device must be cuda or cpu")
	}
	if c.TTS.ConnectTimeoutSeconds < 1 || c.TTS.ConnectTimeoutSeconds > 60 {
		return fmt.Errorf("tts.connect_timeout_seconds must be between 1 and 60")
	}
	if len(order) == 1 && order[0] == "native" && runtime.GOOS != "darwin" {
		return fmt.Errorf("native speech requires macOS; use another backend or --script-only")
	}
	return nil
}

// BackendOrder resolves an explicit backend or the configurable ordered defaults.
func (c *Config) BackendOrder() ([]string, error) {
	selected := c.TTS.Backends
	if c.TTS.Backend != "" && c.TTS.Backend != "auto" {
		selected = strings.Split(c.TTS.Backend, ",")
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("tts.backends must contain at least one backend")
	}
	order := make([]string, 0, len(selected))
	seen := make(map[string]bool)
	for _, value := range selected {
		name := strings.TrimSpace(value)
		if name != "pocket" && name != "openrouter" && name != "native" {
			return nil, fmt.Errorf("unknown speech backend %q (want pocket, openrouter or native)", name)
		}
		if seen[name] {
			return nil, fmt.Errorf("duplicate speech backend %q", name)
		}
		seen[name] = true
		order = append(order, name)
	}
	return order, nil
}
