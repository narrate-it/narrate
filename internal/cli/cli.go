// Package cli implements the narrate command-line contract.
package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"example.com/narrate/internal/config"
	"example.com/narrate/internal/input"
)

// Version is the released tool version.
const Version = "1.0.0"

// Exit codes.
const (
	ExitOK       = 0
	ExitRuntime  = 1
	ExitUsage    = 2
	ExitCanceled = 130
)

const helpText = `narrate — conversational document narrator

Usage:
  narrate [options] FILE
  narrate [options] TEXT...
  narrate [options] -f FILE
  cat doc.md | narrate [options]

Options:
  -f, --input-file=FILE   Read input from FILE ("-" or no text = stdin)
  -o, --output-file=FILE  Write audio to FILE instead of playing. Extension or
                          --file-format chooses the container; no extension
                          defaults to MP3 for Pocket, AIFF for native.
  -v, --voice=VOICE       Voice for the selected TTS backend. -v '?' lists
                          native voices or shows the Pocket default.
  -r, --rate=RATE         Words per minute (positive); passed to native speech.
                          Short and long --flag=value forms are both accepted.
  --file-format=FMT       AIFF|WAVE|MP3 (or "?" to list available formats).
  --tts=BACKEND           pocket (default, DGX) | native (macOS say).
  --script-only           Emit only the rewritten conversational script (text
                          mode; no audio, no TTS, works on all platforms).
  --script-out=FILE       Save the final spoken script to FILE.
  --verbatim              Skip the AI rewrite; no AI credential needed.
  --style=STYLE           conversational (default) | coach
  --minutes=N             Reserved; duration targeting is not yet supported.
  --artifacts-dir=DIR     Save transcript, chapters, and manifest.
  --resume                Reuse validated cached work for identical settings.
  --progress              Show progress on stderr.
  --stream                Play Pocket paragraphs as they arrive (opt-in).
  --force                 Replace existing output/script files.
  --help                  Show this help.
  --version               Print version.

Exit codes: 0 success, 1 runtime failure, 2 usage/config error, 130 Ctrl-C.

Your source text is sent to the configured AI provider for rewriting; the
rewritten script is sent to the selected TTS backend. Keys are read from the
environment or your config file and never printed.

Not supported in v1: -n (AUNetSend), -a (device selection), -i (word
highlighting), and other codec/data-format controls. They are rejected
explicitly.
`

