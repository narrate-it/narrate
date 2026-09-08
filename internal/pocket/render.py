"""One-off, resumable Pocket TTS rendering on the existing DGX cache."""
import hashlib
import json
import os
import time
from pathlib import Path

import argparse
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--script", type=Path, required=True)
parser.add_argument("--output", type=Path, required=True)
parser.add_argument("--voice", default="michael")
parser.add_argument("--gap-ms", type=int, default=650)
parser.add_argument("--ssh-uid", type=int, required=True)
args = parser.parse_args()
os.environ.setdefault("HF_HOME", "/host/vibevoice-cache/hf")
os.environ["OMP_NUM_THREADS"] = "1"
os.environ["MKL_NUM_THREADS"] = "1"

import numpy as np
import scipy.io.wavfile as wavfile
import torch
from pocket_tts import TTSModel

torch.set_num_threads(1)
torch.set_num_interop_threads(1)
text = args.script.read_text()
paragraphs = [part.strip() for part in text.strip().split("\n\n") if part.strip()]
digest = hashlib.sha256(text.encode()).hexdigest()
if not paragraphs:
    raise ValueError("The script is empty")
out = args.output
out.mkdir(parents=True, exist_ok=True)
os.chown(out, args.ssh_uid, -1)
(out / "coach-script.txt").write_text(text)
started = time.monotonic()
print(f"START paragraphs={len(paragraphs)} output={out}", flush=True)
model = TTSModel.load_model()
model.to("cpu")
voice = args.voice
state = model.get_state_for_audio_prompt(voice)
sample_rate = model.sample_rate
manifest = {
    "script_sha256": digest,
    "voice": voice,
    "engine": "Pocket TTS",
    "device": "cpu",
    "sample_rate": sample_rate,
    "paragraph_count": len(paragraphs),
    "gap_seconds": (args.gap_ms / 1000),
    "paragraphs": [],
}
clips = []
peak_ceiling = 10 ** (-1.5 / 20)
for index, paragraph in enumerate(paragraphs):
    path = out / f"paragraph-{index + 1:03d}.wav"
    if path.exists():
        rate, data = wavfile.read(path)
        if rate != sample_rate or data.dtype != np.float32 or data.ndim != 1 or not data.size or not np.isfinite(data).all() or not np.any(data):
            raise RuntimeError(f"Invalid cached clip: {path}")
    else:
        data = model.generate_audio(state, paragraph).detach().cpu().numpy()
        data = np.asarray(data, dtype=np.float32).reshape(-1)
        if not data.size or not np.isfinite(data).all():
            raise RuntimeError(f"Invalid audio in paragraph {index + 1}")
        peak = float(np.max(np.abs(data)))
        if peak == 0:
            raise RuntimeError(f"Silent paragraph {index + 1}")
        if peak > peak_ceiling:
            data *= peak_ceiling / peak
        temporary = path.with_suffix(".partial.wav")
        wavfile.write(temporary, sample_rate, data)
        temporary.replace(path)
    seconds = len(data) / sample_rate
    words = len(paragraph.split())
    if seconds < 0.1 * words or seconds > 2 * words + 10:
        raise RuntimeError(f"Suspicious duration paragraph {index + 1}: {seconds}")
    manifest["paragraphs"].append({
        "index": index + 1, "words": words, "duration_seconds": seconds,
        "sha256": hashlib.sha256(paragraph.encode()).hexdigest(),
    })
    clips.append(data)
    if index < len(paragraphs) - 1:
        clips.append(np.zeros(round(sample_rate * (args.gap_ms / 1000)), dtype=np.float32))
    os.chown(path, args.ssh_uid, -1)
    (out / "progress.json").write_text(json.dumps(manifest, indent=2))
    print(f"PARAGRAPH {index + 1}/{len(paragraphs)} audio={seconds:.1f}s elapsed={time.monotonic() - started:.1f}s", flush=True)

audio = np.concatenate(clips)
wavfile.write(out / "coach-raw.wav", sample_rate, audio)
manifest["duration_seconds"] = len(audio) / sample_rate
manifest["render_seconds"] = time.monotonic() - started
manifest["complete"] = True
(out / "manifest.json").write_text(json.dumps(manifest, indent=2))
print("COMPLETE " + json.dumps({"output": str(out), "duration_seconds": manifest["duration_seconds"], "render_seconds": manifest["render_seconds"]}), flush=True)
