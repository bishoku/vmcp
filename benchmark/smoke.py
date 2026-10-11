"""Opt-in smoke test: creates uniquely named objects and deletes them afterward."""
import json
import time
import queue
import threading
import urllib.parse
import urllib.error
import urllib.request
import uuid
from client import Client

CP = 'http://localhost:8801/api/v1'


def api(method, path, body=None):
    request = urllib.request.Request(CP + path, json.dumps(body).encode() if body is not None else None,
                                     {'Authorization': 'Bearer demo-admin-token', 'Content-Type': 'application/json'}, method=method)
    with urllib.request.urlopen(request, timeout=60) as response:
        raw = response.read()
        return json.loads(raw) if raw else None


def main():
    prefix = 'smoke-' + uuid.uuid4().hex[:8]
    upstreams, vs_created, client = [], False, None
    key_created, binding_created = False, False
    try:
        selections, catalog = [], []
        for profile, tool in [('calculator', 'calculate'), ('knowledge', 'search_docs'), ('support', 'get_customer')]:
            slug = prefix + '-' + profile
            api('POST', '/upstreams', {'slug': slug, 'name': slug, 'url': f'http://mcp-{profile}:9100/mcp', 'transport': 'streamable_http', 'headers': {'Authorization': f'Bearer {profile}-secret'}})
            upstreams.append(slug)
            api('POST', f'/upstreams/{slug}/refresh-tools')
            found = api('GET', f'/upstreams/{slug}/catalog')
            assert len(found['resources']) == 2 and len(found['prompts']) == 1, found
            selections.append({'upstream': slug, 'tool': tool, 'alias': tool})
            if profile == 'calculator':
                selections.extend([{'upstream': slug, 'tool': x, 'alias': x} for x in ['delay', 'publish_update', 'ask_host']])
            catalog.extend([{'upstream': slug, 'kind': 'resource', 'name': 'demo://guide', 'alias': profile + '_guide'},
                            {'upstream': slug, 'kind': 'resource', 'name': 'demo://status'},
                            {'upstream': slug, 'kind': 'template', 'name': 'demo://records/{id}'},
                            {'upstream': slug, 'kind': 'prompt', 'name': 'review'}])
        api('POST', '/virtual-servers', {'slug': prefix, 'name': prefix, 'authRequired': True, 'tools': selections, 'catalog': catalog, 'clientFeatures': {'roots': True}})
        vs_created = True
        key = api('POST', '/agent-keys', {'slug': prefix, 'virtualServers': [prefix]})['key']
        key_created = True
        api('POST', '/identity-bindings', {'slug': prefix, 'conditions': [{'claim': 'azp', 'op': 'equals', 'value': 'billing-bot'}], 'virtualServers': [prefix]})
        binding_created = True
        # Wait for both replicas to observe the newly created server, not an arbitrary fixed sleep.
        for port in (8802, 8803):
            deadline = time.monotonic() + 30
            while True:
                probe = Client(f'http://localhost:{port}/mcp/v1/virtual/{prefix}', key)
                try:
                    probe.initialize(); probe.close(); break
                except (urllib.error.HTTPError, RuntimeError):
                    if time.monotonic() > deadline:
                        raise
                    time.sleep(.25)
        client = Client(f'http://localhost:8800/mcp/v1/virtual/{prefix}', key, host_roots=True)
        client.initialize()
        result = client.call('tools/call', {'name': 'calculate', 'arguments': {'a': 4, 'b': 7}})
        assert result['structuredContent']['answer'] == 11, result
        # Explicitly send the same persisted session through each node.
        for port in (8802, 8803):
            assert len(client.call('tools/list', url=f'http://localhost:{port}/mcp/v1/virtual/{prefix}')['tools']) == 6
        resources = client.call('resources/list')['resources']
        assert len(resources) == 6 and len({r['uri'] for r in resources}) == 6
        assert resources[0]['uri'] == 'vmcp://' + prefix + '-calculator/alias/calculator_guide'
        assert 'calculator' in client.call('resources/read', {'uri': resources[0]['uri']})['contents'][0]['text']
        templates = client.call('resources/templates/list')['resourceTemplates']
        assert len(templates) == 3
        expanded = templates[0]['uriTemplate'].replace('{id}', 'C001')
        assert 'C001' in client.call('resources/read', {'uri': expanded})['contents'][0]['text']
        prompts = client.call('prompts/list')['prompts']
        assert len(prompts) == 3
        assert client.call('prompts/get', {'name': prompts[0]['name'], 'arguments': {'subject': 'security'}})['messages']
        assert client.call('completion/complete', {'ref': {'type': 'ref/prompt', 'name': prompts[0]['name']}, 'argument': {'name': 'subject', 'value': 'sec'}})['completion']['values'] == ['security']
        assert 'Delay completed' in json.dumps(client.call('tools/call', {'name': 'delay', 'arguments': {'seconds': 1}, '_meta': {'progressToken': 'smoke-progress'}}))
        assert 'roots' in json.dumps(client.call('tools/call', {'name': 'ask_host', 'arguments': {'kind': 'roots'}}))
        status_uri = next(r['uri'] for r in resources if r['name'] == 'calculator status')
        client.call('resources/subscribe', {'uri': status_uri})
        received = queue.Queue()
        with client.request('GET') as stream:
            def read_updates():
                for line in stream:
                    if line.startswith(b'data:'):
                        event = json.loads(line[5:])
                        if event.get('method') == 'notifications/resources/updated':
                            received.put(event); return
            thread = threading.Thread(target=read_updates, daemon=True)
            thread.start()
            for attempt in range(15):
                client.call('tools/call', {'name': 'publish_update'})
                try:
                    event = received.get(timeout=1)
                    assert event['params']['uri'] == status_uri, event
                    break
                except queue.Empty:
                    if attempt == 14:
                        raise AssertionError('subscription notification not received')
            thread.join(timeout=2)
        client.call('resources/unsubscribe', {'uri': status_uri})
        encoded = urllib.parse.urlencode({'grant_type': 'client_credentials', 'client_id': 'billing-bot', 'client_secret': 'billing-secret'}).encode()
        with urllib.request.urlopen('http://localhost:9180/realms/vmcp/protocol/openid-connect/token', encoded) as response:
            jwt = json.load(response)['access_token']
        oauth = Client(client.url, jwt)
        oauth.initialize()
        assert oauth.call('tools/list')['tools']
        oauth.close()
        print('PASS: three upstreams, curated tools/catalogs, namespace collisions, prompts/completion, SSE progress/subscriptions, host roots relay, agent-key/JWT auth and cross-node sessions')
    finally:
        if client:
            client.close()
        if binding_created:
            api('DELETE', '/identity-bindings/' + prefix)
        if key_created:
            api('DELETE', '/agent-keys/' + prefix)
        if vs_created:
            api('DELETE', '/virtual-servers/' + prefix)
        for slug in reversed(upstreams):
            api('DELETE', '/upstreams/' + slug + '?force=true')


if __name__ == '__main__':
    main()
