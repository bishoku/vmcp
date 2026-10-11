"""Isolated, disposable full demo stack. Requires Docker and installed Chromium."""
import json
import os
from pathlib import Path
import socket
import subprocess
import tempfile
import uuid

ROOT = Path(__file__).resolve().parents[2]
project = 'vmcp-ui-' + uuid.uuid4().hex[:10]

def run(args, **kwargs):
    return subprocess.run(args, cwd=kwargs.pop('cwd', ROOT), check=True, **kwargs)

def port():
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        return sock.getsockname()[1]

with tempfile.TemporaryDirectory(prefix='vmcp-ui-') as directory:
    config = json.loads(run(['docker', 'compose', '-f', 'benchmark/compose.yaml', 'config', '--format', 'json'], capture_output=True, text=True).stdout)
    config['name'] = project
    # Dedicated subnet is configurable to avoid a local Docker network collision.
    subnet = os.environ.get('VMCP_UI_TEST_SUBNET', '172.30.73.0/24')
    ports = {int(p['published']): port() for service in config['services'].values() for p in service.get('ports', [])}
    def rewrite(value):
        if isinstance(value, dict): return {k: rewrite(v) for k, v in value.items()}
        if isinstance(value, list): return [rewrite(v) for v in value]
        if isinstance(value, str):
            value = value.replace('172.30.42.0/24', subnet)
            for old, new in ports.items(): value = value.replace(f'localhost:{old}', f'localhost:{new}')
        return value
    original_subnet = config['networks']['demo']['ipam']['config'][0]['subnet']
    config = rewrite(config)
    config['networks']['demo']['ipam']['config'][0]['subnet'] = subnet
    for service in config['services'].values():
        for key, value in service.get('environment', {}).items():
            if value == original_subnet: service['environment'][key] = subnet
    for service in config['services'].values():
        for published in service.get('ports', []): published['published'] = str(ports[int(published['published'])])
        if 'build' in service: service['image'] = project + '/' + next(k for k,v in config['services'].items() if v is service) + ':test'
    for network in config['networks'].values(): network.pop('name', None)
    for volume in config['volumes'].values(): volume.pop('name', None)
    gateway = Path(directory) / 'gateway.conf'
    gateway.write_text(rewrite((ROOT / 'benchmark/gateway.conf').read_text()))
    for volume in config['services']['gateway']['volumes']:
        if volume['target'] == '/etc/nginx/conf.d/default.conf': volume['source'] = str(gateway)
    path = Path(directory) / 'compose.json'
    path.write_text(json.dumps(config))
    compose = ['docker', 'compose', '-p', project, '-f', str(path)]
    try:
        run(compose + ['up', '-d', '--build', '--wait', '--wait-timeout', '180'])
        env = {**os.environ, 'VMCP_LIVE_WEB': f'http://localhost:{ports[3300]}', 'VMCP_LIVE_API': f'http://localhost:{ports[8801]}'}
        run(['npx', 'playwright', 'test', '--config', 'playwright.live.config.ts'], cwd=ROOT / 'web', env=env)
    finally:
        run(compose + ['down', '-v', '--remove-orphans', '--timeout', '5'])
        # Test-specific image tags must not replace the running manual demo images.
        for service in config['services'].values():
            if 'build' in service: subprocess.run(['docker', 'image', 'rm', service['image']], capture_output=True)
