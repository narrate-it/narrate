import hashlib,json,pathlib,signal
signal.alarm(60)
root=pathlib.Path('/pocket')
for path in root.rglob('*.py'):
 text=path.read_text()
 if 'anna' in text.lower():
  lines=text.splitlines()
  for i,line in enumerate(lines):
   if 'anna' in line.lower():print(json.dumps({'source':str(path),'lines':lines[max(0,i-2):i+3]}))
for path in root.rglob('*.yaml'):
 print(json.dumps({'config':str(path),'sha256':hashlib.sha256(path.read_bytes()).hexdigest(),'text':path.read_text()}))
cache=pathlib.Path('/hub')
for model in cache.glob('*pocket*'):
 for path in model.glob('snapshots/**/*'):
  if path.is_file():print(json.dumps({'asset':str(path),'size':path.stat().st_size,'sha256':hashlib.file_digest(path.open('rb'),'sha256').hexdigest()}))
for model in cache.glob('*tts-voices*'):
 for path in model.glob('snapshots/**/*'):
  if path.is_file() and any(x in str(path).lower() for x in ['anna','p228_023']): print(json.dumps({'voice_asset':str(path),'size':path.stat().st_size,'sha256':hashlib.file_digest(path.open('rb'),'sha256').hexdigest()}))
