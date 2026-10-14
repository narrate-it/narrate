# Persistent local Anna synthesis

`server.py` loads one Pocket model and Anna state at startup, then streams bounded
PCM16LE NDJSON over authenticated HTTP. Existing Narrate CLI and paragraph/file
workflows are unchanged. The additive Go consumer is `speech/local`.

Prepare the pinned requirements in an ARM64 CPU image, including model weights
and Anna asset. Run the service with local, immutable model config and Anna file:

```sh
python server.py --host 0.0.0.0 --port 8092 \
  --token-file /run/narrate/token \
  --model-config /models/config.yaml --model-config-sha256 "$CONFIG_SHA256" \
  --anna-asset /models/anna.safetensors --anna-sha256 "$ANNA_SHA256"
```

The config must reference prepared local model files (not download URLs). Deny
external egress after preparation. SHA-256 flags are mandatory: never substitute
an arbitrary preset when the approved asset is missing. Use a scoped random
32+ character service credential in a read-only secret file; do not put it in
arguments, manifests, logs, or repository files. Keep the endpoint private to the
trusted LAN/tailnet, or terminate TLS before any untrusted network. Health requires
the same bearer credential. Access/content logs are disabled.

Use the owner's Spark workload API, a unique pod name, a pinned ARM64 image,
2 CPU / 4 GiB limits initially, a read-only models mount and narrowly scoped
read-only token mount. Qualify resource limits with STT concurrently before
release. Do not launch SSH processes or displace another workload. The service
requires no GPU. A prepared container can execute this module directly; no
per-utterance process, file, or transfer occurs. Admission rejects concurrent
requests with 503 instead of accumulating a queue; callers surface overload and
never blindly replay text. A response's phrases must be submitted sequentially.

## Wire contract v1

`POST /v1/synthesize` with `Authorization: Bearer …`, JSON fields `generation`
(nonzero uint64), `text_sequence` (nonzero uint64), `voice` (`anna`), and `text`
(nonempty, at most 16 KiB UTF-8). The response is `application/x-ndjson`:

1. `format` record: `format={encoding:pcm_s16le,sample_rate:24000,channels:1}`.
2. `frame` records: `sequence` starts at 1, `offset` starts at zero in samples,
   `samples` is at most 4800 (200 ms), `pcm` is base64 little-endian signed PCM16.
3. `done`: final frame `sequence` and total sample `offset`, followed by HTTP EOF.

Every record repeats `generation` and `text_sequence`. Missing terminal success,
extra records, wrong generation/order/format, oversized frames and transport loss
are failures. The Go API maps phrase-local offsets to response-global offsets
and emits a zero-sample `PhraseEnd` only after validated terminal completion.
That boundary is synthesized alignment, never evidence of audible playback.

Context cancellation closes HTTP and sets Pocket's stop event when the service
observes disconnect. The service caps generation at 40 seconds and buffering at
two PCM records, then gives the worker two seconds to exit. A noncooperative
worker makes health unavailable and cannot overlap another model invocation.
The supervisor must restart an unhealthy process. Client phrase timeout defaults
to 45 seconds. A stalled callback must itself honor its caller's context; the
client deliberately calls it synchronously to preserve backpressure.

## Evidence and remaining runtime qualification

The owner-approved `zati-coming-soon-anna-20261009.receipt.json` identifies Pocket
Anna; the maintained `zati-video` skill specifies seed 20261009 and native 24 kHz
mono. Those local receipts do **not** contain the original preset asset hash.
A tracked read-only Spark probe subsequently reconciled the cached preset; see
`qualification/asset-pins.json`. Anna SHA-256 is
`5ea82f78db006c9fd34e32ddd5aae82674b5b32646097977436458d00af80dfa`.
`stage_assets.py` verifies and stages that preset, the exact English model and
tokenizer into a new directory, replacing download references with `/models`
paths. `spark_manifest.py` defaults to those concrete Anna and staged-config
hashes. Existing files are never overwritten.

```sh
python stage_assets.py --hub /prepared/hub \
  --pocket-package /prepared/site-packages/pocket_tts --output /prepared/anna-bundle
```

The cached production Pocket 3.0.2 package cannot cancel streaming: it lacks the
`stop` argument. The service rejects such a backend during construction instead
of advertising readiness and failing on the first phrase. Use the newer pinned
requirements; the existing model and Anna assets remain explicitly pinned.

Upstream source pinned at `b94db177bc0f77517d9b124843e25605d6f9aed8` exposes
`generate_audio_stream(..., copy_state=True, stop=threading.Event())`, 24 kHz PCM,
and a non-thread-safe model. See
https://github.com/kyutai-labs/pocket-tts/blob/b94db177bc0f77517d9b124843e25605d6f9aed8/pocket_tts/models/tts_model.py.
The service serializes inference and preserves the same voice state across
phrases. Upstream internals may buffer generation before yielding decoded PCM;
API availability does not prove first-frame latency or uninterrupted delivery.

Run `go test -race ./...` and `python -m unittest -v test_server.py` in this
folder (Python tests need only aiohttp). Fixtures cover authentication, stale and
truncated wire streams, ordering, exact phrase boundaries, disconnect cancellation,
overload and timeout. They do not measure actual Pocket inference. Before release,
record ARM64 model/config/asset hashes, warm first-frame p50/p95, chunk intervals,
cancellation latency, continuity/listening acceptance and concurrent STT load.

### Measured bounded Spark check

`qualification/runtime-receipt.json` records a real ARM64 run of this service
using the pinned upstream source, existing Anna/model assets and staged local
config. One CPU, 2 GiB, no GPU; all host mounts read-only. Two phrases produced
24 kHz PCM with explicit terminal records: first-frame times **97 ms and 76 ms**,
maximum inter-frame gaps **34 ms**, and 3.28 / 2.16 seconds of audio. Cancellation
released inference in **11 ms**; a subsequent phrase completed successfully.
These are two samples, not p50/p95 or a conversational latency guarantee.

The test denied non-loopback Python TCP connections after dependency preparation
and used local model references. This establishes that this exercised path did
not need a remote model fetch; it is not the RFC's all-protocol firewall test.
No speaker, human listening, concurrent STT or endurance acceptance is claimed.
The owned probe pod was removed after recording its terminal result.
