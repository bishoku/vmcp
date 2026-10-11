"""Synthetic MCP servers for the local vMCP demo; never use real customer data."""
import json
import os
import queue
import threading
import time
import uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

VERSION = '2025-06-18'
PROFILES = {
    'calculator': ('calculate', 'Add, subtract or multiply two numbers', {'a': {'type': 'number'}, 'b': {'type': 'number'}, 'operation': {'type': 'string', 'enum': ['add', 'subtract', 'multiply']}}),
    'knowledge': ('search_docs', 'Search the synthetic engineering handbook', {'query': {'type': 'string'}}),
    'support': ('get_customer', 'Return a synthetic customer with maskable PII', {'customer_id': {'type': 'string'}}),
}


def tool(name, description, properties, readonly=True):
    return {'name': name, 'description': description, 'title': name.replace('_', ' ').title(),
            'inputSchema': {'type': 'object', 'properties': properties, 'additionalProperties': False},
            'annotations': {'readOnlyHint': readonly, 'idempotentHint': readonly}, '_meta': {'demo': True}}


class State:
    def __init__(self, profile):
        if profile not in PROFILES:
            raise ValueError('unknown demo profile')
        self.profile = profile
        self.lock = threading.Lock()
        self.sessions = {}
        self.revision = 1

    def tools(self):
        name, description, properties = PROFILES[self.profile]
        return [tool(name, description, properties),
                tool('delay', 'Stream progress for 1–20 seconds', {'seconds': {'type': 'integer', 'minimum': 1, 'maximum': 20}}),
                tool('publish_update', 'Increment the resource revision and notify subscribers', {}, False),
                tool('fail', 'Return a synthetic tool error', {}),
                tool('ask_host', 'Request negotiated sampling, roots or form elicitation from the MCP host', {'kind': {'type': 'string', 'enum': ['sampling', 'roots', 'elicitation']}})]

    def publish(self):
        with self.lock:
            self.revision += 1
            for session in self.sessions.values():
                for uri in session['subscriptions']:
                    try:
                        session['events'].put_nowait({'jsonrpc': '2.0', 'method': 'notifications/resources/updated', 'params': {'uri': uri}})
                    except queue.Full:
                        pass  # Demo hints are bounded; clients can re-read the revision.
            return self.revision


class Server(ThreadingHTTPServer):
    daemon_threads = True

    def __init__(self, address, state, token=''):
        self.state, self.token = state, token
        super().__init__(address, Handler)


