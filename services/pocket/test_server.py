"""Protocol fixtures, not inference qualification."""
import asyncio
import json
import sys
import threading
import types
import unittest
from aiohttp.test_utils import TestClient, TestServer
from server import Service, application

class PCM:
    def detach(self): return self
    def cpu(self): return self
    def clamp(self, *args): return self
    def mul(self, *args): return self
    def to(self, **kwargs): return self
    def numpy(self): return self
    def astype(self, *args, **kwargs): return self
    def tobytes(self): return b'\x04\x00\x05\x00'

class Model:
    def __init__(self, hold=False):
        self.hold = hold
        self.stopped = threading.Event()
    def generate_audio_stream(self, voice, text, copy_state, stop):
        try:
            yield PCM()
            if self.hold: stop.wait(2)
            else: yield PCM()
        finally: self.stopped.set()

class ServerTests(unittest.IsolatedAsyncioTestCase):
    async def asyncSetUp(self):
        self.old_torch = sys.modules.get('torch')
        sys.modules['torch'] = types.SimpleNamespace(int16='int16')
        self.clients = []
    async def asyncTearDown(self):
        for c in self.clients: await c.close()
        if self.old_torch is None: sys.modules.pop('torch', None)
        else: sys.modules['torch'] = self.old_torch
    async def client(self, model, timeout=2):
        service = Service(model, object(), 'secret', timeout)
        c = TestClient(TestServer(application(service)), headers={'Authorization': 'Bearer secret'})
        await c.start_server()
        self.clients.append(c)
        return c, service
    async def post(self,c,generation=1):
        return await c.post('/v1/synthesize',json=dict(generation=generation,text_sequence=1,voice='anna',text='Hello'))
    async def test_ordered_pcm_terminal(self):
        c, _ = await self.client(Model())
        r = await self.post(c,7)
        records = [json.loads(line) for line in (await r.text()).splitlines()]
        self.assertEqual([v['type'] for v in records], ['format','frame','frame','done'])
        self.assertTrue(all(v['generation']==7 for v in records))
        self.assertEqual(records[-1]['offset'],4)
    async def test_authorization_and_validation(self):
        c, _ = await self.client(Model())
        r = await c.get('/healthz',headers={'Authorization':'Bearer wrong'})
        self.assertEqual(r.status,401)
        r = await self.post(c,True)
        self.assertEqual(r.status,400)
    async def test_disconnect_cancels_and_releases(self):
        m = Model(hold=True)
        c, service = await self.client(m)
        r = await self.post(c)
        await r.content.readline()
        r.close()
        self.assertTrue(await asyncio.to_thread(m.stopped.wait,1))
        self.assertFalse(service.busy.locked())
    async def test_overload_and_timeout_not_success(self):
        m = Model(hold=True)
        c, _ = await self.client(m,timeout=.1)
        r = await self.post(c)
        other = await self.post(c,2)
        self.assertEqual(other.status,503)
        self.assertNotIn('"type":"done"',await r.text())
        self.assertTrue(await asyncio.to_thread(m.stopped.wait,1))

if __name__ == '__main__': unittest.main()