// Run executes the CLI; args excludes the program name.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := newFlagSet(stderr)
	var (
		inputFile     = fs.String("f", "", "input file")
		inputFileLong = fs.String("input-file", "", "input file (long)")
		outputFile    = fs.String("o", "", "output audio file")
		outputLong    = fs.String("output-file", "", "output audio file (long)")
		voice         = fs.String("v", "", "voice")
		voiceLong     = fs.String("voice", "", "voice (long)")
		rate          = fs.Int("r", 0, "words per minute")
		rateLong      = fs.Int("rate", 0, "words per minute (long)")
		scriptOnly    = fs.Bool("script-only", false, "")
		scriptOut     = fs.String("script-out", "", "")
		verbatim      = fs.Bool("verbatim", false, "")
		style         = fs.String("style", "conversational", "")
		minutes       = fs.Int("minutes", 0, "")
		tts           = fs.String("tts", "", "")
		fileFormat    = fs.String("file-format", "", "")
		artifacts     = fs.String("artifacts-dir", "", "")
		resume        = fs.Bool("resume", false, "")
		progress      = fs.Bool("progress", false, "")
		stream        = fs.Bool("stream", false, "")
		force         = fs.Bool("force", false, "")
		showHelp      = fs.Bool("help", false, "")
		showVersion   = fs.Bool("version", false, "")
	)
	if err := fs.Parse(args); err != nil {
		usageErr(stderr, err)
		return ExitUsage
	}
	rest := fs.Args() // "--" handled by flag package; anything after is text (may start with -).

	// Merge duplicated long/short forms.
	if *inputFileLong != "" {
		*inputFile = *inputFileLong
	}
	if *outputLong != "" {
		*outputFile = *outputLong
	}
	if *voiceLong != "" {
		*voice = *voiceLong
	}
	if *rateLong != 0 {
		*rate = *rateLong
	}

	if *showHelp {
		fmt.Fprint(stdout, helpText)
		return ExitOK
	}
	if *showVersion {
		fmt.Fprintln(stdout, "narrate version", Version)
		return ExitOK
	}

	// Discovery modes: no input read, no billable calls.
	if *voice == "?" || *voiceLong == "?" {
		return listVoices(stdout, stderr, *tts)
	}
	if *fileFormat == "?" {
		fmt.Fprintln(stdout, "Available formats: AIFF, WAVE, MP3 (Pocket)")
		return ExitOK
	}
	if *fileFormat != "" {
		switch strings.ToUpper(*fileFormat) {
		case "AIFF", "AIFF-C", "WAVE", "WAV", "MP3":
			// accepted; canonical forms are AIFF and WAVE
		default:
			usageErr(stderr, fmt.Errorf("unsupported --file-format %q (available: AIFF, WAVE, MP3)", *fileFormat))
			return ExitUsage
		}
	}
	if *minutes != 0 {
		usageErr(stderr, fmt.Errorf("--minutes is not supported yet; omit it to narrate the complete document"))
		return ExitUsage
	}
	if *rate < 0 {
		usageErr(stderr, fmt.Errorf("--rate must be a positive words-per-minute value"))
		return ExitUsage
	}
	if *style != "conversational" && *style != "coach" {
		usageErr(stderr, fmt.Errorf("--style must be conversational or coach, got %q", *style))
		return ExitUsage
	}
	if *tts != "" && *tts != "native" && *tts != "pocket" {
		usageErr(stderr, fmt.Errorf("--tts must be pocket or native"))
		return ExitUsage
	}

	if *scriptOnly {
		if *outputFile != "" {
			usageErr(stderr, fmt.Errorf("--script-only cannot be combined with -o/--output-file; use --script-out to save the script"))
			return ExitUsage
		}
		if *verbatim && *minutes > 0 {
			usageErr(stderr, fmt.Errorf("--minutes cannot be combined with --verbatim"))
			return ExitUsage
		}
	}

	// Resolve input before any platform/backend checks, so a usage error like
	// ambiguous input is reported as exit 2 rather than shadowed by an
	// unrelated runtime check.
	stdinIsTerm := stdinIsTerminal(stdin)
	src, err := input.Resolve(*inputFile, rest, stdin, stdinIsTerm)
	if err != nil {
		usageErr(stderr, err)
		return ExitUsage
	}

	cfg, _, err := config.Load()
	if err != nil {
		fmt.Fprintln(stderr, "narrate:", err)
		return ExitUsage
	}
	if *tts != "" {
		cfg.TTS.Backend = *tts
	}
	if *voice != "" {
		cfg.TTS.Voice = *voice
	}
	if *rate > 0 {
		cfg.Rate = *rate
	}
	if *stream && (*scriptOnly || *outputFile != "" || cfg.TTS.Backend == "native") {
		usageErr(stderr, fmt.Errorf("--stream requires Pocket speaker playback; omit --script-only, -o, and --tts=native"))
		return ExitUsage
	}
	needAI := !*verbatim
	needAudio := !*scriptOnly
	if needAudio && *outputFile == "" && runtime.GOOS != "darwin" {
		usageErr(stderr, fmt.Errorf("playback requires macOS; use -o to save audio"))
		return ExitUsage
	}
	if needAudio && cfg.TTS.Backend == "native" && (*fileFormat == "MP3" || strings.EqualFold(filepath.Ext(*outputFile), ".mp3")) {
		usageErr(stderr, fmt.Errorf("MP3 output requires --tts=pocket"))
		return ExitUsage
	}
	if err := cfg.Validate(needAI, needAudio); err != nil {
		fmt.Fprintln(stderr, "narrate:", err)
		return ExitUsage
	}

	opts := options{
		cfg:          cfg,
		style:        *style,
		scriptOnly:   *scriptOnly,
		scriptOut:    *scriptOut,
		outputFile:   *outputFile,
		fileFormat:   strings.ToUpper(*fileFormat),
		verbatim:     *verbatim,
		minutes:      *minutes,
		artifactsDir: *artifacts,
		resume:       *resume,
		progress:     *progress,
		stream:       *stream,
		force:        *force,
		rate:         *rate,
	}

	if err := execute(stdin, stdout, stderr, src, opts); err != nil {
		if err == errCanceled {
			fmt.Fprintln(stderr, "narrate: canceled")
			return ExitCanceled
		}
		fmt.Fprintln(stderr, "narrate:", err)
		return ExitRuntime
	}
	return ExitOK
}

// stdinIsTerminal reports whether stdin is an interactive terminal.
func stdinIsTerminal(r io.Reader) bool {
	if f, ok := r.(*os.File); ok {
		fi, err := f.Stat()
		if err != nil {
			return false
		}
		return fi.Mode()&os.ModeCharDevice != 0
	}
	return false
}

func usageErr(stderr io.Writer, err error) {
	fmt.Fprintln(stderr, "narrate:", err)
	fmt.Fprintln(stderr, "Run `narrate --help` for usage.")
}

func newFlagSet(stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("narrate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, helpText) }
	return fs
}

func listVoices(stdout, stderr io.Writer, backend string) int {
	if backend == "" {
		cfg, _, err := config.Load()
		if err != nil {
			fmt.Fprintln(stderr, err)
			return ExitUsage
		}
		backend = cfg.TTS.Backend
	}
	if backend == "native" {
		return listNativeVoices(stdout, stderr)
	}
	if backend == "pocket" || backend == "" {
		fmt.Fprintln(stdout, "Pocket TTS default voice: michael (use -v NAME for another installed catalog voice)")
		return ExitOK
	}
	fmt.Fprintln(stderr, "narrate: unknown backend", backend)
	return ExitUsage
}

func listNativeVoices(stdout, stderr io.Writer) int {
	if runtime.GOOS != "darwin" {
		fmt.Fprintf(stderr, "narrate: voice listing requires macOS; this platform is %s\n", runtime.GOOS)
		return ExitRuntime
	}
	out, err := runCapture("/usr/bin/say", "-v", "?")
	if err != nil {
		fmt.Fprintln(stderr, "narrate: listing voices:", err)
		return ExitRuntime
	}
	fmt.Fprint(stdout, out)
	return ExitOK
}
