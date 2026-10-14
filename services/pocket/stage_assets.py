#!/usr/bin/env python3
"""Prepare the inspected Anna bundle from an existing cache without downloads."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil

p=argparse.ArgumentParser()
p.add_argument('--hub',type=Path,required=True)
p.add_argument('--pocket-package',type=Path,required=True)
p.add_argument('--output',type=Path,required=True,help='New directory; never replaces existing files')
a=p.parse_args()
assets={
 'anna.safetensors':(a.hub/'models--kyutai--pocket-tts-without-voice-cloning/snapshots/e81d79e8194ad4c7ce879c87a4258ef20cbf2487/languages/english/embeddings/anna.safetensors','5ea82f78db006c9fd34e32ddd5aae82674b5b32646097977436458d00af80dfa'),
 'model.safetensors':(a.hub/'models--kyutai--pocket-tts/snapshots/39592ff23c9ef80098bb74895d104c26275fe2c9/languages/english/model.safetensors','473f47d99560bd50eb8b4509d3cacfe7f316ab20bdca86505403a2e6a936a6e9'),
 'tokenizer.model':(a.hub/'models--kyutai--pocket-tts-without-voice-cloning/snapshots/d29db7978e464fb90cb3359ee0c69a273b9142cc/languages/english/tokenizer.model','d461765ae179566678c93091c5fa6f2984c31bbe990bf1aa62d92c64d91bc3f6'),
}
config=a.pocket_package/'config/english.yaml'
if hashlib.sha256(config.read_bytes()).hexdigest()!='463605b15840d1c62666d1669b0fc70495c45ef4e8fc8ebda422f7e0fbc0f546':
 p.error('installed model config differs from inspected production profile')
for path,digest in assets.values():
 with path.open('rb') as f:
  if hashlib.file_digest(f,'sha256').hexdigest()!=digest:p.error('cached asset differs from pinned profile')
text=config.read_text()
text=text.replace('hf://kyutai/pocket-tts/languages/english/model.safetensors@39592ff23c9ef80098bb74895d104c26275fe2c9','/models/model.safetensors')
text=text.replace('hf://kyutai/pocket-tts-without-voice-cloning/languages/english/model.safetensors@d29db7978e464fb90cb3359ee0c69a273b9142cc','/models/model.safetensors')
text=text.replace('hf://kyutai/pocket-tts-without-voice-cloning/languages/english/tokenizer.model@d29db7978e464fb90cb3359ee0c69a273b9142cc','/models/tokenizer.model')
if 'hf://' in text:p.error('unresolved remote model reference')
a.output.mkdir(parents=True,exist_ok=False)
for name,(path,_) in assets.items():shutil.copyfile(path,a.output/name)
(a.output/'config.yaml').write_text(text)
pins={name:digest for name,(_,digest) in assets.items()}
pins['config.yaml']=hashlib.sha256(text.encode()).hexdigest()
(a.output/'pins.json').write_text(json.dumps(pins,indent=2)+'\n')
print(json.dumps(pins,indent=2))
