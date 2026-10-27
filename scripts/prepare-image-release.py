"""Assemble validated Docker Hub manifest digests into a Helm values overlay."""
import argparse
import json
from pathlib import Path
import re

SERVICES = {
    'web', 'control-plane', 'data-plane', 'omni-mcp', 'migrate', 'audit-exporter',
    'audit-exporter-kafka', 'audit-exporter-rabbitmq', 'tenant-router', 'rotate-secrets',
}


def assemble(directory, output, version):
    if not re.fullmatch(r'v(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?:-[0-9A-Za-z]+(?:[.-][0-9A-Za-z]+)*)?', version) or len(version) > 128:
        raise ValueError('invalid release tag')
    records = {}
    for file in sorted(Path(directory).glob('*.json')):
        record = json.loads(file.read_text())
        service = record['service']
        if service not in SERVICES or service in records:
            raise ValueError('unexpected or duplicate image service')
        if record['version'] != version:
            raise ValueError('mixed image versions')
        if record['image'] != f'barishoku/vmcp-{service}':
            raise ValueError('unexpected image repository')
        if not re.fullmatch(r'sha256:[0-9a-f]{64}', record['digest']):
            raise ValueError('invalid manifest digest')
        records[service] = record
    if records.keys() != SERVICES:
        raise ValueError('release does not contain every required image')
    references = {name: f"{record['image']}@{record['digest']}" for name, record in records.items()}
    # JSON-quoted scalar values are valid YAML without a third-party YAML dependency.
    lines = [f'# Docker Hub images for {version}; layer over your production configuration.']
    for key, name in [('web', 'web'), ('controlPlane', 'control-plane'), ('dataPlane', 'data-plane'), ('omni', 'omni-mcp'), ('migration', 'migrate'), ('tenantRouter', 'tenant-router')]:
        lines.extend([f'{key}:', f'  image: {json.dumps(references[name])}'])
    lines.append('exporters:')
    for key, name in [('http', 'audit-exporter'), ('kafka', 'audit-exporter-kafka'), ('rabbitmq', 'audit-exporter-rabbitmq')]:
        lines.extend([f'  {key}:', f'    image: {json.dumps(references[name])}'])
    output = Path(output)
    output.mkdir(parents=True, exist_ok=True)
    (output / 'helm-images.yaml').write_text('\n'.join(lines) + '\n')
    (output / 'images.json').write_text(json.dumps({'version': version, 'images': list(records.values())}, indent=2) + '\n')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('directory', type=Path)
    parser.add_argument('output', type=Path)
    parser.add_argument('--version', required=True)
    args = parser.parse_args()
    assemble(args.directory, args.output, args.version)
