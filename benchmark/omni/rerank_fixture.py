#!/usr/bin/env python3
"""Synthetic-test-only rerank API. No auth/TLS. Bind loopback, never production.

FastEmbed returns logits for this BGE model; this adapter explicitly maps them to
[0,1] with sigmoid. They are not calibrated probabilities. Model weights are not
bundled with Omni. Install the same fastembed version as embedding_fixture.py.
"""
import argparse
import json
import math
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from fastembed.rerank.cross_encoder import TextCrossEncoder

parser = argparse.ArgumentParser()
parser.add_argument('--host', default='127.0.0.1')
parser.add_argument('--port', type=int, default=18095)
parser.add_argument('--cache', default='/tmp/vmcp-omni-reranker-model')
parser.add_argument('--model', default='BAAI/bge-reranker-v2-m3-int8')
parser.add_argument('--threads', type=int, default=4)
args = parser.parse_args()
model = TextCrossEncoder(args.model, cache_dir=args.cache, threads=args.threads)
slots = threading.BoundedSemaphore(2)
failing = threading.Event()

class Handler(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_GET(self):
        if self.path != '/healthz':
            self.send_error(404)
            return
        self.send_response(200)
        self.end_headers()

    def do_POST(self):
        if self.path in ('/test/fail', '/test/recover'):
            (failing.set if self.path.endswith('fail') else failing.clear)()
            self.send_response(204)
            self.end_headers()
            return
        if self.path != '/v1/rerank':
            self.send_error(404)
            return
        if failing.is_set():
            self.send_error(503)
            return
        size = int(self.headers.get('Content-Length', '0'))
        if not 0 < size <= 1024 * 1024:
            self.send_error(413)
            return
        try:
            body = json.loads(self.rfile.read(size))
            docs, query = body['documents'], body['query']
            if body['model'] != args.model or not isinstance(query, str) or not isinstance(docs, list) or not 1 <= len(docs) <= 100 or not all(isinstance(d, str) for d in docs):
                self.send_error(400)
                return
        except (ValueError, KeyError, TypeError):
            self.send_error(400)
            return
        if not slots.acquire(blocking=False):
            self.send_error(429)
            return
        try:
            raw_scores = list(model.rerank(query, docs, batch_size=20))
            scores = [1 / (1 + math.exp(-max(-80, min(80, s)))) for s in raw_scores]
            response = json.dumps({'results': [{'index': i, 'relevance_score': s} for i, s in enumerate(scores)]}).encode()
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.send_header('Content-Length', str(len(response)))
            self.end_headers()
            self.wfile.write(response)
        finally:
            slots.release()

print(f'Ready: {args.model} sigmoid scores on {args.host}:{args.port}', flush=True)
ThreadingHTTPServer((args.host, args.port), Handler).serve_forever()
