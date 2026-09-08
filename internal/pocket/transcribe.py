"""Independently transcribe the complete rendered narration for comparison."""
import json
import math
import time
from pathlib import Path

import argparse
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--directory", type=Path, required=True)
args = parser.parse_args()

import torch
import whisper
import numpy as np
from scipy.io import wavfile
from scipy.signal import resample_poly

torch.set_num_threads(1)
torch.set_num_interop_threads(1)
out = args.directory
assert (out / "manifest.json").exists(), "Render is not complete"
start = time.monotonic()
print("Loading Whisper base.en for complete audio check", flush=True)
model = whisper.load_model("base.en", device="cpu", download_root="/host/vibevoice-cache/whisper")
sample_rate, audio = wavfile.read(out / "coach-raw.wav")
assert audio.dtype == np.float32 and audio.ndim == 1
factor = math.gcd(sample_rate, 16000)
audio = resample_poly(audio, 16000 // factor, sample_rate // factor).astype(np.float32)
result = model.transcribe(audio, language="en", fp16=False, verbose=False)
(out / "heard.txt").write_text(result["text"].strip())
(out / "heard.json").write_text(json.dumps({"text": result["text"], "segments": result["segments"]}))
print(f"TRANSCRIBED words={len(result['text'].split())} elapsed={time.monotonic() - start:.1f}s", flush=True)
