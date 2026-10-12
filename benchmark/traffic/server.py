"""Local-only benchmark traffic UI. No credentials or tool results are persisted."""
import concurrent.futures
import json
import math
import os
from pathlib import Path
import threading
import time
import urllib.parse
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from client import Client

CONTROL = os.getenv('TRAFFIC_CONTROL_URL', 'http://control-plane:8081/api/v1')
GATEWAY = os.getenv('TRAFFIC_GATEWAY_URL', 'http://gateway')
ADMIN = os.getenv('TRAFFIC_ADMIN_TOKEN', 'demo-admin-token')
ORIGIN = os.getenv('TRAFFIC_ORIGIN', 'http://localhost:3302')

class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        raise ValueError('Redirect refused')

OPENER = urllib.request.build_opener(NoRedirect)

class TrafficClient(Client):
    def request(self, method, message=None, url=None):
        headers = {'Accept': 'application/json, text/event-stream', 'Content-Type': 'application/json', 'MCP-Protocol-Version': '2025-06-18'}
        if self.token: headers['Authorization'] = 'Bearer ' + self.token
        if self.sid: headers['Mcp-Session-Id'] = self.sid
        return OPENER.open(urllib.request.Request(self.url, json.dumps(message).encode() if message is not None else None, headers, method=method), timeout=10)


def validate(body):
    slug, tool, key = body.get('server', ''), body.get('tool', ''), body.get('key', '')
    if not isinstance(slug, str) or not slug or len(slug) > 128 or any(c not in 'abcdefghijklmnopqrstuvwxyz0123456789-_' for c in slug):
        raise ValueError('Select a valid virtual server')
    if not isinstance(key, str) or len(key) > 4096: raise ValueError('Invalid agent key')
    if not isinstance(tool, str) or len(tool) > 256: raise ValueError('Invalid tool')
    return slug, tool, key


def discover(slug, key, factory=TrafficClient):
    client = factory(GATEWAY + '/mcp/v1/virtual/' + slug, key)
    try:
        client.initialize()
        result, cursor = [], None
        for _ in range(20):
            page = client.call('tools/list', {'cursor': cursor} if cursor else {})
            result.extend(page.get('tools', []))
            cursor = page.get('nextCursor')
            if not cursor: return result
        raise ValueError('Tool catalog exceeds discovery limit')
    finally:
        try: client.close()
        except Exception: pass


class Job:
    def __init__(self, body, factory=TrafficClient):
        self.slug, self.tool, self.key = validate(body)
        self.rate, self.duration = float(body.get('rate', 0)), float(body.get('duration', 0))
        workers = body.get('workers', 4)
        if isinstance(workers, bool) or not isinstance(workers, int) or not 1 <= workers <= 16: raise ValueError('Workers must be 1–16')
        if not math.isfinite(self.rate) or not .1 <= self.rate <= 100: raise ValueError('Rate must be 0.1–100 requests/second')
        if not math.isfinite(self.duration) or not 1 <= self.duration <= 600: raise ValueError('Duration must be 1–600 seconds')
        self.arguments = body.get('arguments', {})
        if not isinstance(self.arguments, dict): raise ValueError('Arguments must be a JSON object')
        if not self.tool: raise ValueError('Select a tool')
        self.workers, self.factory = workers, factory
        self.lock, self.stop = threading.Lock(), threading.Event()
        self.stats = dict(state='starting', sent=0, ok=0, tool_errors=0, errors=0, skipped=0, active=0)
        self.samples = []
        self.started = time.monotonic()

    def snapshot(self):
        with self.lock:
            values = sorted(self.samples)
            elapsed = max(.001, (getattr(self, 'ended', time.monotonic()) - self.started))
            return dict(self.stats, server=self.slug, tool=self.tool, elapsed=round(elapsed, 1), completed_rps=round((self.stats['ok'] + self.stats['errors'] + self.stats['tool_errors']) / elapsed, 2), p95_ms=round(values[int((len(values)-1)*.95)] * 1000, 1) if values else 0)

    def request(self):
        started, outcome = time.monotonic(), 'errors'
        client = self.factory(GATEWAY + '/mcp/v1/virtual/' + self.slug, self.key)
        try:
            client.initialize()
            result = client.call('tools/call', {'name': self.tool, 'arguments': self.arguments})
            outcome = 'tool_errors' if result.get('isError') else 'ok'
        except Exception:
            pass  # Never log credentials, arguments, upstream errors or results.
        finally:
            with self.lock:
                self.stats[outcome] += 1
                self.stats['active'] -= 1
                self.samples.append(time.monotonic() - started)
                if len(self.samples) > 10000: self.samples.pop(0)
            try: client.close()
            except Exception: pass

    def run(self):
        self.started = time.monotonic()
        with self.lock: self.stats['state'] = 'running'
        try:
            with concurrent.futures.ThreadPoolExecutor(max_workers=self.workers) as pool:
                pending = set()
                next_tick = self.started
                while not self.stop.is_set() and time.monotonic() < self.started + self.duration:
                    if self.stop.wait(max(0, next_tick - time.monotonic())): break
                    if time.monotonic() >= self.started + self.duration: break
                    pending = {f for f in pending if not f.done()}
                    with self.lock:
                        if len(pending) >= self.workers: self.stats['skipped'] += 1
                        else:
                            self.stats['active'] += 1
                            self.stats['sent'] += 1
                            pending.add(pool.submit(self.request))
                    next_tick = max(next_tick + 1 / self.rate, time.monotonic())
                with self.lock: self.stats['state'] = 'draining'
        finally:
            self.key, self.arguments = '', {}
            with self.lock:
                self.ended = time.monotonic()
                self.stats['state'] = 'stopped' if self.stop.is_set() else 'finished'


