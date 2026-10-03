# narrate

Narrate turns documents into spoken scripts and audio with configurable AI
rewriting and speech. By default, it tries a remote Pocket TTS backend, then
OpenRouter, then the native macOS voice. The order is configurable, and
explicit backend selection is available when you want one renderer. Narrate also
gives CLI coding agents concise, phase-based progress updates. Pass `-o FILE`
to save a recording without playing it through the speakers.

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

Use `-o FILE` to write an audio file without speaker playback.

`--verbatim` skips AI rewriting; OpenRouter speech still needs a key. `--script-only` skips all
audio dependencies. Options go before positional text; use `--` before text
beginning with a dash.

## Install

### Install the CLI and plugins with a coding agent

Copy this prompt into a coding agent to install Narrate and the official
integration(s) for the agents you use:

```text
Install or update the Narrate CLI and its official coding-agent plugin(s).

1. Detect my operating system, architecture, and which of Codex, Cursor, and Claude Code I use in this environment. If you cannot tell which integrations I want, ask me before installing plugins.
2. Follow the CLI's official install guide: https://github.com/narrate-it/narrate#install. Prefer the supported native package manager. If an apt feed is not enabled, use the matching .deb from https://github.com/narrate-it/narrate/releases/latest and explain that unattended apt updates require the signed feed.
3. Install the matching official integration(s), following each repository's README:
   - Codex: https://github.com/narrate-it/narrate-codex
   - Cursor: https://github.com/narrate-it/narrate-cursor
   - Claude Code: https://github.com/narrate-it/narrate-claude-code
4. Verify `narrate --version` and confirm each selected plugin or command is available.
5. Set up the documented update method for the chosen CLI install: native package-manager updates for Homebrew/apt, or Narrate's self-updating launcher for a standalone install. Never layer the self-updater onto a package-managed install.

Use only these official Narrate repositories. Preserve existing settings and files; ask before overwriting files, using sudo, adding package sources, or scheduling background updates. Do not configure AI credentials or run a narration. Report what was installed, how it updates, and anything that still needs setup.
```
### Claude Code (marketplace)

```sh
claude plugin marketplace add narrate-it/narrate
claude plugin install narrate@narrate-it
```

Restart Claude Code, then ask it to speak:

```text
/narrate:speak Summarize what we just changed
/narrate:speak /absolute/path/to/report.md
```

