package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"example.com/narrate/internal/audio"
)

// ChapterInfo records a paragraph's start time in the final audio.
type ChapterInfo struct {
	Paragraph     int     `json:"paragraph"`
	ScriptExcerpt string  `json:"script_excerpt"`
	StartSec      float64 `json:"start_seconds"`
}

// runAudio synthesizes the script with the native macOS backend, assembles
// the container, and plays or saves it.
func runAudio(ctx context.Context, stdout, stderr io.Writer, source, script string, o options) error {
	// Speech synthesis via /usr/bin/say writes AIFF directly per paragraph.
	paras := splitScriptParagraphs(script)
	if len(paras) == 0 {
		return fmt.Errorf("empty spoken script")
	}
	started := time.Now()

	var spec audio.ClipSpec
	var chapterList []ChapterInfo
	var total float64

	workDir, err := os.MkdirTemp("", "narrate-audio-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workDir)

	outPath, format := resolveOutput(o.outputFile, o.fileFormat)

	// Streaming final file: create output only if -o given; otherwise a temp
	// container that we play then delete.
	finalFile := outPath
	if finalFile == "" {
		ext := ".aiff"
		if format == "WAVE" {
			ext = ".wav"
		}
		finalFile = filepath.Join(workDir, "out"+ext)
	}
	if outPath != "" {
		if _, err := os.Stat(finalFile); err == nil && !o.force {
			return fmt.Errorf("refusing to overwrite existing %s (use --force)", outPath)
		}
	}

	f, err := os.CreateTemp(filepath.Dir(finalFile), ".narrate-audio-*")
	if err != nil {
		return fmt.Errorf("creating output: %w", err)
	}
	defer os.Remove(f.Name())
	var sink io.WriteSeeker = f
	closer := f

	var finalDuration float64
	writeErr := func() error {
		if o.progress {
			fmt.Fprintf(stderr, "narrate: synthesizing %d paragraphs\n", len(paras))
		}
		// Synthesize the first paragraph to learn the native output format.
		firstClip := filepath.Join(workDir, "clip000.aiff")
		if err := synthParagraph(ctx, stderr, paras[0], firstClip, o); err != nil {
			return fmt.Errorf("synthesis failed: %w", err)
		}
		if o.progress {
			fmt.Fprintf(stderr, "narrate: synthesized paragraph 1/%d (%s elapsed)\n", len(paras), elapsedSince(started))
		}
		spec0, _, err := probeAIFF(firstClip)
		if err != nil {
			return fmt.Errorf("clip validation failed: %w", err)
		}
		spec = spec0

		var w audio.StreamWriter
		switch format {
		case "WAVE":
			w, err = audio.NewWavStreamWriter(sink, spec)
		default: // AIFF
			w, err = audio.NewAiffStreamWriter(sink, spec)
		}
		if err != nil {
			return err
		}

		gapMs := o.cfg.ParagraphGapMs
		for i, p := range paras {
			clip := firstClip
			if i > 0 {
				clip = filepath.Join(workDir, fmt.Sprintf("clip%03d.aiff", i))
				if err := synthParagraph(ctx, stderr, p, clip, o); err != nil {
					return fmt.Errorf("synthesis failed: %w", err)
				}
				if o.progress {
					fmt.Fprintf(stderr, "narrate: synthesized paragraph %d/%d (%s elapsed)\n", i+1, len(paras), elapsedSince(started))
				}
				clipSpec, _, err := probeAIFF(clip)
				if err != nil {
					return fmt.Errorf("clip validation failed: %w", err)
				}
				if clipSpec != spec {
					return fmt.Errorf("clip %d has a different audio format than the first clip", i)
				}
			}
			if err := w.CopyClip(clip); err != nil {
				return fmt.Errorf("assembly failed: %w", err)
			}
			chapterList = append(chapterList, ChapterInfo{
				Paragraph:     i,
				ScriptExcerpt: excerpt(p, 80),
				StartSec:      total,
			})
			dur := clipDuration(clip, spec)
			total += dur
			if i < len(paras)-1 {
				if err := w.WriteSilence(gapMs); err != nil {
					return err
				}
				total += float64(spec.SampleRate*gapMs/1000) / float64(spec.SampleRate)
			}
		}
		finalDuration = total
		if o.progress {
			fmt.Fprintf(stderr, "narrate: encoding %s audio (%s elapsed)\n", strings.ToLower(format), elapsedSince(started))
		}
		return w.Finalize()
	}()

	if closer != nil {
		if err := closer.Close(); writeErr == nil {
			writeErr = err
		}
	}
	if writeErr != nil {
		return writeErr
	}
	if err := publishTemp(f.Name(), finalFile, o.force); err != nil {
		return err
	}
	if o.scriptOut != "" {
		if err := writeDest(o.scriptOut, []byte(script), o.force); err != nil {
			return err
		}
	}

	if o.progress {
		fmt.Fprintf(stderr, "narrate: audio %.1fs written (%s elapsed)\n", finalDuration, elapsedSince(started))
	}
	if err := maybeWriteArtifacts(o, source, script, finalDuration, chapterList); err != nil {
		return err
	}

	if outPath != "" {
		// -o: no playback.
		return nil
	}

	// Playback; delete temp container afterwards.
	if err := playFile(ctx, finalFile); err != nil {
		return fmt.Errorf("playback failed: %w", err)
	}
	return nil
}

// synthParagraph invokes /usr/bin/say with argument arrays (never shell).
// Text goes through stdin; output is explicitly big-endian PCM AIFF.
func synthParagraph(ctx context.Context, stderr io.Writer, text, outAiff string, o options) error {
	args := []string{}
	if o.cfg.TTS.Voice != "" {
		args = append(args, "-v", o.cfg.TTS.Voice)
	}
	if o.rate > 0 {
		args = append(args, "-r", fmt.Sprint(o.rate))
	}
	args = append(args, "-o", outAiff, "--file-format=AIFF", "--data-format=BEI16")
	cmd := exec.CommandContext(ctx, "/usr/bin/say", args...)
	cmd.Stdin = strings.NewReader(text)
	// Send text through stdin to avoid argument limits and option interpretation.
	cmd.Stderr = stderr
	return cmd.Run()
}

// playFile plays a completed audio file with afplay (argument array).
func playFile(ctx context.Context, path string) error {
	cmd := exec.CommandContext(ctx, "/usr/bin/afplay", path)
	return cmd.Run()
}

// probeAIFF parses the COMM chunk of an AIFF clip and validates payload.
func probeAIFF(path string) (audio.ClipSpec, uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return audio.ClipSpec{}, 0, err
	}
	defer f.Close()
	return audio.ValidateAIFF(bufio.NewReader(f))
}

