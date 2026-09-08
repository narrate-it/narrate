# narrate

Turn a document into a conversational script and audio with **OpenRouter**,
then speak it with **Pocket TTS** on the DGX or the macOS `say` voice. The
Pocket path independently verifies the recording with **Whisper base.en** and
normalizes it with **FFmpeg**. Narrate also emits phase-based progress so CLI
agents and humans can see what it is doing when it matters.

This is the stack used by the `coaching-audio` skill.

## Companion repos

- [narrate-cursor](https://github.com/narrate-it/narrate-cursor) for Cursor
  project commands
- [narrate-claude-code](https://github.com/narrate-it/narrate-claude-code) for
  Claude Code MCP prompts
- [narrate-codex](https://github.com/narrate-it/narrate-codex) for the Codex
  plugin

We build narrate, a small set of tools that help CLI agents speak only when it
matters.

## Use

```sh
narrate --progress report.md
narrate -f report.md -o report.mp3 --artifacts-dir report-audio
narrate --verbatim 'Read these exact words.'
cat report.md | narrate --script-only > spoken.txt
```

A single file path is read as a document, including quoted paths with spaces.
Missing path-like arguments fail before AI calls. `-f FILE` remains supported.
To narrate a pathname literally, pipe it through stdin.

`--verbatim` skips AI rewriting and needs no AI key. `--script-only` skips all
audio dependencies. Options go before positional text; use `--` before text
beginning with a dash.

## Install

Requires Go 1.22+. Build and install on your user PATH:

```sh
go build -o narrate .
mkdir -p ~/.local/bin
install -m 755 narrate ~/.local/bin/narrate
```

Ensure `~/.local/bin` is on PATH. Pocket output requires local `python3`,
`scp`, and `ffmpeg`. Playback requires macOS `afplay`; other platforms can
save files with `-o`.

For agent narration, use the companion repos:

- Cursor: `./install.sh /path/to/project` from `narrate-cursor`
- Claude Code: `./install.sh` from `narrate-claude-code`
- Codex: `./install.sh` from `narrate-codex`

## Configuration

Optional config: `~/.config/narrate/config.json`; see
[the example](examples/config.example.json). `NARRATE_CONFIG` selects another
file. Precedence: flags, environment, config, defaults.

```sh
export OPENROUTER_API_KEY='your-key'
export NARRATE_AI_MODEL='openrouter/auto'
export NARRATE_SPARK_URL='http://YOUR-DGX:8080'
export NARRATE_DGX_SSH_HOST='USER@YOUR-DGX'
```

Pocket submits a tracked Spark pod using the existing DGX environment:
`nvcr.io/nvidia/pytorch:26.02-py3`, `/host/vibevoice-cache/venv/bin/python`,
Pocket TTS's default model, and Whisper `base.en`. Each job needs 1 free CPU
and 6 GiB RAM; it does not request a GPU or stop other workloads. Output is
retrieved with SCP, then the task's pod is deleted. Remote files remain under
`/var/lib/zerfoo/vibevoice-out/narrate-*` for recovery. `tts.ssh_uid` defaults
to 1000 and must match the SSH user's UID so it can retrieve private files.
No separate HTTP TTS service or paid speech provider is needed.

`openrouter/auto` lets OpenRouter choose a rewrite model; set an explicit model
ID to control that choice. Narrate uses
[OpenRouter's Chat Completions API](https://openrouter.ai/docs/quickstart).
Direct OpenAI remains available with `NARRATE_AI_PROVIDER=openai`,
`OPENAI_API_KEY`, and `NARRATE_AI_MODEL`.

Other environment options: `NARRATE_AI_BASE_URL`, `NARRATE_AI_API_KEY`
(overrides the provider-specific key), `NARRATE_TTS_BACKEND`,
`NARRATE_TTS_VOICE`, and `NARRATE_CACHE_DIR`.

## Audio and verification

Without `-o`, Narrate finishes synthesis, Whisper verification and encoding
before playing the complete recording. Streaming is **opt-in**:

```sh
narrate --stream -f report.md
```

`--stream` starts speaker playback with the first completed paragraph while
Pocket generates later paragraphs. Early playback uses peak-limited raw clips
before Whisper verification and final loudness normalization. Ctrl-C stops
playback and cancels the Spark job. AI rewriting still completes before synthesis.
Use `-o` to save a complete recording without playback; `--stream` cannot be
combined with `-o`, `--script-only`, or `--tts=native`.

Pocket defaults to MP3: mono 44.1 kHz, 128 kbps, paragraph chapters and
loudness normalization targeting -18 LUFS / -1.5 dBTP. Natural runtime is
preserved, with 650 ms paragraph gaps. `.wav` and `.aiff` output also work.
FFmpeg fully decodes the encoded output to check it.

Whisper transcribes the full raw narration. `verification.json` records a
normalized word alignment and differing spans. This is an automated comparison,
not word error rate or a human listening review. Large differences produce a
warning; names and pronunciation can cause ASR differences. Inspect the
script and `heard.txt` when fidelity matters.

`--artifacts-dir DIR` saves the source/script transcript, manifest with chapter
offsets, Whisper transcription and comparison. Evidence is also retained in
the local Pocket cache. `--resume` reuses a completed render and transcription
for matching text, voice, gap, endpoint and renderer settings. Interrupted
remote jobs are retained as files, but are not automatically resumed. Clear
the corresponding cache after changing DGX model versions.

Use `--script-out FILE` to save the spoken script with audio. Existing
script/audio files are refused unless `--force` is passed; generation failures
preserve previous audio. Artifact files are replaced on each run.

## macOS say alternative

```sh
narrate --tts=native --verbatim 'Speak with the Mac voice.'
narrate --tts=native -v '?'
narrate --tts=native -v Samantha -r 180 -f report.md -o report.aiff
```

Native speech supports AIFF/WAV and speaking rate. Pocket uses `-v michael`
by default; `-r` is rejected for Pocket. Duration targeting (`--minutes`),
network/device/highlighting switches and automatic publication are outside
this CLI's current workflow. Narrate does not upload to SoundCloud.

## Privacy

Source text goes to the configured AI provider unless `--verbatim` is used.
Pocket receives the script on your configured DGX. Native speech runs locally.
Scripts, audio and transcripts are kept in private local cache directories and
remote job directories. Credentials are excluded from cache keys and manifests.

## Verification

```sh
go test ./...
go vet ./...
python3 -m unittest discover -s internal/pocket -p '*_test.py'
```

Tests cover provider requests, native PCM conversion, output preservation,
Pocket defaults, Spark resource gating/cleanup, transfer and FFmpeg commands.
A live Pocket + Whisper + normalized MP3 run was verified on September 7, 2026.
