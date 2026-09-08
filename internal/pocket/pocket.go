// Package pocket embeds the coaching-audio Pocket TTS/Whisper Spark pipeline.
package pocket

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

//go:embed driver.py render.py transcribe.py playback.py
var scripts embed.FS

type Config struct {
	Playback  bool   `json:"playback"`
	SSHUID    int    `json:"ssh_uid"`
	Directory string `json:"directory"`
	SparkURL  string `json:"spark_url"`
	SSHHost   string `json:"ssh_host"`
	Voice     string `json:"voice"`
	GapMS     int    `json:"gap_ms"`
	Resume    bool   `json:"resume"`
	Format    string `json:"format"`
	Output    string `json:"output"`
}

type Result struct {
	Duration  float64   `json:"duration"`
	Starts    []float64 `json:"starts"`
	Directory string
}

func Render(ctx context.Context, cfg Config, script, cacheDir string, stderr io.Writer) (Result, error) {
	dependencies := []string{"python3", "ffmpeg", "scp"}
	if cfg.Playback {
		dependencies = append(dependencies, "/usr/bin/afplay")
	}
	for _, name := range dependencies {
		if _, err := exec.LookPath(name); err != nil {
			return Result{}, fmt.Errorf("Pocket TTS requires %s: %w", name, err)
		}
	}
	h := sha256.New()
	for _, value := range []string{script, cfg.Voice, cfg.SparkURL, cfg.SSHHost, fmt.Sprint(cfg.GapMS)} {
		fmt.Fprintf(h, "%s\x00", value)
	}
	for _, name := range []string{"render.py", "transcribe.py", "driver.py", "playback.py"} {
		data, _ := scripts.ReadFile(name)
		h.Write(data)
	}
	cfg.Directory = filepath.Join(cacheDir, "pocket-"+hex.EncodeToString(h.Sum(nil))[:24])
	if err := os.MkdirAll(cfg.Directory, 0700); err != nil {
		return Result{}, err
	}
	lock, err := os.OpenFile(filepath.Join(cfg.Directory, "lock"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return Result{}, fmt.Errorf("Pocket cache busy; if no run is active remove %s/lock: %w", cfg.Directory, err)
	}
	lock.Close()
	defer os.Remove(lock.Name())
	for _, name := range []string{"render.py", "transcribe.py", "driver.py", "playback.py"} {
		data, err := scripts.ReadFile(name)
		if err != nil {
			return Result{}, err
		}
		if err := os.WriteFile(filepath.Join(cfg.Directory, name), data, 0600); err != nil {
			return Result{}, err
		}
	}
	if err := os.WriteFile(filepath.Join(cfg.Directory, "script.txt"), []byte(script), 0600); err != nil {
		return Result{}, err
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return Result{}, err
	}
	cmd := exec.CommandContext(ctx, "python3", filepath.Join(cfg.Directory, "driver.py"))
	cmd.Stdin = strings.NewReader(string(data))
	cmd.Stderr = stderr
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 40 * time.Second
	if err := cmd.Run(); err != nil {
		return Result{}, fmt.Errorf("Pocket generation failed (evidence: %s): %w", cfg.Directory, err)
	}
	data, err = os.ReadFile(filepath.Join(cfg.Directory, "result.json"))
	if err != nil {
		return Result{}, err
	}
	var result Result
	if err := json.Unmarshal(data, &result); err != nil {
		return Result{}, err
	}
	result.Directory = cfg.Directory
	return result, nil
}
