"""Real install/upgrade/restart smoke test in a disposable Kind context only.

Build/load vmcp/{control-plane,data-plane,omni-mcp,migrate,fixture-mcp,web}:helm-test first.
Requires Helm, kubectl, a running Kind cluster, and HELM_BIN/KUBECONFIG as needed.
"""
import base64
import json
import os
from pathlib import Path
import socket
import subprocess
import tempfile
import time
import urllib.request

ROOT = Path(__file__).resolve().parents[2]
HELM = os.environ.get('HELM_BIN', 'helm')
CONTEXT = 'kind-vmcp-helm-test'
NS = 'vmcp-helm-smoke'

def run(args, **kwargs):
    return subprocess.run(args, cwd=ROOT, check=True, **kwargs)

def kube(*args, **kwargs):
    return run(['kubectl', '--context', CONTEXT, '-n', NS, *args], **kwargs)

def resource(kind, name, spec=None):
    d = {'apiVersion': 'v1', 'kind': kind, 'metadata': {'name': name, 'namespace': NS}}
    if spec is not None:
        d['spec'] = spec
    return d

def fixtures():
    namespace = resource('Namespace', NS)
    namespace['metadata'].pop('namespace')
    items = [namespace]
    for name, image, port, extra in [
        ('postgres', 'postgres:17-alpine', 5432, {'env': [{'name': 'POSTGRES_PASSWORD', 'value': 'smoke-only'}, {'name': 'POSTGRES_DB', 'value': 'vmcp'}]}),
        ('redis', 'redis:7-alpine', 6379, {}),
        ('fixture', 'vmcp/fixture-mcp:helm-test', 9100, {'imagePullPolicy': 'Never'}),
    ]:
        container = {'name': name, 'image': image, 'ports': [{'containerPort': port}], **extra}
        items += [resource('Pod', name, {'containers': [container]}), resource('Service', name, {'selector': {'fixture': name}, 'ports': [{'port': port, 'targetPort': port}]})]
        items[-2]['metadata']['labels'] = {'fixture': name}
    return {'apiVersion': 'v1', 'kind': 'List', 'items': items}

def port():
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        return sock.getsockname()[1]

def wait_request(url, **kwargs):
    deadline = time.monotonic() + 90
    while True:
        try:
            with urllib.request.urlopen(urllib.request.Request(url, **kwargs), timeout=5) as response:
                return response.read(), response.headers
        except Exception:
            if time.monotonic() >= deadline:
                raise
            time.sleep(1)

def verify(cp_url, dp_url):
    admin = {'Authorization': 'Bearer smoke-admin-only-for-tests', 'Content-Type': 'application/json'}
    def api(method, path, data=None):
        body = None if data is None else json.dumps(data).encode()
        raw, _ = wait_request(cp_url + '/api/v1/' + path, method=method, headers=admin, data=body)
        return json.loads(raw) if raw else None
    api('POST', 'upstreams', {'slug': 'fixture', 'name': 'Fixture', 'url': 'http://fixture:9100/mcp', 'transport': 'streamable_http'})
    api('POST', 'upstreams/fixture/refresh-tools', {})
    api('POST', 'virtual-servers', {'slug': 'smoke', 'name': 'Smoke', 'authRequired': True, 'tools': [{'upstream': 'fixture', 'tool': 'echo', 'alias': 'echo'}]})
    key = api('POST', 'agent-keys', {'slug': 'smoke', 'virtualServers': ['smoke']})['key']
    endpoint = dp_url + '/mcp/v1/virtual/smoke'
    headers = {'Authorization': 'Bearer ' + key, 'Content-Type': 'application/json', 'Accept': 'application/json, text/event-stream'}
    def rpc(message):
        raw, h = wait_request(endpoint, method='POST', headers=headers, data=json.dumps(message).encode())
        if raw and not raw.lstrip().startswith(b'{'):
            messages = [json.loads(line[5:].strip()) for line in raw.splitlines() if line.startswith(b'data:')]
            raw = json.dumps(next(m for m in reversed(messages) if m.get('id') == message.get('id'))).encode()
        return (json.loads(raw) if raw else None), h
    init, response_headers = rpc({'jsonrpc': '2.0', 'id': 1, 'method': 'initialize', 'params': {'protocolVersion': '2025-06-18', 'capabilities': {}, 'clientInfo': {'name': 'helm-smoke', 'version': '1'}}})
    assert init.get('result'), init
    headers['Mcp-Session-Id'] = response_headers['Mcp-Session-Id']
    headers['MCP-Protocol-Version'] = '2025-06-18'
    rpc({'jsonrpc': '2.0', 'method': 'notifications/initialized'})
    tools, _ = rpc({'jsonrpc': '2.0', 'id': 2, 'method': 'tools/list'})
    assert any(t['name'] == 'echo' for t in tools['result']['tools']), tools
    result, _ = rpc({'jsonrpc': '2.0', 'id': 3, 'method': 'tools/call', 'params': {'name': 'echo', 'arguments': {'message': 'helm-smoke'}}})
    assert 'helm-smoke' in json.dumps(result.get('result')), result
    return headers, endpoint

