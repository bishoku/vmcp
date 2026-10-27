#!/usr/bin/env python3
"""Test-only local real-model OpenAI-compatible embedding server; no response cache.

Install fastembed==0.9.1 in an isolated venv. Bind only a test node/loopback IP.
Only use synthetic metadata: this fixture has no authentication or TLS.
"""
import argparse
import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from fastembed import TextEmbedding

parser = argparse.ArgumentParser()
parser.add_argument('--host', default='127.0.0.1')
parser.add_argument('--port', type=int, default=18091)
parser.add_argument('--cache', default='/tmp/vmcp-omni-model')
parser.add_argument('--model', default='sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2')
args = parser.parse_args()
model_name = args.model
model = TextEmbedding(model_name, cache_dir=args.cache, threads=2)
slots = threading.BoundedSemaphore(16)
failing = threading.Event()

class Handler(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_POST(self):
        if self.path in ('/test/fail', '/test/recover'):
            (failing.set if self.path.endswith('fail') else failing.clear)()
            self.send_response(204); self.end_headers(); return
        if self.path != '/v1/embeddings':
            self.send_error(404); return
        if failing.is_set():
            self.send_error(503); return
        size = int(self.headers.get('Content-Length', '0'))
        if not 0 < size <= 1024*1024:
            self.send_error(413); return
        data = json.loads(self.rfile.read(size))
        texts = data['input']
        if data['model'] != model_name or not isinstance(texts, list) or not 1 <= len(texts) <= 32:
            self.send_error(400); return
        if not slots.acquire(blocking=False):
            self.send_error(429); return
        try:
            vectors = list(model.embed(texts))
            body = json.dumps({'data': [{'index': i, 'embedding': v.tolist()} for i,v in enumerate(vectors)]}).encode()
            self.send_response(200); self.send_header('Content-Type','application/json'); self.send_header('Content-Length',str(len(body))); self.end_headers(); self.wfile.write(body)
        finally:
            slots.release()

print(f'Ready: {model_name} on {args.host}:{args.port}', flush=True)
ThreadingHTTPServer((args.host,args.port),Handler).serve_forever()