The plugin checks for a newer CLI release every six hours, verifies its
SHA-256 checksum, and caches it for later calls. Supports macOS and Linux on
ARM64 and x86-64; requires `curl` and `shasum` or `sha256sum`.
No Go installation, manual build, `/narrate:install`, or PATH changes are
needed. Speech uses the configured backend order. OpenRouter requires credentials;
remote Pocket requires an endpoint and SSH access; native speech depends on the
platform. See [configuration](#configuration).

Adding the marketplace registers the catalog; the second command installs
the plugin. This follows Claude Code's
[marketplace installation flow](https://code.claude.com/docs/en/plugin-marketplaces).

### Homebrew

After the first tagged release, install the formula directly from this repo:

```sh
brew install narrate-it/narrate/narrate
```

Each CLI release updates the formula with the release's verified binary
checksums. Package-managed installs stay under Homebrew. To schedule daily
upgrades, install [Homebrew Autoupdate](https://github.com/DomT4/homebrew-autoupdate)
and start it for Narrate:

```sh
brew tap domt4/autoupdate
brew autoupdate start 1d --upgrade --only=narrate-it/narrate/narrate
```

### Debian and Ubuntu

Once the signed apt feed is enabled by the repository owner, add its source and
install Narrate:

```sh
arch=$(dpkg --print-architecture)
case "$arch" in amd64|arm64) ;; *) echo "Unsupported architecture: $arch" >&2; exit 1 ;; esac
sudo install -d -m 755 /etc/apt/keyrings
curl --fail --location https://narrate-it.github.io/narrate/narrate-archive-keyring.asc \
  | sudo gpg --dearmor --yes --output /etc/apt/keyrings/narrate.gpg
printf 'deb [arch=%s signed-by=/etc/apt/keyrings/narrate.gpg] https://narrate-it.github.io/narrate stable main\n' "$arch" \
  | sudo tee /etc/apt/sources.list.d/narrate.list >/dev/null
sudo apt update
sudo apt install narrate unattended-upgrades
printf 'Unattended-Upgrade::Origins-Pattern { "origin=Narrate,label=Narrate"; };\n' \
  | sudo tee /etc/apt/apt.conf.d/52narrate-unattended >/dev/null
sudo dpkg-reconfigure -plow unattended-upgrades
```

The apt feed is signed and publishes the latest Debian packages on each CLI
release. Its first publication requires the repository owner to enable GitHub
Pages, set `PUBLISH_APT_REPO=true`, and add the apt signing key secrets, as
described in [package release setup](docs/package-release-setup.md).

### Standalone CLI

Requires Go 1.22+. Build and install the CLI plus its self-updating launcher:

```sh
sh scripts/install-local.sh
```

The launcher checks GitHub Releases every six hours, downloads only HTTPS
release assets and verifies their SHA-256 checksum before using an update.
It keeps using the installed binary when the network is unavailable. Ensure
`~/.local/bin` is on PATH. Pocket output requires local `python3`, `scp`, and
`ffmpeg`; playback requires macOS `afplay`; other platforms can save files
with `-o`.

For other agent integrations, use the companion repos:

- Cursor: `./install.sh /path/to/project` from `narrate-cursor`
- Claude Code MCP prompts (alternative): `./install.sh` from `narrate-claude-code`
- Codex: `./install.sh` from `narrate-codex`

## Configuration

Optional config: `~/.config/narrate/config.json`; see
[the example](examples/config.example.json). `NARRATE_CONFIG` selects another
file. Precedence: flags, environment, config, defaults.

```sh
export OPENROUTER_API_KEY='your-key'
export NARRATE_AI_MODEL='openrouter/auto'
export NARRATE_TTS_BACKEND='auto'
export NARRATE_SPARK_URL='http://spark.example:8080'
export NARRATE_REMOTE_SSH_HOST='user@gpu.example'
```

Speech defaults to `tts.backend: "auto"` and tries `tts.backends` in order;
the default order is `pocket`, `openrouter`, `native`. Configure a different
order or omit backends you do not use. `--tts=auto` follows that order;
`--tts=pocket`, `--tts=openrouter`, or `--tts=native` selects one backend.
`--tts=pocket,openrouter,native` provides an ordered list for that invocation.
A backend that is unavailable can be skipped. A local execution error or
cancellation stops the run instead of silently switching backends.

Set `tts.voices` by backend. Pocket defaults to `michael`, and native speech
uses the macOS system default. OpenRouter requires a configured voice supported
by the selected speech model. Set `tts.model` or
`NARRATE_TTS_MODEL` to choose the OpenRouter speech model; it is separate from
`ai.model` / `NARRATE_AI_MODEL` for rewriting. `OPENROUTER_API_KEY` can supply
both OpenRouter requests; `tts.api_key` / `NARRATE_TTS_API_KEY` can provide a
separate speech credential. Speech uses
[OpenRouter's audio speech endpoint](https://openrouter.ai/docs/guides/overview/multimodal/tts)
and requires `ffmpeg` to decode returned audio.

Remote Pocket speech uses the Spark-compatible endpoint in `tts.spark_url` or
`NARRATE_SPARK_URL`, plus the SSH host in `tts.ssh_host` or
`NARRATE_REMOTE_SSH_HOST`.
Set `tts.remote_host_path` to the directory mounted on the remote host and
`tts.remote_python` to its Python interpreter; for example `/srv/narrate` and
`/host/venv/bin/python`. The remote output and cache directories default to
`output` and `cache`. The default container image is
`nvcr.io/nvidia/pytorch:26.02-py3`. Connection setup times out after 3 seconds
by default. `tts.device` selects the actual Torch device (`cuda` by default;
set it to `cpu` when needed). All remote values are configurable; see the
example config for the full set.

A failed OpenRouter rewrite in audio mode reads the source text for speech;
`--script-only` reports rewrite errors. Cancellation and local output failures
are not retried through another backend. The manifest records the backend that
generated the recording.

Set `tts.speed` or `NARRATE_TTS_SPEED` for a persistent speech speed; the CLI
flag takes precedence. The default is `1`, with a supported range of `0.5` to
`2`. The adjustment applies to OpenRouter and native speech through `ffmpeg`
and preserves pitch. Pocket also applies pacing after rendering; streaming requires speed `1`.

```sh
narrate --speed=0.85 --verbatim 'Hello from Narrate.'
```

`openrouter/auto` lets OpenRouter choose a rewrite model; set an explicit model
ID to control that choice. Narrate uses
[OpenRouter's Chat Completions API](https://openrouter.ai/docs/quickstart).
Direct OpenAI remains available with `NARRATE_AI_PROVIDER=openai`,
`OPENAI_API_KEY`, and `NARRATE_AI_MODEL`.

Other environment options: `NARRATE_AI_BASE_URL`, `NARRATE_AI_API_KEY`
(overrides the provider-specific key), `NARRATE_TTS_VOICE`,
`NARRATE_TTS_MODEL`, `NARRATE_TTS_API_KEY`, `NARRATE_TTS_SPEED`,
`NARRATE_CACHE_DIR`. Configure remote connection and runtime fields in the
JSON config file.

## Audio and verification

Without `-o`, Narrate finishes synthesis and encoding (plus Whisper verification
for Pocket)
before playing the complete recording. Streaming is **opt-in**:

```sh
narrate --tts=pocket --stream -f report.md
```

`--stream` starts speaker playback with the first completed paragraph while
Pocket generates later paragraphs. Early playback uses peak-limited raw clips
before Whisper verification and final loudness normalization. Ctrl-C stops
playback and cancels the Spark job. AI rewriting still completes before synthesis.
Use `-o` to save a complete recording without playback; `--stream` cannot be
combined with `-o` or `--script-only`, and requires `--tts=pocket`.

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
the corresponding cache after changing remote model versions.

Use `--script-out FILE` to save the spoken script with audio. Existing
script/audio files are refused unless `--force` is passed; generation failures
preserve previous audio. Artifact files are replaced on each run.

## Native speech

```sh
narrate --tts=native --verbatim 'Speak with the platform voice.'
narrate --tts=native -v '?'
narrate --tts=native -v Samantha -r 180 -f report.md -o report.aiff
```

Native speech supports AIFF/WAV and speaking rate; MP3 output requires `ffmpeg`. Pocket uses `-v michael`
by default; `-r` is rejected for Pocket and OpenRouter. Duration targeting (`--minutes`),
network/device/highlighting switches and automatic publication are outside
this CLI's current workflow. Narrate does not upload to SoundCloud.

## Privacy

Source text goes to the configured AI provider unless `--verbatim` is used.
OpenRouter speech receives the spoken script. Remote Pocket receives the script
on the configured host. Native speech runs locally.
Scripts, audio and transcripts are kept in private local cache directories and
remote job directories. Credentials are excluded from cache keys and manifests.

## Verification

```sh
go test ./...
go vet ./...
python3 -m unittest discover -s internal/pocket -p '*_test.py'
```

The test suite covers provider requests, native audio conversion, output
preservation, remote job management, file transfer, and audio decoding.