class Handler(BaseHTTPRequestHandler):
    job = None
    job_lock = threading.Lock()
    def log_message(self, *args): pass
    def reply(self, status, value, content='application/json'):
        data = json.dumps(value).encode() if content == 'application/json' else value
        self.send_response(status)
        self.send_header('Content-Type', content)
        self.send_header('Cache-Control', 'no-store')
        self.send_header('X-Content-Type-Options', 'nosniff')
        self.send_header('Content-Security-Policy', "default-src 'self'; script-src 'self'; style-src 'self'; frame-ancestors 'none'")
        self.send_header('Content-Length', str(len(data)))
        self.end_headers()
        self.wfile.write(data)
    def trusted(self):
        return self.headers.get('Host') == urllib.parse.urlsplit(ORIGIN).netloc and self.headers.get('Origin', ORIGIN) == ORIGIN
    def do_GET(self):
        if not self.trusted(): return self.reply(403, {'error': 'Untrusted origin'})
        try:
            if self.path in ('/', '/app.js', '/style.css'):
                name, kind = {'/': ('index.html', 'text/html'), '/app.js': ('app.js', 'application/javascript'), '/style.css': ('style.css', 'text/css')}[self.path]
                return self.reply(200, Path(__file__).with_name(name).read_bytes(), kind)
            if self.path == '/api/status': return self.reply(200, self.job.snapshot() if self.job else {'state': 'idle'})
            if self.path == '/api/servers':
                items, cursor = [], ''
                for _ in range(20):
                    req = urllib.request.Request(CONTROL + '/virtual-servers?limit=100' + ('&cursor=' + urllib.parse.quote(cursor) if cursor else ''), headers={'Authorization': 'Bearer ' + ADMIN})
                    with OPENER.open(req, timeout=10) as response: page = json.load(response)
                    items.extend({'slug': x['slug'], 'name': x.get('name', x['slug'])} for x in (page.get('items') or []))
                    cursor = page.get('nextCursor')
                    if not cursor: return self.reply(200, items)
                raise ValueError('Server catalog exceeds limit')
            self.reply(404, {'error': 'Not found'})
        except Exception: self.reply(502, {'error': 'Unable to read configured virtual servers'})
    def do_POST(self):
        if not self.trusted() or self.headers.get('Origin') != ORIGIN: return self.reply(403, {'error': 'Untrusted origin'})
        if self.headers.get('Content-Type') != 'application/json': return self.reply(415, {'error': 'JSON required'})
        try:
            size = int(self.headers.get('Content-Length', '0'))
            if not 0 < size <= 65536: raise ValueError('Request too large or empty')
            body = json.loads(self.rfile.read(size))
            if not isinstance(body, dict): raise ValueError('Expected JSON object')
            if self.path == '/api/tools':
                slug, _, key = validate(body)
                return self.reply(200, discover(slug, key))
            if self.path == '/api/start':
                with self.job_lock:
                    if self.job and self.job.snapshot()['state'] in ('starting', 'running', 'draining'): return self.reply(409, {'error': 'Stop or finish the current run first'})
                    job = Job(body)
                    if job.tool not in [t['name'] for t in discover(job.slug, job.key)]: raise ValueError('Tool is not exposed to this agent')
                    Handler.job = job
                    threading.Thread(target=job.run, daemon=True).start()
                return self.reply(200, job.snapshot())
            if self.path == '/api/stop':
                if self.job: self.job.stop.set()
                return self.reply(200, {'stopping': True})
            self.reply(404, {'error': 'Not found'})
        except (ValueError, TypeError, OverflowError): self.reply(400, {'error': 'Check server, tool, arguments, rate (0.1–100), duration (1–600) and workers (1–16)'})
        except Exception: self.reply(502, {'error': 'MCP discovery failed. Check agent key, permissions and gateway availability.'})

if __name__ == '__main__':
    ThreadingHTTPServer(('0.0.0.0', 8080), Handler).serve_forever()
