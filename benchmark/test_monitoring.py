"""Opt-in real-stack regression. Creates unique synthetic objects and cleans up.
Run: python3 benchmark/test_monitoring.py (after benchmark compose is started).
"""
import base64
import json
import time
import urllib.parse
import urllib.request
import uuid
from client import Client
from smoke import api


def get(url, authenticated=False):
    headers = {}
    if authenticated:
        token = base64.b64encode(b'admin:vmcp-demo-grafana').decode()
        headers['Authorization'] = 'Basic ' + token
    with urllib.request.urlopen(urllib.request.Request(url, headers=headers), timeout=10) as response:
        return json.load(response)


def query(expression):
    return get('http://localhost:9090/api/v1/query?' + urllib.parse.urlencode({'query': expression}))['data']['result']


def eventually(check, timeout=45):
    deadline = time.monotonic() + timeout
    while True:
        try:
            return check()
        except (AssertionError, OSError, RuntimeError):
            if time.monotonic() >= deadline:
                raise
            time.sleep(1)


def main():
    def targets_ready():
        targets = get('http://localhost:9090/api/v1/targets')['data']['activeTargets']
        assert len(targets) == 3, targets
        assert all(t['health'] == 'up' for t in targets), targets
        assert {t['labels']['instance'] for t in targets} == {'control-plane:8081', 'data-plane-1:8080', 'data-plane-2:8080'}, targets
    eventually(targets_ready)
    rules = get('http://localhost:9090/api/v1/rules')['data']['groups']
    names = {r['name'] for group in rules for r in group['rules']}
    assert {'VMCPAuthorizationStale', 'VMCPAuditIncomplete', 'VMCPAuditLoss', 'VMCPPoolSaturation', 'VMCPRedisDegraded'} <= names, names
    datasource = eventually(lambda: get('http://localhost:3301/api/datasources/uid/vmcp-prometheus', True))
    assert datasource['url'] == 'http://prometheus:9090', datasource
    for uid in ['vmcp-operations', 'vmcp-mcp']:
        dashboard = eventually(lambda: get(f'http://localhost:3301/api/dashboards/uid/{uid}', True))
        assert dashboard['meta']['provisioned'], dashboard
        assert dashboard['dashboard']['panels'], dashboard

    slug = 'monitor-' + uuid.uuid4().hex[:8]
    upstream_created = server_created = key_created = False
    clients = []
    try:
        api('POST', '/upstreams', {'slug': slug, 'name': slug, 'url': 'http://mcp-calculator:9100/mcp', 'transport': 'streamable_http', 'headers': {'Authorization': 'Bearer calculator-secret'}})
        upstream_created = True
        api('POST', f'/upstreams/{slug}/refresh-tools')
        api('POST', '/virtual-servers', {'slug': slug, 'name': slug, 'authRequired': True, 'tools': [{'upstream': slug, 'tool': 'calculate', 'alias': 'sum'}], 'catalog': [{'upstream': slug, 'kind': 'resource', 'name': 'demo://guide', 'alias': 'guide'}]})
        server_created = True
        key = api('POST', '/agent-keys', {'slug': slug, 'virtualServers': [slug]})['key']
        key_created = True
        for port in [8802, 8803]:
            client = Client(f'http://localhost:{port}/mcp/v1/virtual/{slug}', key)
            clients.append(client)
            eventually(client.initialize)
            result = client.call('tools/call', {'name': 'sum', 'arguments': {'a': 4, 'b': 7}})
            assert result['structuredContent']['answer'] == 11, result
            uri = client.call('resources/list')['resources'][0]['uri']
            assert client.call('resources/read', {'uri': uri})['contents']

        def observed():
            for family, labels in [
                ('virtual_tool_calls', f'virtual_server="{slug}",tool="sum",outcome="ok"'),
                ('upstream_requests', f'upstream="{slug}",method="tools/call",tool="calculate",outcome="ok"'),
                ('upstream_requests', f'upstream="{slug}",method="resources/read",tool="",outcome="ok"'),
            ]:
                rows = query(f'vmcp_{family}_total{{{labels}}}')
                assert len(rows) == 2 and all(float(row['value'][1]) == 1 for row in rows), rows
            rows = query(f'vmcp_virtual_requests_total{{virtual_server="{slug}",outcome="ok"}}')
            assert len(rows) == 2, rows
        eventually(observed)
        print('PASS: three scrape targets, provisioned dashboards/source/rules, and per-node virtual/tool/upstream/catalog metrics')
    finally:
        for client in clients:
            if client.sid:
                try:
                    with client.request('DELETE'):
                        pass
                except OSError:
                    pass
        if key_created: api('DELETE', '/agent-keys/' + slug)
        if server_created: api('DELETE', '/virtual-servers/' + slug)
        if upstream_created: api('DELETE', '/upstreams/' + slug)


if __name__ == '__main__':
    main()
