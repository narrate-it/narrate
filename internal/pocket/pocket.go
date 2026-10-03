// Package pocket embeds the remote Pocket TTS and Whisper rendering pipeline.
package pocket

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

//go:embed driver.py render.py transcribe.py playback.py
var scripts embed.FS

type Config struct {
	Playback              bool    `json:"playback"`
	SSHUID                int     `json:"ssh_uid"`
	Directory             string  `json:"directory"`
	SparkURL              string  `json:"spark_url"`
	SSHHost               string  `json:"ssh_host"`
	Voice                 string  `json:"voice"`
	GapMS                 int     `json:"gap_ms"`
	Resume                bool    `json:"resume"`
	Format                string  `json:"format"`
	Output                string  `json:"output"`
	Device                string  `json:"device"`
	HostPath              string  `json:"host_path"`
	PythonPath            string  `json:"python_path"`
	OutputSubdir          string  `json:"output_subdir"`
	CacheSubdir           string  `json:"cache_subdir"`
	Image                 string  `json:"image"`
	Speed                 float64 `json:"speed"`
	ConnectTimeoutSeconds int     `json:"connect_timeout_seconds"`
}

type UnavailableError struct{ Err error }

func (e *UnavailableError) Error() string { return e.Err.Error() }
func (e *UnavailableError) Unwrap() error { return e.Err }

func preflight(ctx context.Context, endpoint, device string, timeoutSeconds int) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return &UnavailableError{fmt.Errorf("invalid Pocket remote endpoint")}
	}
	timeout := time.Duration(timeoutSeconds) * time.Second
	preCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(preCtx, http.MethodGet, strings.TrimRight(endpoint, "/")+"/api/v1/resources", nil)
	if err != nil {
		return &UnavailableError{err}
	}
	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return &UnavailableError{fmt.Errorf("Pocket remote preflight unavailable")}
	}
	defer resp.Body.Close()
	var inv struct {
		Available struct {
			CPU    int `json:"cpuMillis"`
			Memory int `json:"memoryMB"`
			GPU    int `json:"gpuCount"`
		} `json:"available"`
	}
	if resp.StatusCode != http.StatusOK {
		return &UnavailableError{fmt.Errorf("Pocket preflight HTTP %d", resp.StatusCode)}
	}
	if err := json.NewDecoder(resp.Body).Decode(&inv); err != nil {
		return &UnavailableError{fmt.Errorf("Pocket preflight response: %w", err)}
	}
	if inv.Available.CPU < 1000 || inv.Available.Memory < 6144 || (device == "cuda" && inv.Available.GPU < 1) {
		return &UnavailableError{fmt.Errorf("Pocket remote capacity unavailable for %s", device)}
	}
	return nil
}

func defaults(cfg *Config) {
	if cfg.Device == "" {
		cfg.Device = "cuda"
	}
	if cfg.OutputSubdir == "" {
		cfg.OutputSubdir = "output"
	}
	if cfg.CacheSubdir == "" {
		cfg.CacheSubdir = "cache"
	}
	if cfg.Image == "" {
		cfg.Image = "nvcr.io/nvidia/pytorch:26.02-py3"
	}
	if cfg.Speed == 0 {
		cfg.Speed = 1
	}
	if cfg.ConnectTimeoutSeconds == 0 {
		cfg.ConnectTimeoutSeconds = 3
	}
}

type Result struct {
	Duration  float64   `json:"duration"`
	Starts    []float64 `json:"starts"`
	Directory string
}

func Render(ctx context.Context, cfg Config, script, cacheDir string, stderr io.Writer) (Result, error) {
	defaults(&cfg)
	if cfg.SparkURL == "" || cfg.SSHHost == "" || cfg.HostPath == "" || cfg.PythonPath == "" || cfg.SSHUID <= 0 {
		return Result{}, &UnavailableError{fmt.Errorf("Pocket remote settings are incomplete")}
	}
	if !safeSSHHost(cfg.SSHHost) {
		return Result{}, fmt.Errorf("Pocket SSH host contains unsupported characters")
	}
	if cfg.Device != "cuda" && cfg.Device != "cpu" {
		return Result{}, fmt.Errorf("unsupported Pocket device %q", cfg.Device)
	}
	if cfg.Speed <= 0 {
		return Result{}, fmt.Errorf("Pocket speed must be greater than zero")
	}
	if cfg.ConnectTimeoutSeconds < 1 || cfg.ConnectTimeoutSeconds > 60 {
		return Result{}, fmt.Errorf("Pocket connect timeout must be from 1 to 60 seconds")
	}
	if !safeSubdir(cfg.OutputSubdir) || !safeSubdir(cfg.CacheSubdir) {
		return Result{}, fmt.Errorf("Pocket output and cache subdirectories must be safe relative paths")
	}
	if !safeHostPath(cfg.HostPath) {
		return Result{}, fmt.Errorf("Pocket host path must be an absolute path using letters, digits, slash, dot, underscore, or dash")
	}
	if err := preflight(ctx, cfg.SparkURL, cfg.Device, cfg.ConnectTimeoutSeconds); err != nil {
		return Result{}, err
	}
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
	for _, value := range []string{script, cfg.Voice, cfg.SparkURL, cfg.SSHHost, cfg.HostPath, cfg.PythonPath, cfg.Device, cfg.OutputSubdir, cfg.CacheSubdir, cfg.Image, fmt.Sprint(cfg.GapMS), fmt.Sprint(cfg.Speed), fmt.Sprint(cfg.ConnectTimeoutSeconds)} {
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
	var diagnostic strings.Builder
	cmd.Stderr = io.MultiWriter(stderr, &diagnostic)
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 40 * time.Second
	if err := cmd.Run(); err != nil {
		wrapped := fmt.Errorf("Pocket generation failed (evidence: %s): %w", cfg.Directory, err)
		if ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
		if strings.Contains(diagnostic.String(), "UNAVAILABLE:") {
			return Result{}, &UnavailableError{fmt.Errorf("remote Pocket rendering unavailable")}
		}
		return Result{}, wrapped
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

func safeSubdir(value string) bool {
	if value == "" || filepath.IsAbs(value) {
		return false
	}
	for _, part := range strings.Split(filepath.ToSlash(value), "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return safePathChars(filepath.ToSlash(value))
}

func safeHostPath(value string) bool {
	return filepath.IsAbs(value) && filepath.Clean(value) == value && safePathChars(value)
}
func safeSSHHost(value string) bool {
	if value == "" || strings.HasPrefix(value, "-") {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("@._:-", r)) {
			return false
		}
	}
	return true
}
func safePathChars(value string) bool {
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("/._-", r)) {
			return false
		}
	}
	return true
}
