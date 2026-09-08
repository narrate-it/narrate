package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"example.com/narrate/internal/ai"
	"example.com/narrate/internal/cache"
	"example.com/narrate/internal/config"
	"example.com/narrate/internal/input"
	"example.com/narrate/internal/narration"
)

// errCanceled signals Ctrl-C.
var errCanceled = errors.New("canceled")

type options struct {
	cfg          *config.Config
	style        string
	scriptOnly   bool
	scriptOut    string
	outputFile   string
	fileFormat   string
	verbatim     bool
	minutes      int
	artifactsDir string
	resume       bool
	progress     bool
	stream       bool
	force        bool
	rate         int
}

// execute runs the selected workflow with Ctrl-C cancellation.
func execute(stdin io.Reader, stdout, stderr io.Writer, src input.Source, o options) (err error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer func() {
		if ctx.Err() != nil {
			err = errCanceled
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)
	go func() {
		select {
		case <-sig:
			cancel()
		case <-ctx.Done():
		}
	}()

	// Refuse invalid destinations before contacting the AI provider.
	for _, path := range []string{o.outputFile, o.scriptOut} {
		if path == "" {
			continue
		}
		if _, statErr := os.Lstat(path); statErr == nil && !o.force {
			return fmt.Errorf("refusing to overwrite existing %s (use --force)", path)
		} else if statErr != nil && !os.IsNotExist(statErr) {
			return statErr
		}
		if info, statErr := os.Stat(filepath.Dir(path)); statErr != nil {
			return statErr
		} else if !info.IsDir() {
			return fmt.Errorf("output parent is not a directory: %s", path)
		}
	}
	if o.outputFile != "" && o.scriptOut != "" {
		audioPath, _ := filepath.Abs(o.outputFile)
		scriptPath, _ := filepath.Abs(o.scriptOut)
		if audioPath == scriptPath {
			return fmt.Errorf("audio and script output must use different paths")
		}
	}

	// Rewrite stage.
	var script string
	if o.verbatim {
		script = narration.JoinScript([]string{src.Text})
	} else {
		rewriteStarted := time.Now()
		store, err := cache.Open(o.cfg.CacheDir)
		if err != nil {
			return fmt.Errorf("opening cache: %w", err)
		}
		client, err := ai.New(ai.Config{
			Provider: o.cfg.AI.Provider,
			APIKey:   o.cfg.AI.APIKey,
			BaseURL:  o.cfg.AI.BaseURL,
			Model:    o.cfg.AI.Model,
		})
		if err != nil {
			return err
		}

		// Cache identity: source hash + prompt digest + provider/model/options.
		promptDigest, err := narration.EffectiveDigest(o.style)
		if err != nil {
			return err
		}
		srcSum := sha256Hex(src.Text)
		rewriteKey := cache.Key("rewrite-v1", srcSum, promptDigest, o.cfg.AI.Provider, o.cfg.AI.BaseURL, o.cfg.AI.Model, o.style)

		chunker := narration.NewChunker(12000, 400)
		chunks := chunker.Chunk(src.Text)

		if o.resume {
			var cached struct {
				Parts []string `json:"parts"`
			}
			if ok, _ := store.GetJSON(rewriteKey, &cached); ok && len(cached.Parts) == len(chunks) {
				if o.progress {
					fmt.Fprintf(stderr, "narrate: rewrite reused from cache (%d chunks, %s elapsed)\n", len(cached.Parts), elapsedSince(rewriteStarted))
				}
				script = narration.JoinScript(cached.Parts)
			}
		}

		if script == "" {
			if o.progress {
				fmt.Fprintf(stderr, "narrate: rewriting %d chunks\n", len(chunks))
			}
			rw := &narration.Rewriter{Client: client, Style: o.style}
			parts, err := rw.RewriteAllWithCount(ctx, chunks, func(done, total int) {
				if o.progress {
					fmt.Fprintf(stderr, "narrate: rewriting chunk %d/%d (%s elapsed)\n", done, total, elapsedSince(rewriteStarted))
				}
			})
			if err != nil {
				return fmt.Errorf("rewrite failed: %w", err)
			}
			if err := store.PutJSON(rewriteKey, struct {
				Parts []string `json:"parts"`
			}{parts}); err != nil {
				return fmt.Errorf("caching rewrite: %w", err)
			}
			script = narration.JoinScript(parts)
		}
	}

	// Script outputs.
	if o.scriptOnly {
		if o.scriptOut != "" {
			if err := writeDest(o.scriptOut, []byte(script), o.force); err != nil {
				return err
			}
			if o.progress {
				fmt.Fprintf(stderr, "narrate: script saved to %s\n", o.scriptOut)
			}
		} else {
			fmt.Fprintln(stdout, script)
		}
		return maybeWriteArtifacts(o, src.Text, script, 0, nil)
	}

	// Audio workflow.
	if o.cfg.TTS.Backend == "pocket" || o.cfg.TTS.Backend == "" {
		return runPocketAudio(ctx, stdout, stderr, src.Text, script, o)
	}
	return runAudio(ctx, stdout, stderr, src.Text, script, o)
}

// writeDest refuses existing files without --force, then writes atomically.
func writeDest(path string, data []byte, force bool) error {
	if _, err := os.Stat(path); err == nil && !force {
		return fmt.Errorf("refusing to overwrite existing %s (use --force)", path)
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".narrate-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return publishTemp(tmpName, path, force)
}

func maybeWriteArtifacts(o options, source, script string, durationSec float64, chapters []ChapterInfo) error {
	if o.artifactsDir == "" {
		return nil
	}
	if err := os.MkdirAll(o.artifactsDir, 0o700); err != nil {
		return err
	}
	type manifestT struct {
		Version    string        `json:"version"`
		TTSBackend string        `json:"tts_backend,omitempty"`
		Voice      string        `json:"voice,omitempty"`
		Prompt     string        `json:"prompt_version"`
		Style      string        `json:"style"`
		Verbatim   bool          `json:"verbatim"`
		Duration   float64       `json:"duration_seconds"`
		Chapters   []ChapterInfo `json:"chapters,omitempty"`
		ScriptSHA  string        `json:"script_sha256"`
		SourceSHA  string        `json:"source_sha256"`
	}
	m := manifestT{
		Version:   Version,
		Prompt:    narration.PromptVersion,
		Style:     o.style,
		Verbatim:  o.verbatim,
		Duration:  durationSec,
		Chapters:  chapters,
		ScriptSHA: sha256Hex(script),
		SourceSHA: sha256Hex(source),
	}
	if !o.scriptOnly {
		m.TTSBackend = o.cfg.TTS.Backend
		m.Voice = o.cfg.TTS.Voice
		if m.TTSBackend == "pocket" && m.Voice == "" {
			m.Voice = "michael"
		}
	}
	mb, _ := json.MarshalIndent(m, "", "  ")
	if err := writeDest(filepath.Join(o.artifactsDir, "manifest.json"), mb, true); err != nil {
		return err
	}
	if err := writeDest(filepath.Join(o.artifactsDir, "spoken-script.txt"), []byte(script), true); err != nil {
		return err
	}
	// Readable transcript: source and script side by side.
	var tr strings.Builder
	tr.WriteString("# narrate transcript\n\n## Source\n\n")
	tr.WriteString(source)
	tr.WriteString("\n\n## Spoken script\n\n")
	tr.WriteString(script)
	tr.WriteString("\n")
	return writeDest(filepath.Join(o.artifactsDir, "transcript.md"), []byte(tr.String()), true)
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// publishTemp atomically publishes completed output; Link enforces no-clobber.
func publishTemp(temp, dest string, force bool) error {
	if force {
		return os.Rename(temp, dest)
	}
	if err := os.Link(temp, dest); err != nil {
		return fmt.Errorf("publishing %s (use --force to replace existing files): %w", dest, err)
	}
	return nil
}

func elapsedSince(start time.Time) string {
	return time.Since(start).Truncate(time.Second).String()
}
