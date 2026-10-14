#!/usr/bin/env python3
"""Authenticated persistent Anna PCM service. Models/assets must be prepared offline."""
import argparse
import asyncio
import base64
import hashlib
import hmac
import json
import logging
import os
import queue
import threading
import time
from pathlib import Path

from aiohttp import web

FORMAT = {"encoding": "pcm_s16le", "sample_rate": 24000, "channels": 1}


class Service:
    def __init__(self, model, voice, token, timeout=40):
        self.model, self.voice, self.token = model, voice, token
        self.timeout = timeout
        self.busy = threading.Lock()
        self.healthy = True

    def authorize(self, request):
        return hmac.compare_digest(request.headers.get("Authorization", ""), "Bearer " + self.token)

    async def health(self, request):
        if not self.authorize(request):
            raise web.HTTPUnauthorized()
        return web.json_response({"ready": self.healthy, "voice": "anna", "format": FORMAT}, status=200 if self.healthy else 503)

    async def synthesize(self, request):
        if not self.authorize(request):
            raise web.HTTPUnauthorized()
        try:
            data = await request.json()
            generation, text_sequence = data["generation"], data["text_sequence"]
            text = data["text"]
            if (type(generation) is not int or not 0 < generation < 2**64
                    or type(text_sequence) is not int or not 0 < text_sequence < 2**64
                    or data["voice"] != "anna" or not isinstance(text, str)
                    or not text.strip() or len(text.encode()) > 16384):
                raise ValueError()
        except (ValueError, KeyError, TypeError):
            raise web.HTTPBadRequest() from None
        if not self.healthy or not self.busy.acquire(blocking=False):
            raise web.HTTPServiceUnavailable()
        stop = threading.Event()
        output = queue.Queue(maxsize=2)
        deadline = time.monotonic() + self.timeout

        def put(item):
            while not stop.is_set() and time.monotonic() < deadline:
                try:
                    output.put(item, timeout=.05)
                    return True
                except queue.Full:
                    pass
            stop.set()
            return False

        def produce():
            try:
                # Pocket's model is not thread safe. This worker owns the model until exit.
                stream = self.model.generate_audio_stream(self.voice, text, copy_state=True, stop=stop)
                for audio in stream:
                    if stop.is_set():
                        break
                    pcm = audio.detach().cpu().clamp(-1, 1).mul(32767).to(dtype=__import__('torch').int16).numpy().astype('<i2', copy=False).tobytes()
                    for index in range(0, len(pcm), 9600):
                        if not put(("frame", pcm[index:index+9600])):
                            return
                if not stop.is_set():
                    put(("done", None))
            except Exception:
                put(("error", None))  # Never leak text/model paths to callers or logs.
            finally:
                self.busy.release()

        worker = threading.Thread(target=produce, daemon=True)
        response = web.StreamResponse(headers={"Content-Type": "application/x-ndjson", "Cache-Control": "no-store"})
        worker.start()
        sequence, offset = 0, 0

        async def send(kind, **fields):
            record = dict(type=kind, generation=generation, text_sequence=text_sequence, **fields)
            await response.write(json.dumps(record, separators=(',', ':')).encode() + b'\n')

        try:
            await response.prepare(request)
            await send("format", format=FORMAT)
            while time.monotonic() < deadline:
                if request.transport is None or request.transport.is_closing():
                    break
                try:
                    kind, pcm = output.get_nowait()
                except queue.Empty:
                    await asyncio.sleep(.01)
                    continue
                if kind == "frame":
                    sequence += 1
                    await asyncio.wait_for(send("frame", sequence=sequence, offset=offset, samples=len(pcm)//2, pcm=base64.b64encode(pcm).decode()), timeout=max(.01, deadline-time.monotonic()))
                    offset += len(pcm)//2
                elif kind == "done" and offset:
                    await send("done", sequence=sequence, offset=offset)
                    await response.write_eof()
                    return response
                else:
                    break
            # No terminal success on timeout, cancellation, inference failure or empty audio.
            response.force_close()
            return response
        except (ConnectionError, asyncio.TimeoutError):
            response.force_close()
            return response
        finally:
            stop.set()
            await asyncio.to_thread(worker.join, 2)
            if worker.is_alive():
                self.healthy = False  # Do not admit overlapping inference after failed teardown.


def application(service):
    app = web.Application(client_max_size=20 << 10)
    app.router.add_get('/healthz', service.health)
    app.router.add_post('/v1/synthesize', service.synthesize)
    return app


def verified_file(path, digest):
    path = Path(path)
    if not path.is_file() or len(digest) != 64 or hashlib.sha256(path.read_bytes()).hexdigest() != digest.lower():
        raise ValueError("required pinned asset hash mismatch")
    return path


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--host', default='127.0.0.1')
    parser.add_argument('--port', type=int, default=8092)
    parser.add_argument('--token-file', required=True)
    parser.add_argument('--anna-asset', required=True)
    parser.add_argument('--anna-sha256', required=True)
    parser.add_argument('--model-config', required=True)
    parser.add_argument('--model-config-sha256', required=True)
    args = parser.parse_args()
    token = Path(args.token_file).read_text().strip()
    if len(token) < 32:
        raise ValueError('service token must contain at least 32 characters')
    anna = verified_file(args.anna_asset, args.anna_sha256)
    config = verified_file(args.model_config, args.model_config_sha256)
    os.environ['HF_HUB_OFFLINE'] = '1'
    os.environ['TRANSFORMERS_OFFLINE'] = '1'
    # Avoid upstream performance logging and accidental content diagnostics.
    logging.disable(logging.CRITICAL)
    import torch
    from pocket_tts import TTSModel
    torch.set_num_threads(2)
    torch.manual_seed(20261009)
    model = TTSModel.load_model(config=str(config))
    if model.sample_rate != 24000:
        raise ValueError('Anna profile requires native 24000 Hz')
    voice = model.get_state_for_audio_prompt(str(anna))
    web.run_app(application(Service(model, voice, token)), host=args.host, port=args.port, access_log=None, handler_cancellation=True, shutdown_timeout=3)


if __name__ == '__main__':
    main()
