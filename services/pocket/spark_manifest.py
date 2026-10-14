#!/usr/bin/env python3
"""Render a concrete Spark Pod; never submits or changes running workloads."""
import argparse
import json
import re

p = argparse.ArgumentParser()
p.add_argument('--name', required=True)
p.add_argument('--image', required=True, help='Prepared ARM64 image with /app/server.py; sha256 digest required')
p.add_argument('--models-host-path', required=True)
p.add_argument('--token-directory', required=True, help='Dedicated directory containing only token')
p.add_argument('--anna-sha256', default='5ea82f78db006c9fd34e32ddd5aae82674b5b32646097977436458d00af80dfa')
p.add_argument('--config-sha256', default='193c319c5148f03fc8310423dbaa9022de557dce97ed89b06d668f1af911c859')
a = p.parse_args()
if not re.fullmatch(r'[a-z0-9][a-z0-9-]{0,61}[a-z0-9]', a.name):
    p.error('unique DNS label required')
if not re.fullmatch(r'[^\s]+@sha256:[0-9a-f]{64}', a.image):
    p.error('immutable image digest required')
if not all(re.fullmatch(r'[0-9a-f]{64}', v) for v in [a.anna_sha256, a.config_sha256]):
    p.error('asset/config SHA-256 required')
if not all(v.startswith('/') and v != '/' for v in [a.models_host_path, a.token_directory]):
    p.error('explicit narrow absolute mount directories required')
print(json.dumps({
    'apiVersion': 'v1', 'kind': 'Pod', 'metadata': {'name': a.name},
    'spec': {'restartPolicy': 'Never', 'containers': [{
        'name': 'anna', 'image': a.image,
        'command': ['python', '/app/server.py'],
        'args': ['--host', '0.0.0.0', '--port', '8092', '--token-file', '/run/narrate/token',
                 '--model-config', '/models/config.yaml', '--model-config-sha256', a.config_sha256,
                 '--anna-asset', '/models/anna.safetensors', '--anna-sha256', a.anna_sha256],
        'ports': [{'containerPort': 8092, 'hostPort': 8092}],
        'resources': {'limits': {'cpu': '2', 'memory': '4Gi'}},
        'volumeMounts': [{'name': 'models', 'mountPath': '/models', 'readOnly': True},
                         {'name': 'token', 'mountPath': '/run/narrate', 'readOnly': True}],
    }], 'volumes': [{'name': 'models', 'hostPath': {'path': a.models_host_path}},
                    {'name': 'token', 'hostPath': {'path': a.token_directory}}]},
}, indent=2))
