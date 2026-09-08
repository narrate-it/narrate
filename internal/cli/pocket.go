package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"example.com/narrate/internal/pocket"
)

func runPocketAudio(ctx context.Context, stdout, stderr io.Writer, source, script string, o options) error {
	format := o.fileFormat
	if format == "" {
		switch strings.ToLower(filepath.Ext(o.outputFile)) {
		case ".wav":
			format = "WAVE"
		case ".aif", ".aiff":
			format = "AIFF"
		case ".mp3", "":
			format = "MP3"
		default:
			return fmt.Errorf("unsupported Pocket output extension; use .mp3, .wav or .aiff")
		}
	}
	if format == "WAV" {
		format = "WAVE"
	}
	if format == "AIFF-C" {
		format = "AIFF"
	}
	dir := os.TempDir()
	if o.outputFile != "" {
		dir = filepath.Dir(o.outputFile)
	}
	temp, err := os.CreateTemp(dir, ".narrate-pocket-*")
	if err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	voice := o.cfg.TTS.Voice
	if voice == "" {
		voice = "michael"
	}
	result, err := pocket.Render(ctx, pocket.Config{Playback: o.stream && o.outputFile == "", SSHUID: o.cfg.TTS.SSHUID, SparkURL: o.cfg.TTS.SparkURL, SSHHost: o.cfg.TTS.SSHHost,
		Voice: voice, GapMS: o.cfg.ParagraphGapMs, Resume: o.resume, Format: format, Output: temp.Name()}, script, o.cfg.CacheDir, stderr)
	if err != nil {
		return err
	}
	paras := splitScriptParagraphs(script)
	if len(result.Starts) != len(paras) {
		return fmt.Errorf("Pocket chapter count does not match script")
	}
	chapters := make([]ChapterInfo, len(paras))
	for i, p := range paras {
		chapters[i] = ChapterInfo{Paragraph: i, ScriptExcerpt: excerpt(p, 80), StartSec: result.Starts[i]}
	}
	if o.scriptOut != "" {
		if err := writeDest(o.scriptOut, []byte(script), o.force); err != nil {
			return err
		}
	}
	if err := maybeWriteArtifacts(o, source, script, result.Duration, chapters); err != nil {
		return err
	}
	if o.artifactsDir != "" {
		for _, name := range []string{"heard.txt", "heard.json", "verification.json"} {
			data, err := os.ReadFile(filepath.Join(result.Directory, name))
			if err != nil {
				return err
			}
			if err := writeDest(filepath.Join(o.artifactsDir, name), data, true); err != nil {
				return err
			}
		}
	}
	if o.progress {
		fmt.Fprintf(stderr, "narrate: Pocket TTS (%s) complete in %.1fs; Whisper comparison: %s\n", voice, result.Duration, filepath.Join(result.Directory, "verification.json"))
	}
	if o.outputFile != "" {
		return publishTemp(temp.Name(), o.outputFile, o.force)
	}
	if o.stream {
		return nil
	} // Streaming already played the paragraphs.
	return playFile(ctx, temp.Name())
}
