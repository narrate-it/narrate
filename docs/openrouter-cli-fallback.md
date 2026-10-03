# OpenRouter CLI speech and native fallback

Decision: 2026-10-02.

The CLI defaults to OpenRouter speech, using the existing bounded speech client.
Pocket remains an explicit `--tts=pocket` backend; `--tts=native` selects macOS
speech directly. Speech model and voice settings are separate from the rewrite
model. No speech model or voice is assumed globally. The configuration example
uses `mistralai/voxtral-mini-tts-2603` and its published `en_paul_neutral` voice.

Missing speech credentials/model/voice and provider request or audio failures
fall back to the macOS system voice with a stderr notice. Failed OpenRouter
rewriting in audio mode reads the original source with native speech. Text-only
rewriting retains error behavior. Cancellation, output errors, and missing local
decoder tools remain failures. Provider voices are cleared on native fallback.

Every speech request is bounded to 4,000 runes, a 60-second timeout, and the
existing response-size limit. Long paragraphs are split without changing their
text. No speech request is retried. Unknown billing after a timeout remains
possible. All audio is generated and decoded before publication or playback.
The actual backend is recorded in the artifact manifest. MP3 output is supported
for both provider and fallback speech through ffmpeg; existing output is preserved
on generation failure. OpenRouter speech is not cached by `--resume`.

Speech accepts the official OpenRouter endpoint or a parsed loopback HTTP test
endpoint. Userinfo, non-loopback hosts, query strings and fragments are rejected
before credentials are attached. OpenAI credentials are excluded from OpenRouter
speech credential resolution.

Verification:

- The missing-configuration regression test failed before the CLI integration.
- `go test ./...` and `go vet ./...` passed after integration and review.
- CLI tests cover valid provider audio, refusal, malformed audio, native fallback,
  MP3 decoding, output preservation, cancellation, Unicode chunking, rewrite
  failure, and actual manifest backend. Configuration tests cover precedence and
  credential isolation; speech tests reject credential-leaking endpoint forms.
- A live OpenRouter Voxtral request generated a decodable MP3 and recorded
  `openrouter` as the actual backend. A refused request successfully produced a
  native fallback MP3. These are generation checks, not human listening review.

The implementation is installed locally for verification. It has not been
published as a CLI release; a future package or launcher update may replace the
local build until the change is included in a release.
