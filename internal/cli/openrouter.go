package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/narrate-it/narrate/speech/openrouter"
)

// speechUnavailable marks provider failures; local output errors do not trigger fallback.
type speechUnavailable struct{ reason string }

func (e *speechUnavailable) Error() string { return e.reason }

func nativeFallbackOptions(o options) options {
	cfg := *o.cfg
	cfg.TTS.Backend = "native"
	cfg.TTS.Voice = "" // Provider voice names are not macOS voice names.
	o.cfg = &cfg
	return o
}

func runPreferredAudio(ctx context.Context, stdout, stderr io.Writer, source, script string, o options) error {
	// Resolve the requested container before fallback so MP3 stays MP3.
	if o.fileFormat == "" && (o.outputFile != "" && filepath.Ext(o.outputFile) == "") {
		o.fileFormat = "MP3"
	}
	return runAudio(ctx, stdout, stderr, source, script, o)
}

func synthOpenRouterParagraph(ctx context.Context, text, outAIFF string, o options) error {
	if strings.TrimSpace(o.cfg.TTS.APIKey) == "" || strings.TrimSpace(o.cfg.TTS.Model) == "" || strings.TrimSpace(o.cfg.TTS.Voice) == "" {
		return &speechUnavailable{"set OPENROUTER_API_KEY, NARRATE_TTS_MODEL and NARRATE_TTS_VOICE (or tts.api_key, tts.model and tts.voice)"}
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return fmt.Errorf("OpenRouter audio requires ffmpeg: %w", err)
	}
	voice := o.cfg.TTS.Voice
	// Bound every call, split long paragraphs without changing their text, and
	// decode each result before any playback or output publication.
	chunks := speechChunks(text)
	var rawFiles []string
	for i, chunk := range chunks {
		result, err := (openrouter.Client{Key: o.cfg.TTS.APIKey, BaseURL: o.cfg.TTS.BaseURL}).Speak(ctx, o.cfg.TTS.Model, voice, chunk)
		if err != nil {
			reason := err.Error()
			var callErr *openrouter.CallError
			if errors.As(err, &callErr) && callErr.Status != 0 {
				reason = fmt.Sprintf("%s (HTTP %d)", reason, callErr.Status)
			}
			return &speechUnavailable{reason}
		}
		check := exec.CommandContext(ctx, ffmpeg, "-nostdin", "-v", "error", "-i", "pipe:0", "-f", "null", "-")
		check.Stdin = bytes.NewReader(result.Audio)
		var diagnostics bytes.Buffer
		check.Stderr = &diagnostics
		if err := check.Run(); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			for _, problem := range []string{"Invalid data found", "Failed to read frame size", "Header missing", "Error while decoding", "Could not find codec parameters"} {
				if strings.Contains(diagnostics.String(), problem) {
					return &speechUnavailable{"provider returned invalid audio"}
				}
			}
			return fmt.Errorf("validating OpenRouter audio: %w", err)
		}
		mp3 := fmt.Sprintf("%s.%d.mp3", outAIFF, i)
		if err := os.WriteFile(mp3, result.Audio, 0o600); err != nil {
			return fmt.Errorf("saving OpenRouter clip: %w", err)
		}
		raw := fmt.Sprintf("%s.%d.pcm", outAIFF, i)
		cmd := exec.CommandContext(ctx, ffmpeg, "-nostdin", "-v", "error", "-i", mp3, "-ar", "22050", "-ac", "1", "-f", "s16be", raw)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("converting OpenRouter audio: %w", err)
		}
		info, err := os.Stat(raw)
		if err != nil {
			return fmt.Errorf("checking decoded clip: %w", err)
		}
		if info.Size() == 0 {
			return &speechUnavailable{"provider returned empty audio"}
		}
		rawFiles = append(rawFiles, raw)
	}
	pcm := outAIFF + ".pcm"
	f, err := os.Create(pcm)
	if err != nil {
		return err
	}
	for _, raw := range rawFiles {
		clip, err := os.Open(raw)
		if err != nil {
			f.Close()
			return err
		}
		_, err = io.Copy(f, clip)
		clip.Close()
		if err != nil {
			f.Close()
			return err
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, ffmpeg, "-nostdin", "-v", "error", "-f", "s16be", "-ar", "22050", "-ac", "1", "-i", pcm, "-c:a", "pcm_s16be", "-f", "aiff", outAIFF)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("assembling decoded speech: %w", err)
	}
	return nil
}

func speechChunks(text string) []string {
	runes := []rune(text)
	var chunks []string
	for len(runes) > 4000 {
		end := 4000
		for i := end; i > 2000; i-- {
			if runes[i-1] == ' ' || runes[i-1] == '\n' {
				end = i
				break
			}
		}
		chunks = append(chunks, string(runes[:end]))
		runes = runes[end:]
	}
	if len(runes) > 0 {
		chunks = append(chunks, string(runes))
	}
	return chunks
}
