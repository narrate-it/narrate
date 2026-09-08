import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('driver', Path(__file__).with_name('driver.py'))
driver = importlib.util.module_from_spec(spec)
spec.loader.exec_module(driver)

class Response:
    def __init__(self, data): self.data = json.dumps(data).encode()
    def __enter__(self): return self
    def __exit__(self, *args): pass
    def read(self): return self.data

class PipelineTest(unittest.TestCase):
    def exercise_transport(self, streaming=False):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            script = 'First paragraph.\n\nSecond paragraph.'
            for name in ['render.py','transcribe.py','script.txt']:
                (root/name).write_text(script)
            manifest = {'complete':True,'script_sha256':hashlib.sha256(script.encode()).hexdigest(),
                        'paragraphs':[{'duration_seconds':2},{'duration_seconds':3}],
                        'duration_seconds':5.65,'gap_seconds':0.65}
            calls, commands, played = [], [], []
            class Player:
                def add(self, path): played.append((Path(path).name, len(calls)))
                def check(self): pass
                def finish(self): pass
                def close(self): pass
            status_reads = 0
            def request(req, **kwargs):
                nonlocal status_reads
                if isinstance(req,str):
                    response = Response({})
                    response.data = b'PARAGRAPH 1/2' if status_reads == 1 else b'PARAGRAPH 1/2\nPARAGRAPH 2/2'
                    return response
                calls.append((req.method,req.full_url))
                if req.full_url.endswith('/resources'): return Response({'available':{'cpuMillis':1000,'memoryMB':6144}})
                if req.method == 'GET':
                    status_reads += 1
                    return Response({'status':'running' if streaming and status_reads == 1 else 'completed'})
                if req.method == 'POST':
                    pod = json.loads(req.data)
                    self.assertEqual(pod['spec']['containers'][0]['resources']['limits'],{'cpu':'1','memory':'6Gi'})
                    self.assertNotIn(script,pod['spec']['containers'][0]['args'][0])
                return Response({})
            def command(args, **kwargs):
                commands.append(args)
                if args[0] == 'scp':
                    name = args[-2].split('/')[-1]
                    data = {'manifest.json':json.dumps(manifest),'heard.txt':script,'heard.json':'{}','coach-raw.wav':'wave','paragraph-001.wav':'one','paragraph-002.wav':'two'}[name]
                    Path(args[-1]).write_text(data)
            cfg = dict(directory=temp,spark_url='http://spark.test',ssh_host='user@host',ssh_uid=1000,
                       voice='michael',gap_ms=650,resume=False,playback=streaming,format='MP3',output=str(root/'out.mp3'))
            with patch.object(driver.urllib.request,'urlopen',side_effect=request), patch.object(driver.subprocess,'run',side_effect=command), patch.object(driver,'Playback',return_value=Player()) as player, patch.object(driver.time,'sleep'):
                driver.run(cfg)
            self.assertEqual(calls[-1][0],'DELETE')
            self.assertEqual(len([c for c in commands if c[0]=='scp']),6 if streaming else 4)
            if streaming:
                self.assertEqual(played,[('paragraph-001.wav',3),('paragraph-002.wav',4)])
            else:
                player.assert_not_called()
            self.assertIn('libmp3lame',commands[-2])
            self.assertIn('loudnorm=I=-18:TP=-1.5:LRA=11',commands[-2])
            self.assertEqual(json.loads((root/'result.json').read_text())['starts'],[0,2.65])
            self.assertEqual(json.loads((root/'verification.json').read_text())['alignment_ratio'],1)
            self.assertIn('START=2650',(root/'chapters.ffmetadata').read_text())

    def test_saved_output_never_plays(self):
        self.exercise_transport(False)

    def test_first_paragraph_plays_while_render_running(self):
        self.exercise_transport(True)

    def test_capacity_refuses_submission(self):
        with tempfile.TemporaryDirectory() as temp:
            with patch.object(driver.urllib.request,'urlopen',return_value=Response({'available':{'cpuMillis':999,'memoryMB':6144}})) as request:
                with self.assertRaisesRegex(RuntimeError,'1 free CPU'):
                    driver.run(dict(directory=temp,spark_url='http://spark.test',resume=False))
                self.assertEqual(request.call_count,1)

    def test_failed_job_is_cleaned_up(self):
        with tempfile.TemporaryDirectory() as temp:
            for name in ['render.py','transcribe.py','script.txt']: (Path(temp)/name).write_text('text')
            calls=[]
            def request(req,**kwargs):
                if isinstance(req,str): return Response({'logs':'failure'})
                calls.append(req.method)
                if req.full_url.endswith('/resources'): return Response({'available':{'cpuMillis':1000,'memoryMB':6144}})
                return Response({'status':'failed'})
            cfg=dict(directory=temp,spark_url='http://spark.test',resume=False,voice='michael',gap_ms=650,ssh_uid=1000)
            with patch.object(driver.urllib.request,'urlopen',side_effect=request):
                with self.assertRaisesRegex(RuntimeError,'failed'): driver.run(cfg)
            self.assertEqual(calls,['GET','POST','GET','DELETE'])

if __name__ == '__main__': unittest.main()