# Never target the user's current Kubernetes context or change kubeconfig defaults.
run(['kubectl', '--context', CONTEXT, 'get', 'nodes'], stdout=subprocess.DEVNULL)
forwarders = []
installed = False
try:
    kube('apply', '-f', '-', input=json.dumps(fixtures()).encode())
    kube('wait', '--for=condition=Ready', 'pod/postgres', 'pod/redis', 'pod/fixture', '--timeout=180s')
    with tempfile.TemporaryDirectory(prefix='vmcp-helm-smoke-') as directory:
        values = {
            'imagePolicy': {'requireDigest': False},
            'workloadDefaults': {'replicas': 2, 'resources': {'requests': {'cpu': '25m', 'memory': '64Mi'}, 'limits': {'cpu': '1', 'memory': '256Mi'}}},
            'credentials': {'masterKey': {'value': base64.b64encode(bytes(range(32))).decode()}, 'adminToken': {'value': 'smoke-admin-only-for-tests'}},
            'database': {k: {'value': 'postgres://postgres:smoke-only@postgres:5432/vmcp?sslmode=disable'} for k in ['controlPlane', 'dataPlane', 'migration']},
            'rateLimit': {'redisURL': {'value': 'redis://redis:6379/0'}},
            'runtime': {'publicGatewayURL': 'https://mcp.example.test', 'egressAllowCIDRs': ['10.0.0.0/8', '172.16.0.0/12', '192.168.0.0/16']},
            'networkPolicy': {'enabled': False},
        }
        for component, image in [('controlPlane','control-plane'),('dataPlane','data-plane'),('web','web'),('migration','migrate')]:
            values[component] = {'image': 'vmcp/' + image + ':helm-test'}
        file = Path(directory) / 'values.json'
        file.write_text(json.dumps(values))
        common = [HELM, '--kube-context', CONTEXT, '-n', NS]
        run([*common, 'install', 'smoke', 'deploy/helm/vmcp', '-f', str(file), '--wait', '--wait-for-jobs', '--timeout', '10m'])
        installed = True
        web_port, dp_port = port(), port()
        for svc, local in [('smoke-vmcp-web', web_port), ('smoke-vmcp-data-plane', dp_port)]:
            forwarders.append(subprocess.Popen(['kubectl', '--context', CONTEXT, '-n', NS, 'port-forward', 'service/' + svc, f'{local}:8080'], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL))
        cp_url, dp_url = f'http://127.0.0.1:{web_port}', f'http://127.0.0.1:{dp_port}'
        raw, headers = wait_request(cp_url + '/runtime-config.js')
        assert b'https://mcp.example.test' in raw and headers.get('Cache-Control') == 'no-store'
        runtime_headers, direct_endpoint = verify(cp_url, dp_url)
        # Enable Omni only during upgrade: old virtual servers must remain unpublished.
        values['omni'] = {'enabled': True, 'image': 'vmcp/omni-mcp:helm-test', 'secret': {'value': 'isolated-smoke-omni-secret-at-least-32-characters'}, 'refreshInterval': '1s'}
        file.write_text(json.dumps(values))
        run([*common, 'upgrade', 'smoke', 'deploy/helm/vmcp', '-f', str(file), '--wait', '--wait-for-jobs', '--timeout', '10m'])
        # kubectl port-forward pins a pod; enabling the shared secret rolls the
        # existing workloads, so reconnect forwards after the successful upgrade.
        for proc in forwarders:
            proc.terminate()
            proc.wait(timeout=10)
        forwarders.clear()
        for svc, local in [('smoke-vmcp-web', web_port), ('smoke-vmcp-data-plane', dp_port)]:
            forwarders.append(subprocess.Popen(['kubectl', '--context', CONTEXT, '-n', NS, 'port-forward', 'service/' + svc, f'{local}:8080'], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL))
        raw, _ = wait_request(direct_endpoint, method='POST', headers=runtime_headers, data=json.dumps({'jsonrpc':'2.0','id':4,'method':'tools/call','params':{'name':'echo','arguments':{'message':'direct-after-omni-upgrade'}}}).encode())
        assert b'direct-after-omni-upgrade' in raw, raw
        omni_port = port()
        forwarders.append(subprocess.Popen(['kubectl', '--context', CONTEXT, '-n', NS, 'port-forward', 'service/smoke-vmcp-omni-mcp', f'{omni_port}:8080'], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL))
        omni_url = f'http://127.0.0.1:{omni_port}'
        wait_request(omni_url + '/readyz')
        admin = {'Authorization': 'Bearer smoke-admin-only-for-tests', 'Content-Type': 'application/json'}
        raw, _ = wait_request(cp_url + '/api/v1/virtual-servers/smoke', headers=admin)
        assert json.loads(raw)['omniEnabled'] is False, raw
        wait_request(cp_url + '/api/v1/virtual-servers/smoke', method='PATCH', headers=admin, data=json.dumps({'omniEnabled': True}).encode())
        omni_headers = {'Authorization': runtime_headers['Authorization'], 'Content-Type': 'application/json', 'Accept': 'application/json, text/event-stream'}
        def omni_call(name, arguments):
            raw, _ = wait_request(omni_url + '/mcp/v1/omni', method='POST', headers=omni_headers, data=json.dumps({'jsonrpc':'2.0','id':1,'method':'tools/call','params':{'name':name,'arguments':arguments}}).encode())
            result = json.loads(raw)
            assert 'error' not in result, result
            return result['result']
        deadline = time.monotonic() + 30
        while True:
            found = omni_call('discover_tools', {'query':'echo','limit':5})
            payload = json.loads(found['content'][0]['text'])
            if payload['tools']: break
            assert time.monotonic() < deadline, found
            time.sleep(1)
        ref = payload['tools'][0]['tool_ref']
        schema = omni_call('get_tool_schema', {'tool_ref':ref})
        assert 'schema_version' in json.dumps(schema), schema
        result = omni_call('invoke_tool', {'tool_ref':ref, 'arguments':{'message':'omni-helm-smoke'}})
        assert 'omni-helm-smoke' in json.dumps(result), result
        kube('rollout', 'restart', 'deployment/smoke-vmcp-data-plane')
        kube('rollout', 'status', 'deployment/smoke-vmcp-data-plane', '--timeout=180s')
        # Check persistence through the management proxy after the rolling restart.
        raw, _ = wait_request(cp_url + '/api/v1/virtual-servers', headers={'Authorization': 'Bearer smoke-admin-only-for-tests'})
        assert b'"smoke"' in raw
        print('Passed real Helm install, HTTP MCP tool call, upgrade with optional Omni discovery/schema/invocation, runtime web configuration and rolling restart.')
except Exception:
    subprocess.run(['kubectl', '--context', CONTEXT, '-n', NS, 'get', 'pods,jobs', '-o', 'wide'])
    subprocess.run(['kubectl', '--context', CONTEXT, '-n', NS, 'get', 'events', '--sort-by=.lastTimestamp'])
    raise
finally:
    for proc in forwarders:
        proc.terminate()
        proc.wait(timeout=10)
    if installed:
        run([HELM, '--kube-context', CONTEXT, '-n', NS, 'uninstall', 'smoke', '--wait'])
    run(['kubectl', '--context', CONTEXT, 'delete', 'namespace', NS, '--wait=false'])
