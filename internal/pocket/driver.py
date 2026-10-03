"""Run the Pocket TTS stack on a configured remote GPU or CPU."""
import base64
import difflib
import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import signal
import subprocess
import sys
import time
import urllib.request
import urllib.error
import uuid
import wave
from playback import Playback


def run(cfg):
    player = Playback(cfg['gap_ms']) if cfg.get('playback') else None
    try:
        produce(cfg, player)
        if player:
            player.finish()
    finally:
        if player:
            player.close()


def produce(cfg, player):
    root = Path(cfg['directory'])
    for key in ('spark_url', 'ssh_host', 'host_path', 'python_path'):
        if not cfg.get(key): raise RuntimeError('UNAVAILABLE: missing remote setting ' + key)
    cfg.setdefault('device', 'cuda'); cfg.setdefault('output_subdir', 'output')
    cfg.setdefault('cache_subdir', 'cache'); cfg.setdefault('image', 'nvcr.io/nvidia/pytorch:26.02-py3')
    cfg.setdefault('speed', 1.0); cfg.setdefault('connect_timeout_seconds', 3)
    base = cfg['spark_url'].rstrip('/')
    def emit(message):
        print('narrate: ' + message, file=sys.stderr, flush=True)
    def api(method, path, body=None):
        request = urllib.request.Request(base + path, data=json.dumps(body).encode() if body else None,
                                         method=method, headers={'Content-Type': 'application/json'})
        try:
            with urllib.request.urlopen(request, timeout=cfg['connect_timeout_seconds']) as response:
                data = response.read()
        except (urllib.error.URLError, urllib.error.HTTPError) as exc:
            raise RuntimeError('UNAVAILABLE: remote API request failed') from exc
        return json.loads(data) if data else {}

    raw = root / 'narration-raw.wav'
    if not (cfg['resume'] and raw.exists() and (root / 'heard.json').exists() and (root / 'manifest.json').exists()):
        resources = api('GET', '/api/v1/resources')['available']
        if resources['cpuMillis'] < 1000 or resources['memoryMB'] < 6144:
            raise RuntimeError('UNAVAILABLE: remote needs 1 free CPU and 6 GiB RAM (1 free CPU required)')
        if cfg['device'] == 'cuda' and resources.get('gpuCount', 0) < 1:
            raise RuntimeError('UNAVAILABLE: remote GPU capacity unavailable')
        name = 'narrate-' + uuid.uuid4().hex[:16]
        remote = '/host/' + cfg['output_subdir'].strip('/') + '/' + name
        # Every payload is base64 encoded; narration never becomes shell syntax.
        payload = []
        for filename in ['render.py', 'transcribe.py', 'script.txt']:
            encoded = base64.b64encode((root / filename).read_bytes()).decode()
            payload.append('printf %s ' + shlex.quote(encoded) + ' | base64 -d > /tmp/' + filename)
        python = cfg['python_path']
        payload += [shlex.join([python, '/tmp/render.py', '--script', '/tmp/script.txt', '--output', remote,
                               '--voice', cfg['voice'], '--gap-ms', str(cfg['gap_ms']), '--ssh-uid', str(cfg['ssh_uid']), '--device', cfg['device'], '--cache', '/host/' + cfg['cache_subdir'].strip('/') + '/hf', '--speed', str(cfg['speed'])]),
                    shlex.join([python, '/tmp/transcribe.py', '--directory', remote, '--device', cfg['device'], '--cache', '/host/' + cfg['cache_subdir'].strip('/') + '/whisper']),
                    shlex.join(['chown','-R',str(cfg['ssh_uid']),remote])]
        pod = {'apiVersion':'v1', 'kind':'Pod', 'metadata':{'name':name}, 'spec':{
            'restartPolicy':'Never', 'containers':[{'name':'render',
            'image':cfg['image'], 'command':['bash','-lc'],
            'args':['set -e; umask 077; ' + '; '.join(payload)],
            'resources':{'limits':{'cpu':'1','memory':'6Gi', **({'nvidia.com/gpu':'1'} if cfg['device']=='cuda' else {})}},
            'volumeMounts':[{'name':'cache','mountPath':'/host'}]}],
            'volumes':[{'name':'cache','hostPath':{'path':cfg['host_path'],'type':'Directory'}}]}}
        def download(filename):
            dest = root / (filename + '.partial')
            try:
                subprocess.run(['scp','-q','-o','BatchMode=yes','-o','ConnectTimeout=10',
                                cfg['ssh_host'] + ':' + cfg['host_path'].rstrip('/') + '/' + cfg['output_subdir'].strip('/') + '/' + name + '/' + filename,
                                str(dest)], check=True, timeout=300)
            except (subprocess.SubprocessError, OSError) as exc:
                raise RuntimeError('UNAVAILABLE: remote artifact transfer failed') from exc
            dest.replace(root / filename)
            return root / filename

        next_paragraph = 1
        total_paragraphs = None
        last_report = 0.0
        whisper_started = False
        whisper_done = False
        render_started = False
        submitted = False
        try:
            api('POST', '/api/v1/pods', pod)
            submitted = True
            deadline = time.monotonic() + 7200
            emit('Pocket TTS + Whisper job ' + name)
            while True:
                job_status = api('GET', '/api/v1/pods/' + name).get('status')
                logs = ''
                if job_status != 'pending':
                    try:
                        with urllib.request.urlopen(base + '/api/v1/pods/' + name + '/logs', timeout=15) as response:
                            logs = response.read().decode(errors='replace')
                    except urllib.error.HTTPError as exc:
                        if exc.code != 404 or job_status == 'completed':
                            raise
                match = re.search(r'START paragraphs=(\d+)', logs)
                if match:
                    total_paragraphs = int(match.group(1))
                    if not render_started:
                        emit(f'Pocket rendering {total_paragraphs} paragraphs')
                        render_started = True
                if 'Loading Whisper base.en for complete audio check' in logs and not whisper_started:
                    emit('Whisper verification started')
                    whisper_started = True
                if 'TRANSCRIBED words=' in logs and not whisper_done:
                    emit('Whisper verification complete')
                    whisper_done = True
                if player:
                    player.check()
                    ready = [int(n) for n in re.findall(r'PARAGRAPH (\d+)/', logs)]
                    while ready and next_paragraph <= max(ready):
                        path = download(f'paragraph-{next_paragraph:03d}.wav')
                        player.add(path)
                        if total_paragraphs:
                            emit(f'Pocket paragraph {next_paragraph}/{total_paragraphs} ready while job {job_status}')
                        else:
                            emit(f'Pocket paragraph {next_paragraph} ready while job {job_status}')
                        next_paragraph += 1
                if job_status == 'completed':
                    if player and next_paragraph == 1:
                        raise RuntimeError('completed job published no playable paragraphs')
                    break
                if job_status in ['failed', 'error', 'stopped']:
                    raise RuntimeError('UNAVAILABLE: remote synthesis failed; inspect job logs')
                if time.monotonic() > deadline:
                    raise RuntimeError('Pocket render timed out after two hours')
                if time.monotonic() - last_report >= 15:
                    if total_paragraphs:
                        emit(f'Pocket/Whisper {job_status} ({next_paragraph - 1}/{total_paragraphs} paragraphs ready)')
                    else:
                        emit('Pocket/Whisper ' + str(job_status))
                    last_report = time.monotonic()
                time.sleep(1 if player else 15)
            for filename in ['narration-raw.wav','manifest.json','heard.txt','heard.json']:
                download(filename)
        finally:
            if submitted:
                try:
                    with urllib.request.urlopen(base + '/api/v1/pods/' + name + '/logs', timeout=15) as response:
                        (root / 'spark.log').write_bytes(response.read())
                except Exception:
                    pass
                try:
                    api('DELETE', '/api/v1/pods/' + name)
                except Exception as exc:
                    emit('cleanup failed for ' + name + ': ' + str(exc))
    elif player:
        player.add(raw)
        emit('playing cached narration')
    manifest = json.loads((root / 'manifest.json').read_text())
    script = (root / 'script.txt').read_text()
    if not manifest.get('complete') or manifest['script_sha256'] != hashlib.sha256(script.encode()).hexdigest():
        raise RuntimeError('Pocket manifest does not match the complete script')
    heard = (root / 'heard.txt').read_text()
    normalize = lambda text: re.findall(r"\w+", text.lower())
    expected, actual = normalize(script), normalize(heard)
    comparison = difflib.SequenceMatcher(None, expected, actual, autojunk=False)
    check = {'engine':'Whisper base.en', 'alignment_ratio':comparison.ratio(),
             'note':'Automated comparison, not a listening review or word error rate.',
             'differences':[{'operation':op,'script':' '.join(expected[a:b]),'heard':' '.join(actual[c:d])}
                            for op,a,b,c,d in comparison.get_opcodes() if op != 'equal']}
    (root / 'verification.json').write_text(json.dumps(check, indent=2))
    if not actual:
        raise RuntimeError('Whisper returned an empty transcription')
    if check['alignment_ratio'] < 0.8:
        emit('transcription differs substantially; review ' + str(root / 'verification.json'))
    emit('encoding final audio with FFmpeg')
    speed = float(cfg.get('speed', 1.0))
    paced_raw = raw
    if speed != 1.0:
        filters = []
        remaining = speed
        while remaining > 2.0:
            filters.append('atempo=2.0'); remaining /= 2.0
        while remaining < 0.5:
            filters.append('atempo=0.5'); remaining /= 0.5
        filters.append('atempo=' + str(remaining))
        segments = []
        source_offset = 0.0
        for index, paragraph in enumerate(manifest['paragraphs']):
            source_duration = paragraph['duration_seconds']
            segment = root / f'paced-{index:03d}.wav'
            subprocess.run(['ffmpeg','-nostdin','-v','error','-y','-ss',str(source_offset),'-t',str(source_duration),'-i',str(raw),'-af',','.join(filters),'-c:a','pcm_s16le',str(segment)],check=True,timeout=1800)
            with wave.open(str(segment),'rb') as clip: paragraph['duration_seconds'] = clip.getnframes()/clip.getframerate()
            segments.append(segment)
            source_offset += source_duration
            if index < len(manifest['paragraphs'])-1:
                gap = root / f'paced-gap-{index:03d}.wav'; gap_duration = manifest['gap_seconds']
                subprocess.run(['ffmpeg','-nostdin','-v','error','-y','-ss',str(source_offset),'-t',str(gap_duration),'-i',str(raw),'-c:a','pcm_s16le',str(gap)],check=True,timeout=1800)
                segments.append(gap); source_offset += gap_duration
        listfile = root / 'paced-concat.txt'
        listfile.write_text(''.join("file '" + str(path).replace("'", "'\\''") + "'\n" for path in segments))
        paced_raw = root / 'narration-paced.wav'
        subprocess.run(['ffmpeg','-nostdin','-v','error','-y','-f','concat','-safe','0','-i',str(listfile),'-c','copy',str(paced_raw)],check=True,timeout=1800)
    starts = []
    current = 0.0
    metadata = [';FFMETADATA1']
    for index, paragraph in enumerate(manifest['paragraphs']):
        starts.append(current)
        end = current + paragraph['duration_seconds']
        if index < len(manifest['paragraphs']) - 1:
            end += manifest['gap_seconds']
        metadata += ['[CHAPTER]', 'TIMEBASE=1/1000', 'START=' + str(round(current*1000)),
                     'END=' + str(round(end*1000)), 'title=Paragraph ' + str(index+1)]
        current = end
    manifest['duration_seconds'] = current
    (root / 'chapters.ffmetadata').write_text('\n'.join(metadata)+'\n')
    fmt = cfg['format']
    codec, container = {'MP3':('libmp3lame','mp3'),'WAVE':('pcm_s16le','wav'),'AIFF':('pcm_s16be','aiff')}[fmt]
    command = ['ffmpeg','-nostdin','-hide_banner','-loglevel','error','-y','-i',str(paced_raw),
               '-i',str(root/'chapters.ffmetadata'),'-map','0:a','-map_metadata','1','-map_chapters','1',
               '-af','loudnorm=I=-18:TP=-1.5:LRA=11','-ar','44100','-ac','1','-c:a',codec]
    if fmt == 'MP3':
        command += ['-b:a','128k','-id3v2_version','3']
    subprocess.run(command + ['-f',container,cfg['output']],check=True,timeout=1800)
    subprocess.run(['ffmpeg','-nostdin','-v','error','-i',cfg['output'],'-f','null','-'],check=True,timeout=1800)
    emit('Pocket render complete')
    (root / 'result.json').write_text(json.dumps({'duration':manifest['duration_seconds'],'starts':starts}))


def interrupted(signum, frame):
    raise KeyboardInterrupt()

if __name__ == '__main__':
    signal.signal(signal.SIGTERM, interrupted)
    try:
        run(json.load(sys.stdin))
    except (Exception, KeyboardInterrupt) as exc:
        print('narrate: Pocket stack: ' + str(exc), file=sys.stderr)
        sys.exit(1)