class Handler(BaseHTTPRequestHandler):
    protocol_version = 'HTTP/1.1'

    def log_message(self, *_):
        pass  # Synthetic payloads/credentials are not operational logs.

    def reply(self, status, body=None, headers=None):
        data = json.dumps(body).encode() if body is not None else b''
        self.send_response(status)
        self.send_header('Content-Length', str(len(data)))
        self.send_header('Content-Type', 'application/json')
        for name, value in (headers or {}).items():
            self.send_header(name, value)
        self.end_headers()
        self.wfile.write(data)

    def allowed(self):
        if self.path != '/mcp':
            self.reply(404)
            return False
        if self.server.token and self.headers.get('Authorization') != 'Bearer ' + self.server.token:
            self.reply(401)
            return False
        if self.headers.get('Origin'):
            self.reply(403)  # No browser origins are needed for these private fixtures.
            return False
        version = self.headers.get('MCP-Protocol-Version')
        if version and version != VERSION:
            self.reply(400)
            return False
        return True

    def session(self):
        with self.server.state.lock:
            session = self.server.state.sessions.get(self.headers.get('Mcp-Session-Id'))
            if session:
                session['seen'] = time.monotonic()
            return session

    def do_GET(self):
        if self.path == '/healthz':
            self.reply(200, {'profile': self.server.state.profile, 'status': 'ok'})
            return
        if not self.allowed():
            return
        session = self.session()
        if session is None:
            self.reply(404)
            return
        self.start_stream()
        try:
            while not session['closed'].is_set():
                try:
                    self.event(session['events'].get(timeout=5))
                except queue.Empty:
                    self.wfile.write(b': keepalive\n\n')
                    self.wfile.flush()
        except (BrokenPipeError, ConnectionResetError):
            pass

    def do_DELETE(self):
        if not self.allowed():
            return
        with self.server.state.lock:
            session = self.server.state.sessions.pop(self.headers.get('Mcp-Session-Id'), None)
        if session is None:
            self.reply(404)
        else:
            session['closed'].set()
            self.reply(204)

    def do_POST(self):
        if not self.allowed():
            return
        try:
            size = int(self.headers.get('Content-Length', '0'))
            if size <= 0 or size > 1048576:
                raise ValueError('invalid size')
            msg = json.loads(self.rfile.read(size))
            if not isinstance(msg, dict) or msg.get('jsonrpc') != '2.0':
                raise ValueError('invalid envelope')
            if 'id' in msg and (type(msg['id']) not in (str, int)):
                raise ValueError('invalid ID')
        except (ValueError, TypeError):
            self.reply(400)
            return
        state, method, params = self.server.state, msg.get('method'), msg.get('params', {})
        if not isinstance(params, dict):
            self.reply(400)
            return
        if method == 'initialize':
            if 'id' not in msg or self.headers.get('Mcp-Session-Id'):
                self.reply(400)
                return
            sid = str(uuid.uuid4())
            with state.lock:
                expired = [key for key, value in state.sessions.items() if time.monotonic() - value['seen'] > 1800]
                for key in expired:
                    state.sessions.pop(key)['closed'].set()
                if len(state.sessions) >= 256:
                    self.reply(429)
                    return
                state.sessions[sid] = {'events': queue.Queue(64), 'subscriptions': set(), 'closed': threading.Event(),
                                       'seen': time.monotonic(), 'capabilities': params.get('capabilities', {}), 'pending': {}, 'running': {}, 'initialized': False, 'logLevel': 'warning'}
            self.result(msg, {'protocolVersion': VERSION, 'serverInfo': {'name': 'demo-' + state.profile, 'version': '1.0.0'},
                              'capabilities': {'tools': {}, 'resources': {'subscribe': True}, 'prompts': {}, 'completions': {}, 'logging': {}}}, {'Mcp-Session-Id': sid})
            return
        session = self.session()
        if session is None:
            self.reply(404)
            return
        if method == 'notifications/cancelled':
            running = session['running'].get(str(params.get('requestId')))
            if running:
                running.set()
            self.reply(202)
            return
        if method == 'notifications/initialized':
            session['initialized'] = True
            self.reply(202)
            return
        if method is None and 'id' in msg:
            pending = session['pending'].get(str(msg['id']))
            if pending:
                pending['response'] = msg
                pending['ready'].set()
            self.reply(202 if pending else 400)
            return
        if 'id' not in msg:
            self.reply(202)
            return
        if not session['initialized'] and method != 'ping':
            self.error(msg, -32600, 'initialize first')
            return
        try:
            result = self.dispatch(msg, session)
            if result is not None:
                self.result(msg, result)
        except (ValueError, TypeError, KeyError) as err:
            self.error(msg, -32602, str(err))

    def result(self, msg, value, headers=None):
        self.reply(200, {'jsonrpc': '2.0', 'id': msg['id'], 'result': value}, headers)

    def error(self, msg, code, message):
        self.reply(200, {'jsonrpc': '2.0', 'id': msg['id'], 'error': {'code': code, 'message': message}})

    def dispatch(self, msg, session):
        method, p, state = msg['method'], msg.get('params', {}), self.server.state
        profile = state.profile
        if method == 'ping':
            return {}
        if method == 'logging/setLevel':
            if p.get('level') not in ('debug', 'info', 'notice', 'warning', 'error', 'critical', 'alert', 'emergency'):
                raise ValueError('unknown logging level')
            session['logLevel'] = p['level']
            return {}
        if method == 'tools/list':
            return {'tools': state.tools()}
        if method == 'resources/list':
            return {'resources': [{'uri': 'demo://guide', 'name': profile + ' guide', 'description': 'Synthetic guide; URI intentionally collides across upstreams', 'mimeType': 'text/plain'},
                                  {'uri': 'demo://status', 'name': profile + ' status', 'mimeType': 'application/json'}]}
        if method == 'resources/templates/list':
            return {'resourceTemplates': [{'uriTemplate': 'demo://records/{id}', 'name': profile + ' records', 'mimeType': 'text/plain'}]}
        if method == 'resources/read':
            uri = p['uri']
            if uri not in ('demo://guide', 'demo://status') and not uri.startswith('demo://records/'):
                raise ValueError('unknown resource')
            text = json.dumps({'profile': profile, 'revision': state.revision, 'contact': 'demo.person@example.test', 'uri': uri})
            return {'contents': [{'uri': uri, 'mimeType': 'text/plain', 'text': text}]}
        if method in ('resources/subscribe', 'resources/unsubscribe'):
            if p['uri'] not in ('demo://guide', 'demo://status'):
                raise ValueError('unknown subscribable resource')
            with state.lock:
                if method == 'resources/subscribe':
                    session['subscriptions'].add(p['uri'])
                else:
                    session['subscriptions'].discard(p['uri'])
            return {}
        if method == 'prompts/list':
            return {'prompts': [{'name': 'review', 'description': 'Review a ' + profile + ' scenario', 'arguments': [{'name': 'subject', 'required': True}]}]}
        if method == 'prompts/get':
            if p['name'] != 'review' or not isinstance(p.get('arguments', {}).get('subject'), str):
                raise ValueError('review requires a subject string')
            return {'description': profile + ' review', 'messages': [{'role': 'user', 'content': {'type': 'text', 'text': f"Review {p['arguments']['subject']} using the synthetic {profile} guide. Contact demo.person@example.test."}}]}
        if method == 'completion/complete':
            ref = p.get('ref', {})
            if not ((ref.get('type') == 'ref/prompt' and ref.get('name') == 'review') or
                    (ref.get('type') == 'ref/resource' and ref.get('uri') == 'demo://records/{id}')):
                raise ValueError('unknown completion reference')
            values = [x for x in ['billing', 'onboarding', 'security'] if x.startswith(p.get('argument', {}).get('value', ''))]
            return {'completion': {'values': values, 'total': len(values), 'hasMore': False}}
        if method == 'tools/call':
            name, args = p['name'], p.get('arguments', {})
            if name == 'delay':
                return self.delay(msg, args, session)
            if name == 'ask_host':
                return self.ask_host(msg, session, args)
            if name == 'publish_update':
                value = {'revision': state.publish()}
            elif name == 'fail':
                return {'isError': True, 'content': [{'type': 'text', 'text': 'Synthetic upstream error for manual testing'}]}
            elif name == 'calculate' and profile == 'calculator':
                a, b = args.get('a', 0), args.get('b', 0)
                if type(a) not in (int, float) or type(b) not in (int, float):
                    raise ValueError('a and b must be numbers')
                operation = args.get('operation', 'add')
                if operation not in ('add', 'subtract', 'multiply'):
                    raise ValueError('unknown operation')
                value = {'answer': {'add': a + b, 'subtract': a - b, 'multiply': a * b}[operation]}
            elif name == 'search_docs' and profile == 'knowledge':
                value = {'query': args.get('query', ''), 'matches': ['Engineering handbook', 'Security checklist'], 'contact': 'demo.person@example.test'}
            elif name == 'get_customer' and profile == 'support':
                value = {'customer_id': args.get('customer_id', 'C001'), 'name': 'Demo Person', 'email': 'demo.person@example.test', 'ssn': '123-45-6789', 'phone': '+1 415 555 0123'}
            else:
                raise ValueError('unknown tool')
            return {'content': [{'type': 'text', 'text': json.dumps(value)}], 'structuredContent': value, 'isError': False}
        self.error(msg, -32601, 'method not supported by demo')
        return None

    def start_stream(self):
        self.send_response(200)
        self.send_header('Content-Type', 'text/event-stream')
        self.send_header('Cache-Control', 'no-store')
        self.send_header('Connection', 'close')
        self.end_headers()
        self.close_connection = True
        self.wfile.write(b': connected\n\n')
        self.wfile.flush()

    def event(self, msg):
        self.wfile.write(('data: ' + json.dumps(msg) + '\n\n').encode())
        self.wfile.flush()

    def delay(self, msg, args, session):
        seconds = args.get('seconds', 3)
        if type(seconds) is not int or not 1 <= seconds <= 20:
            raise ValueError('seconds must be an integer between 1 and 20')
        token = msg.get('params', {}).get('_meta', {}).get('progressToken')
        cancelled = threading.Event()
        session['running'][str(msg['id'])] = cancelled
        self.start_stream()
        try:
            if session['logLevel'] in ('debug', 'info', 'notice', 'warning'):
                self.event({'jsonrpc': '2.0', 'method': 'notifications/message', 'params': {'level': 'warning', 'logger': 'synthetic-demo', 'data': 'Demo diagnostic: demo.person@example.test'}})
            for n in range(seconds):
                if cancelled.wait(1):
                    self.event({'jsonrpc': '2.0', 'id': msg['id'], 'error': {'code': -32800, 'message': 'Demo operation cancelled'}})
                    return None
                if token is not None:
                    self.event({'jsonrpc': '2.0', 'method': 'notifications/progress', 'params': {'progressToken': token, 'progress': n + 1, 'total': seconds}})
            self.event({'jsonrpc': '2.0', 'id': msg['id'], 'result': {'content': [{'type': 'text', 'text': 'Delay completed'}], 'isError': False}})
        except (BrokenPipeError, ConnectionResetError):
            pass
        finally:
            session['running'].pop(str(msg['id']), None)
        return None

    def ask_host(self, msg, session, args):
        kind = args.get('kind', 'roots')
        if kind not in ('sampling', 'roots', 'elicitation') or kind not in session['capabilities']:
            raise ValueError('host capability not negotiated')
        requests = {
            'roots': ('roots/list', {}),
            'sampling': ('sampling/createMessage', {'messages': [{'role': 'user', 'content': {'type': 'text', 'text': 'Summarize this synthetic demo in one sentence'}}], 'maxTokens': 64, 'includeContext': 'none'}),
            'elicitation': ('elicitation/create', {'message': 'Choose a demo topic (no personal data)', 'requestedSchema': {'type': 'object', 'properties': {'topic': {'type': 'string', 'enum': ['billing', 'security']}}, 'required': ['topic']}}),
        }
        request_id = str(uuid.uuid4())
        pending = {'ready': threading.Event()}
        session['pending'][request_id] = pending
        method, params = requests[kind]
        self.start_stream()
        try:
            self.event({'jsonrpc': '2.0', 'id': request_id, 'method': method, 'params': params})
            if pending['ready'].wait(30):
                text = json.dumps(pending['response'].get('result', pending['response'].get('error')))
            else:
                text = 'Host response timed out'
            self.event({'jsonrpc': '2.0', 'id': msg['id'], 'result': {'content': [{'type': 'text', 'text': text}]}})
        except (BrokenPipeError, ConnectionResetError):
            pass
        finally:
            session['pending'].pop(request_id, None)
        return None


if __name__ == '__main__':
    profile = os.environ.get('DEMO_PROFILE', 'calculator')
    server = Server(('0.0.0.0', 9100), State(profile), os.environ.get('DEMO_TOKEN', ''))
    print('Synthetic MCP demo ready: ' + profile, flush=True)
    server.serve_forever()
