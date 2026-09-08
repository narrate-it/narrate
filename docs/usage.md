# narrate(1) — conversational document narrator

## SYNOPSIS

    narrate [options] [TEXT ... | -f FILE | stdin]

## DESCRIPTION

Rewrites written text into a faithful conversational monologue using an AI
provider (OpenRouter by default), then speaks it with Pocket TTS on the DGX (default), writes audio
with `-o`, or prints only the script with `--script-only`.

## INPUT

A single positional file path reads the file; missing path-like arguments are
rejected before AI calls. Other positional args are joined with spaces. `-f FILE` reads a UTF-8 file; `-f -`
or no input reads stdin to EOF (terminal: end with Ctrl-D). `-f` plus
positional text is rejected. Use `--` before text beginning with a dash.

## OUTPUT

`-o FILE` saves audio without playing. Suffix decides container
(`.mp3`/`.aiff`/`.wav`); no suffix means MP3 for Pocket, AIFF for native; `--file-format` overrides the suffix.
`-o` files are refused if they exist unless `--force`. Without `-o`, audio
plays the complete recording via `afplay` after generation and verification.
Use `--stream` to opt into paragraph playback while Pocket is still generating.
Streaming cannot be combined with `-o`, `--script-only`, or `--tts=native`.

`--script-only` prints only the rewritten script to stdout (clean for
pipes), or saves it with `--script-out`. Works on all platforms, no TTS
needed. `--verbatim` skips the rewrite (no AI credential required).

## VOICES / FORMATS

`--tts=native -v '?'` lists native voices. Pocket defaults to `michael`. `--file-format '?'` lists formats (AIFF, WAVE, MP3 for Pocket).

## OPTIONS

See `narrate --help` for the full list. `--minutes N` is reserved and
rejected until duration targeting is implemented. `--resume`
reuses cache for identical input+settings; with no prior cache it simply
does full work. `--artifacts-dir DIR` writes `manifest.json`,
`transcript.md`, `spoken-script.txt`, plus Pocket transcription and comparison evidence.

See README for the Pocket/Spark configuration and `--tts=native` alternative.

## EXIT STATUS

0 success · 1 runtime failure · 2 usage/config error · 130 Ctrl-C

## NOT SUPPORTED (rejected explicitly)

`-n` AUNetSend, `-a` device, `-i` word highlighting, codec/data-format
controls. Unlike `say`, terminal stdin collects the whole document to EOF.
