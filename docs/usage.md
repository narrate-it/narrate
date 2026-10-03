# narrate(1) — conversational document narrator

## SYNOPSIS

    narrate [options] [TEXT ... | -f FILE | stdin]

## DESCRIPTION

Rewrites written text into a faithful conversational monologue using an AI
provider (OpenRouter by default), then speaks it through OpenRouter (default, with macOS native fallback), writes audio
with `-o`, or prints only the script with `--script-only`.

## INPUT

A single positional file path reads the file; missing path-like arguments are
rejected before AI calls. Other positional args are joined with spaces. `-f FILE` reads a UTF-8 file; `-f -`
or no input reads stdin to EOF (terminal: end with Ctrl-D). `-f` plus
positional text is rejected. Use `--` before text beginning with a dash.

## OUTPUT

`-o FILE` saves audio without playing. Suffix decides container
(`.mp3`/`.aiff`/`.wav`); no suffix means MP3 for OpenRouter/Pocket, AIFF for explicit native; `--file-format` overrides the suffix.
`-o` files are refused if they exist unless `--force`. Without `-o`, audio
plays the complete recording via `afplay` after generation (and Whisper verification for Pocket).
Use `--stream` to opt into paragraph playback while Pocket is still generating.
Streaming requires `--tts=pocket` and cannot be combined with `-o` or `--script-only`.

`--script-only` prints only the rewritten script to stdout (clean for
pipes), or saves it with `--script-out`. Works on all platforms, no TTS
needed. `--verbatim` skips rewriting; OpenRouter speech still requires a credential.

## VOICES / FORMATS

`--tts=native -v '?'` lists native voices. Pocket defaults to `michael`. `--file-format '?'` lists formats (AIFF, WAVE, MP3; ffmpeg required for MP3).

## OPTIONS

See `narrate --help` for the full list. `--minutes N` is reserved and
rejected until duration targeting is implemented. `--resume`
reuses cache for identical input+settings; with no prior cache it simply
does full work. `--artifacts-dir DIR` writes `manifest.json`,
`transcript.md`, `spoken-script.txt`, plus Pocket transcription and comparison evidence.

Set `NARRATE_TTS_MODEL` to an OpenRouter speech model and `OPENROUTER_API_KEY`
to its credential. Set `NARRATE_TTS_VOICE` to a voice supported by the selected model. Missing configuration
or provider failure uses the system default macOS voice, with a stderr notice.
A failed OpenRouter rewrite in audio mode reads the source natively. Explicit
`--tts=native` and `--tts=pocket` select those backends. See README for details.

## SPEECH SPEED

`--speed=0.85` slows OpenRouter/native speech while preserving pitch. Valid
values are 0.5–2, default 1. Configure `tts.speed` or `NARRATE_TTS_SPEED` for
persistent behavior; the flag takes precedence. Adjustment requires ffmpeg,
works for native fallback, and happens before chapter timing and output
publication. Paragraph gaps retain their configured duration. Pocket requires
`--speed=1`; `--rate` remains the native synthesis words-per-minute control.

## EXIT STATUS

0 success · 1 runtime failure · 2 usage/config error · 130 Ctrl-C

## NOT SUPPORTED (rejected explicitly)

`-n` AUNetSend, `-a` device, `-i` word highlighting, codec/data-format
controls. Unlike `say`, terminal stdin collects the whole document to EOF.
