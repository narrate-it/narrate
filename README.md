# narrate

Narrate turns documents into spoken scripts and audio with configurable
AI rewriting, Pocket TTS on a DGX, or local macOS `say`. It also gives CLI
coding agents concise, phase-based progress updates. When you pass `-o FILE`,
Narrate saves the recording without playing it through the speakers.

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

Use `-o FILE` to write an audio file without speaker playback.

`--verbatim` skips AI rewriting and needs no AI key. `--script-only` skips all
audio dependencies. Options go before positional text; use `--` before text
beginning with a dash.

## Install

### Install with a coding agent

Paste this prompt into your coding agent:

```text
Install Narrate using its official instructions at
https://github.com/narrate-it/narrate#install. Detect my operating system and
use the documented Homebrew or apt package when available. Verify the
installation by running `narrate --version`. Do not configure AI credentials
or change Narrate settings. Set up its documented automatic-update method too,
but ask before using sudo, adding package sources, or scheduling background
updates. If this platform has no supported package yet, tell me what is
missing and stop.
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
needed. On macOS, coding updates use local speech by default when no backend
is configured: no API key or DGX required. Pocket TTS and AI document rewriting need the
[configuration below](#configuration).

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
