# narrate(1) — conversational document narrator

## SYNOPSIS

    narrate [options] [TEXT ... | -f FILE | stdin]

## DESCRIPTION

Rewrites text into a faithful spoken script using an AI provider (OpenRouter by
default), then generates audio with the configured speech backend order. Use
`-o` to save audio or `--script-only` to print only the script.

## INPUT

A single positional file path reads the file; missing path-like arguments are
rejected before AI calls. Other positional arguments are joined with spaces.
`-f FILE` reads a UTF-8 file; `-f -` or no input reads stdin to EOF (terminal:
end with Ctrl-D). `-f` plus positional text is rejected. Use `--` before text
beginning with a dash.

## OUTPUT

`-o FILE` saves audio without playing. The suffix selects the container
(`.mp3`, `.aiff`, or `.wav`); `--file-format` overrides it. Existing output
files are refused unless `--force`. Without `-o`, Narrate plays the complete
recording after generation. Use `--stream` for paragraph playback while Pocket
is generating; it requires `--tts=pocket` and cannot be combined with `-o` or
`--script-only`.

`--script-only` prints only the rewritten script to stdout, or saves it with
`--script-out`; it does not require speech dependencies. `--verbatim` skips AI
rewriting. OpenRouter speech still requires credentials.

## SPEECH BACKENDS

`--tts=auto` follows the configured backend order, which defaults to
`pocket,openrouter,native`. Use `--tts=pocket`, `--tts=openrouter`, or
`--tts=native` to select one backend. A comma separated list such as
`--tts=pocket,openrouter,native` supplies an order for that invocation.

An unavailable backend can be skipped. Local execution errors, output failures,
and cancellation stop the run without trying another backend. See
[backend selection](backend-priority.md) and the README for setup and
configuration details.

## VOICES / FORMATS

`--tts=native -v '?'` lists native voices. Backend voice defaults are configured
in `tts.voices`. `--file-format '?'` lists supported formats.

## OPTIONS

See `narrate --help` for the full list. `--minutes N` is reserved and rejected
until duration targeting is implemented. `--resume` reuses cache for identical
input and settings; with no prior cache it performs the full work.
`--artifacts-dir DIR` saves the script, manifest, and available verification
evidence.

## SPEECH SPEED

`--speed=0.85` slows supported speech while preserving pitch. Valid values are
`0.5`–`2`, with default `1`. Configure `tts.speed` or `NARRATE_TTS_SPEED` for
persistent behavior; the flag takes precedence. Adjustment requires `ffmpeg`
and applies to OpenRouter and native synthesis. Pocket requires speed `1`;
`--rate` remains the native synthesis words-per-minute control.

## EXIT STATUS

0 success · 1 runtime failure · 2 usage/config error · 130 Ctrl-C
