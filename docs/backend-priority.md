# Speech backend selection

Narrate can try multiple speech backends in a configured order. The default
configuration is `auto` with the order `pocket`, `openrouter`, `native`.
Change `tts.backends` to reorder or omit backends for your environment.

Use `--tts=auto` to follow the configured order, `--tts=pocket`,
`--tts=openrouter`, or `--tts=native` to select one backend, or pass a comma
separated list such as `--tts=pocket,openrouter,native` to set an order for one
run.

An unavailable backend can be skipped so the next configured backend can be
tried. A local execution error, output failure, or cancellation stops the run;
these conditions do not trigger another backend. The artifact manifest records
the backend that generated the audio.

## Backend setup

- **Pocket** uses a Spark-compatible endpoint and SSH access to a configured
  remote host. Set the endpoint, host directory, Python interpreter, and Torch
  device in the configuration. The default device is `cuda`; `cpu` can be
  selected explicitly.
- **OpenRouter** requires an API key, a speech model, and a voice supported by
  that model. It also requires `ffmpeg` to decode provider audio.
- **Native** uses macOS speech and its default voice unless a
  voice is configured.

Voice settings are per backend in `tts.voices`. Pocket defaults to `michael`;
native speech defaults to the macOS system voice. OpenRouter requires a
configured voice supported by the selected speech model. See
[the example configuration](../examples/config.example.json) for remote
connection settings and backend order.

OpenRouter speech uses a bounded request and is not retried by the client.
Provider timeouts can have uncertain billing. Do not treat backend fallback as
a retry guarantee for requests that may already have reached a provider.

## Verification

Go tests cover configured order, per-backend settings, cancellation, explicit
selection, text-only independence and local decoder failure. Python transport
tests cover GPU resource requests, remote capacity gating, transfer cancellation
and cleanup. A live CUDA render generated a decodable recording, with the model
and generated speech tensor verified on CUDA. A separate unavailable-remote run
selected OpenRouter and generated audio. These checks establish automated
generation and transport behavior; they are not a human listening review.
