import asyncio,base64,hashlib,importlib.metadata,json,logging,os,pathlib,signal,time,uuid
signal.alarm(180)
logging.disable(logging.CRITICAL)
print(json.dumps({'whisper_cache_directories':[p.name for p in pathlib.Path('/hub').glob('*whisper*')]}),flush=True)
for package in ['pocket-tts','torch','aiohttp']:
 print(json.dumps({'package':package,'version':importlib.metadata.version(package)}),flush=True)
import torch
from pocket_tts import TTSModel
from aiohttp.test_utils import TestClient,TestServer
from server import Service,application

torch.set_num_threads(1)
torch.manual_seed(20261009)
config=pathlib.Path('/models/config.yaml')
anna=pathlib.Path('/models/anna.safetensors')
assert hashlib.sha256(config.read_bytes()).hexdigest()=='193c319c5148f03fc8310423dbaa9022de557dce97ed89b06d668f1af911c859'
assert hashlib.sha256(anna.read_bytes()).hexdigest()=='5ea82f78db006c9fd34e32ddd5aae82674b5b32646097977436458d00af80dfa'
started=time.monotonic()
model=TTSModel.load_model(config=str(config))
voice=model.get_state_for_audio_prompt(str(anna))
print(json.dumps({'model_load_seconds':time.monotonic()-started,'sample_rate':model.sample_rate}),flush=True)

async def main():
 token=uuid.uuid4().hex
 service=Service(model,voice,token)
 client=TestClient(TestServer(application(service)),headers={'Authorization':'Bearer '+token})
 await client.start_server()
 try:
  for generation,text in enumerate(['Hello, I am Zati. How can I help?','Of course. Take your time.'],1):
   started=time.monotonic(); first=None;frames=0; samples=0;terminal=False;last=0;maxgap=0;audio=hashlib.sha256()
   response=await client.post('/v1/synthesize',json=dict(generation=generation,text_sequence=1,voice='anna',text=text))
   assert response.status==200
   async for line in response.content:
    item=json.loads(line)
    if item['type']=='frame':
     now=time.monotonic()-started
     if first is None:first=now
     else:maxgap=max(maxgap,now-last)
     last=now;frames+=1;samples+=item['samples'];audio.update(base64.b64decode(item['pcm']))
    elif item['type']=='done':terminal=True;assert item['offset']==samples
   print(json.dumps({'generation':generation,'first_pcm_seconds':first,'total_seconds':time.monotonic()-started,'frames':frames,'samples':samples,'max_frame_gap_seconds':maxgap,'terminal':terminal,'pcm_sha256':audio.hexdigest()}),flush=True)
   assert terminal and samples>0
  response=await client.post('/v1/synthesize',json=dict(generation=3,text_sequence=1,voice='anna',text='This is a deliberately long response. '*60))
  await response.content.readline()
  started=time.monotonic();response.close()
  while service.busy.locked() and time.monotonic()-started<4:await asyncio.sleep(.01)
  print(json.dumps({'cancel_seconds':time.monotonic()-started,'worker_released':not service.busy.locked(),'healthy':service.healthy}),flush=True)
  assert not service.busy.locked()
  response=await client.post('/v1/synthesize',json=dict(generation=4,text_sequence=1,voice='anna',text='I am listening.'))
  records=[json.loads(line) for line in (await response.text()).splitlines()]
  recovered=records[-1]['type']=='done'
  print(json.dumps({'recovered_after_cancellation':recovered,'external_connections_allowed':False}),flush=True)
  assert recovered
 finally:await client.close()
asyncio.run(main())
