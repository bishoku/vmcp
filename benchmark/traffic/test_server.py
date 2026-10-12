import json
import threading
import time
import unittest
from server import Job, discover, validate, Handler, ORIGIN
from http.server import ThreadingHTTPServer
import urllib.request
import urllib.error
from urllib.parse import urlsplit

class Fake:
    active = peak = 0
    lock = threading.Lock()
    calls = []
    def __init__(self, url, key): self.key = key
    def initialize(self): pass
    def close(self): pass
    def call(self, method, params):
        if method == 'tools/list': return {'tools':[{'name':'sum'}]}
        with self.lock:
            Fake.active += 1
            Fake.peak = max(Fake.active, Fake.peak)
            Fake.calls.append(params)
        time.sleep(.05)
        with self.lock: Fake.active -= 1
        return {'isError': params['arguments'].get('fail', False)}

class Tests(unittest.TestCase):
    def body(self, **extra): return dict(server='demo', tool='sum', key='secret', rate=100, duration=1, workers=2, arguments={}, **extra)
    def test_validation(self):
        for field, value in [('rate',float('nan')),('duration',float('inf')),('workers',True),('workers',17),('arguments',[]),('server','../x'),('tool','')]:
            body=self.body(); body[field]=value
            with self.subTest(field=field), self.assertRaises((ValueError, TypeError)): Job(body, Fake)
    def test_bounded_and_secret_scrub(self):
        Fake.peak=0
        job=Job(self.body(), Fake); job.run(); status=job.snapshot()
        self.assertLessEqual(Fake.peak,2)
        self.assertGreater(status['skipped'],0)
        self.assertEqual(status['sent'],status['ok'])
        self.assertEqual(status['active'],0)
        self.assertEqual(job.key,'')
        self.assertNotIn('secret',json.dumps(status))
    def test_transport_failure(self):
        class Broken(Fake):
            def initialize(self): raise RuntimeError('secret upstream payload')
        body=self.body();body.update(rate=1)
        job=Job(body,Broken);job.run()
        self.assertEqual(job.snapshot()['errors'],1)
        self.assertNotIn('secret',json.dumps(job.snapshot()))
        self.assertEqual(job.key,'')

    def test_stop(self):
        job=Job(self.body(),Fake)
        thread=threading.Thread(target=job.run); thread.start(); time.sleep(.08)
        job.stop.set(); thread.join(2)
        self.assertFalse(thread.is_alive())
        self.assertEqual(job.snapshot()['state'],'stopped')
        self.assertLess(job.snapshot()['sent'],20)
    def test_tool_error(self):
        body=self.body();body.update(rate=1,arguments={'fail':True})
        job=Job(body,Fake);job.run()
        self.assertEqual(job.snapshot()['tool_errors'],1)
    def test_http_origin_and_input_guards(self):
        server = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
        thread = threading.Thread(target=server.serve_forever, daemon=True); thread.start()
        url = 'http://127.0.0.1:' + str(server.server_port)
        try:
            for headers, data, status in [
                ({'Host':'evil.example'}, None, 403),
                ({'Host':urlsplit(ORIGIN).netloc,'Origin':'http://evil.example','Content-Type':'application/json'}, b'{}', 403),
                ({'Host':urlsplit(ORIGIN).netloc,'Origin':ORIGIN,'Content-Type':'text/plain'}, b'{}', 415),
                ({'Host':urlsplit(ORIGIN).netloc,'Origin':ORIGIN,'Content-Type':'application/json'}, b'[]', 400),
            ]:
                with self.assertRaises(urllib.error.HTTPError) as error:
                    urllib.request.urlopen(urllib.request.Request(url + '/api/start', data=data, headers=headers))
                self.assertEqual(error.exception.code,status)
            with urllib.request.urlopen(urllib.request.Request(url + '/api/status',headers={'Host':urlsplit(ORIGIN).netloc})) as response:
                self.assertEqual(json.load(response)['state'],'idle')
        finally:
            server.shutdown(); server.server_close(); thread.join()

    def test_discovery(self): self.assertEqual(discover('demo','secret',Fake),[{'name':'sum'}])

if __name__ == '__main__': unittest.main()
