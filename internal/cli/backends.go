package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/narrate-it/narrate/internal/pocket"
)

type backendUnavailable struct{ reason string }

func (e *backendUnavailable) Error() string { return e.reason }

type backendRunner func(context.Context, string, options) error

func runBackendChain(ctx context.Context, stdout, stderr io.Writer, source, script string, o options) error {
	return tryBackendChain(ctx, stderr, o, func(ctx context.Context, name string, attempt options) error {
		switch name {
		case "pocket":
			return runPocketAudio(ctx, stdout, stderr, source, script, attempt)
		case "openrouter":
			return runPreferredAudio(ctx, stdout, stderr, source, script, attempt)
		case "native":
			if runtime.GOOS != "darwin" {
				return &backendUnavailable{"native speech requires macOS"}
			}
			if _, err := exec.LookPath("/usr/bin/say"); err != nil {
				return &backendUnavailable{"native speech is not installed"}
			}
			return runAudio(ctx, stdout, stderr, source, script, attempt)
		}
		return fmt.Errorf("unsupported backend %q", name)
	})
}

func tryBackendChain(ctx context.Context, stderr io.Writer, o options, run backendRunner) error {
	order, err := o.cfg.BackendOrder()
	if err != nil {
		return err
	}
	// Keep a selected container consistent when moving between backends.
	if o.fileFormat == "" && o.outputFile != "" && filepath.Ext(o.outputFile) == "" && order[0] != "native" {
		o.fileFormat = "MP3"
	}
	var failures []error
	for index, name := range order {
		if err := ctx.Err(); err != nil {
			return err
		}
		attempt := o
		cfg := *o.cfg
		cfg.TTS.Backend = name
		if voice, ok := cfg.TTS.Voices[name]; ok {
			cfg.TTS.Voice = voice
		} else if len(order) > 1 && name != "openrouter" {
			cfg.TTS.Voice = ""
		}
		if o.voiceOverride && index == 0 {
			cfg.TTS.Voice = o.cfg.TTS.Voice
		}
		attempt.cfg = &cfg
		err := run(ctx, name, attempt)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var remote *pocket.UnavailableError
		var provider *speechUnavailable
		var native *backendUnavailable
		if !errors.As(err, &remote) && !errors.As(err, &provider) && !errors.As(err, &native) {
			return err
		}
		failures = append(failures, fmt.Errorf("%s unavailable: %w", name, err))
		if index+1 < len(order) {
			fmt.Fprintf(stderr, "narrate: %s unavailable; trying %s\n", name, order[index+1])
		}
	}
	return fmt.Errorf("no configured speech backend succeeded: %w", errors.Join(failures...))
}
