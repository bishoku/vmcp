import http.client
import json
import threading
import unittest

from server import Server, State


class DemoTest(unittest.TestCase):
    def setUp(self):
        self.server = Server(('127.0.0.1', 0), State('calculator'), 'demo')
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        self.port = self.server.server_port
        self.sid = None

    def tearDown(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join()

    def request(self, method='POST', msg=None, sid=None, token='demo'):
        c = http.client.HTTPConnection('127.0.0.1', self.port, timeout=5)
        headers = {'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json', 'Accept': 'application/json, text/event-stream', 'MCP-Protocol-Version': '2025-06-18'}
        if sid:
            headers['Mcp-Session-Id'] = sid
        c.request(method, '/mcp', json.dumps(msg) if msg else None, headers)
        return c, c.getresponse()

    def rpc(self, method, params=None):
        c, r = self.request(msg={'jsonrpc': '2.0', 'id': 2, 'method': method, 'params': params or {}}, sid=self.sid)
        body = json.loads(r.read())
        c.close()
        return body

    def initialize(self, capabilities=None):
        c, r = self.request(msg={'jsonrpc': '2.0', 'id': 1, 'method': 'initialize', 'params': {'protocolVersion': '2025-06-18', 'clientInfo': {'name': 'test', 'version': '1'}, 'capabilities': capabilities or {}}})
        self.sid = r.getheader('Mcp-Session-Id')
        self.assertEqual(r.status, 200)
        r.read(); c.close()
        c, r = self.request(msg={'jsonrpc': '2.0', 'method': 'notifications/initialized'}, sid=self.sid)
        self.assertEqual(r.status, 202)
        r.read(); c.close()

    def test_catalog_profiles_and_namespace_collisions(self):
        for profile, expected in [('calculator', 'calculate'), ('knowledge', 'search_docs'), ('support', 'get_customer')]:
            self.server.state.profile = profile
            self.initialize()
            self.assertEqual(self.rpc('tools/list')['result']['tools'][0]['name'], expected)
            self.assertEqual(self.rpc('resources/list')['result']['resources'][0]['uri'], 'demo://guide')
            self.assertEqual(self.rpc('prompts/list')['result']['prompts'][0]['name'], 'review')
            self.assertIn(profile, self.rpc('resources/read', {'uri': 'demo://guide'})['result']['contents'][0]['text'])
            self.assertIn(profile, self.rpc('prompts/get', {'name': 'review', 'arguments': {'subject': 'security'}})['result']['messages'][0]['content']['text'])
        self.assertIn('error', self.rpc('prompts/get', {'name': 'review'}))
        self.assertIn('error', self.rpc('resources/read', {'uri': 'file:///etc/passwd'}))

    def test_auth_session_validation_and_tool_results(self):
        c, r = self.request(msg={'jsonrpc': '2.0', 'id': None, 'method': 'initialize'})
        self.assertEqual(r.status, 400); r.read(); c.close()
        c, r = self.request(msg={'jsonrpc': '2.0', 'id': 1, 'method': 'ping'}, token='wrong')
        self.assertEqual(r.status, 401); r.read(); c.close()
        self.initialize()
        self.assertEqual(self.rpc('tools/call', {'name': 'calculate', 'arguments': {'a': 2, 'b': 3}})['result']['structuredContent']['answer'], 5)
        self.assertTrue(self.rpc('tools/call', {'name': 'fail'})['result']['isError'])
        self.assertIn('error', self.rpc('tools/call', {'name': 'delay', 'arguments': {'seconds': 500}}))
        c, r = self.request(method='DELETE', sid=self.sid)
        self.assertEqual(r.status, 204); r.read(); c.close()
        c, r = self.request(msg={'jsonrpc': '2.0', 'id': 1, 'method': 'ping'}, sid=self.sid)
        self.assertEqual(r.status, 404); r.read(); c.close()

    def test_subscription_hints_and_completion(self):
        self.initialize()
        self.rpc('resources/subscribe', {'uri': 'demo://status'})
        self.rpc('tools/call', {'name': 'publish_update'})
        event = self.server.state.sessions[self.sid]['events'].get_nowait()
        self.assertEqual(event['params']['uri'], 'demo://status')
        self.rpc('resources/unsubscribe', {'uri': 'demo://status'})
        values = self.rpc('completion/complete', {'ref': {'type': 'ref/prompt', 'name': 'review'}, 'argument': {'name': 'subject', 'value': 'sec'}})['result']['completion']['values']
        self.assertEqual(values, ['security'])


    def test_cancellation_reaches_running_operation(self):
        self.initialize()
        c, r = self.request(msg={'jsonrpc': '2.0', 'id': 8, 'method': 'tools/call', 'params': {'name': 'delay', 'arguments': {'seconds': 20}}}, sid=self.sid)
        other, ack = self.request(msg={'jsonrpc': '2.0', 'method': 'notifications/cancelled', 'params': {'requestId': 8}}, sid=self.sid)
        self.assertEqual(ack.status, 202); ack.read(); other.close()
        self.assertIn('Demo operation cancelled', r.read().decode()); c.close()

    def test_progress_and_host_reply_routing(self):
        self.initialize({'roots': {}})
        c, r = self.request(msg={'jsonrpc': '2.0', 'id': 3, 'method': 'tools/call', 'params': {'name': 'ask_host', 'arguments': {'kind': 'roots'}}}, sid=self.sid)
        self.assertEqual(r.getheader('Content-Type'), 'text/event-stream')
        r.readline(); r.readline()
        request = json.loads(r.readline().decode()[6:]); r.readline()
        other, reply = self.request(msg={'jsonrpc': '2.0', 'id': request['id'], 'result': {'roots': []}}, sid=self.sid)
        self.assertEqual(reply.status, 202); reply.read(); other.close()
        terminal = json.loads(r.readline().decode()[6:])
        self.assertEqual(terminal['id'], 3)
        c.close()
        c, r = self.request(msg={'jsonrpc': '2.0', 'id': 4, 'method': 'tools/call', 'params': {'name': 'delay', 'arguments': {'seconds': 1}, '_meta': {'progressToken': 'demo-progress'}}}, sid=self.sid)
        payload = r.read().decode(); c.close()
        self.assertIn('demo-progress', payload)
        self.assertIn('Delay completed', payload)


if __name__ == '__main__':
    unittest.main()