func clipDuration(path string, spec audio.ClipSpec) float64 {
	_, frames, err := probeAIFF(path)
	if err != nil || spec.SampleRate <= 0 {
		return 0
	}
	return float64(frames) / float64(spec.SampleRate)
}

// resolveOutput maps -o plus --file-format to a path and canonical format.
// Explicit format wins over suffix; no extension defaults to AIFF (like say).
func resolveOutput(out, format string) (string, string) {
	if out == "" {
		return "", "AIFF"
	}
	if format == "WAVE" || format == "WAV" {
		return out, "WAVE"
	}
	if format == "AIFF" || format == "AIFF-C" {
		return out, "AIFF"
	}
	switch strings.ToLower(filepath.Ext(out)) {
	case ".wav":
		return out, "WAVE"
	case ".aiff", ".aif":
		return out, "AIFF"
	default:
		return out, "AIFF"
	}
}

func splitScriptParagraphs(script string) []string {
	var out []string
	for _, p := range strings.Split(script, "\n\n") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func excerpt(s string, n int) string {
	rs := []rune(strings.TrimSpace(s))
	if len(rs) <= n {
		return string(rs)
	}
	return string(rs[:n]) + "…"
}

// runCapture runs a command and returns its combined stdout.
func runCapture(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	return string(out), err
}
