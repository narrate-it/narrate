#!/usr/bin/env python3
"""Add cached Pocket presets to an existing prepared model bundle, offline."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--embeddings', type=Path, required=True,
                        help='Pinned upstream snapshot embeddings directory')
    parser.add_argument('--output', type=Path, required=True,
                        help='Existing prepared model directory containing anna.safetensors')
    args = parser.parse_args()
    if not (args.output / 'anna.safetensors').is_file():
        parser.error('prepare the model bundle first')
    manifest = {}
    for source in sorted(args.embeddings.glob('*.safetensors')):
        if not source.stem.replace('_', '').isalnum() or len(source.stem) > 64:
            parser.error('invalid preset identifier')
        data = source.read_bytes()
        destination = args.output / source.name
        if destination.exists() and destination.read_bytes() != data:
            parser.error('existing preset differs; use a new prepared bundle')
        if not destination.exists():
            shutil.copyfile(source, destination)
        manifest[source.stem] = {'sha256': hashlib.sha256(data).hexdigest()}
    if not manifest:
        parser.error('no cached presets found')
    path = args.output / 'voices.json'
    if path.exists() and json.loads(path.read_text()) != manifest:
        parser.error('existing manifest differs; use a new prepared bundle')
    path.write_text(json.dumps(manifest, indent=2) + '\n')
    print('Prepared presets: ' + ', '.join(manifest))


if __name__ == '__main__':
    main()
