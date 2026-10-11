"""Small manual MCP client; no third-party packages. Not a conformance SDK."""
import argparse
import json
import time
import urllib.request

VERSION = '2025-06-18'


class Client:
    def __init__(self, url, token='', host_roots=False, verbose=False):
        self.url, self.token, self.sid = url, token, None
        self.sequence, self.host_roots, self.verbose = 0, host_roots, verbose

    def request(self, method, message=None, url=None):
        headers = {'Accept': 'application/json, text/event-stream', 'Content-Type': 'application/json', 'MCP-Protocol-Version': VERSION}
        if self.token:
            headers['Authorization'] = 'Bearer ' + self.token
        if self.sid:
            headers['Mcp-Session-Id'] = self.sid
        data = json.dumps(message).encode() if message is not None else None
        return urllib.request.urlopen(urllib.request.Request(url or self.url, data, headers, method=method), timeout=80)

    def initialize(self):
        caps = {'roots': {'listChanged': True}} if self.host_roots else {}
        result = self.call('initialize', {'protocolVersion': VERSION, 'capabilities': caps, 'clientInfo': {'name': 'vmcp-demo-client', 'version': '1'}})
        with self.request('POST', {'jsonrpc': '2.0', 'method': 'notifications/initialized'}):
            pass
        return result

    def call(self, method, params=None, url=None):
        self.sequence += 1
        request_id = self.sequence
        with self.request('POST', {'jsonrpc': '2.0', 'id': request_id, 'method': method, 'params': params or {}}, url) as response:
            self.sid = response.headers.get('Mcp-Session-Id', self.sid)
            if 'text/event-stream' in response.headers.get('Content-Type', ''):
                terminal = None
                for line in response:
                    if not line.startswith(b'data:'):
                        continue
                    message = json.loads(line[5:])
                    if 'method' in message and 'id' in message:
                        # Opt-in roots demonstration returns no filesystem roots.
                        reply = {'jsonrpc': '2.0', 'id': message['id']}
                        if message['method'] == 'roots/list' and self.host_roots:
                            reply['result'] = {'roots': []}
                        else:
                            reply['error'] = {'code': -32601, 'message': 'Demo client declines unsupported host request'}
                        with self.request('POST', reply, url):
                            pass
                    elif message.get('id') == request_id:
                        terminal = message
                        break
                    elif self.verbose:
                        print(json.dumps(message), flush=True)
                if terminal is None:
                    raise RuntimeError('stream ended without a response')
                result = terminal
            else:
                result = json.load(response)
        if 'error' in result:
            raise RuntimeError(json.dumps(result['error']))
        return result['result']

    def watch(self, uri, seconds):
        self.call('resources/subscribe', {'uri': uri})
        deadline = time.monotonic() + seconds
        with self.request('GET') as response:
            for line in response:
                if line.startswith(b'data:'):
                    print(line[5:].decode().strip(), flush=True)
                if time.monotonic() >= deadline:
                    break
        self.call('resources/unsubscribe', {'uri': uri})

    def close(self):
        if self.sid:
            try:
                with self.request('DELETE'):
                    pass
            finally:
                self.sid = None


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--url', required=True)
    parser.add_argument('--token', default='')
    parser.add_argument('--method', default='tools/list')
    parser.add_argument('--params', default='{}', help='JSON method parameters')
    parser.add_argument('--watch', help='Subscribe to an exposed resource URI and print GET SSE events')
    parser.add_argument('--seconds', type=int, default=40)
    parser.add_argument('--host-roots', action='store_true', help='Offer roots capability; return an empty list, never host paths')
    args = parser.parse_args()
    client = Client(args.url, args.token, args.host_roots, verbose=True)
    try:
        client.initialize()
        if args.watch:
            client.watch(args.watch, args.seconds)
        else:
            print(json.dumps(client.call(args.method, json.loads(args.params)), indent=2))
    finally:
        client.close()


if __name__ == '__main__':
    main()
