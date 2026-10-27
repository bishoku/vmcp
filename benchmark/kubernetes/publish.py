#!/usr/bin/env python3
"""Archive full load-test evidence locally and export a small reviewable result set."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import shutil
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parents[2]
PUBLIC_FILES = {'report.md', 'comparison.md', 'throughput.svg', 'memory.svg',
                'summary.json', 'reproduction.json', 'diagnostics.json'}
DIAGNOSTICS = {
    'postgres-profile.json', 'postgres-effective-settings.json', 'postgres-cgroup-limits.json',
    'polling-wait-summary.json', 'poll-probe.json', 'postgres-runtime-resources.json',
    'postgres-hot-query-intervals.json', 'postgres-index-plans.txt',
    'postgres-delivery-custom-generic-plans.txt', 'postgres-demo-settings.txt',
    'postgres-benchmark-settings.txt', 'postgres-cleanup-selector-comparison.txt',
    'postgres-load-summary.json', 'postgres-hot-path-counters.json',
    'postgres-late-load-plans.txt', 'postgres-locks-and-plans-under-load.txt',
    'postgres-plan-mid-soak.txt', 'postgres-cleanup-plan-mid-soak.txt',
    'cgroup-deltas-after-spike.json',
}


def read_json(path, default=None):
    return json.loads(path.read_text()) if path.exists() else default


def digest_file(path):
    with path.open('rb') as source:
        return hashlib.file_digest(source, 'sha256').hexdigest()


def rewrite_links(text, directory, archive_link='../README.md#raw-artifacts'):
    def replace(match):
        label, target = match.groups()
        if '://' in target or target.startswith(('#', 'mailto:')):
            return match.group(0)
        path = target.split('#', 1)[0]
        name = Path(path).name
        if name in PUBLIC_FILES or (directory / path).is_dir():
            return match.group(0)
        if name in DIAGNOSTICS:
            return f'[{label}](diagnostics.json)'
        if name == 'report.json':
            return '[Özet sonuçlar](summary.json); ham örnekler [yerel arşivde](' + archive_link + ')'
        return f'[{label} — yerel arşiv]({archive_link})'
    return re.sub(r'\[([^\]]+)\]\(([^)]+)\)', replace, text)


def archive_run(source, archives):
    files = sorted(path for path in source.rglob('*') if path.is_file())
    if any(path.is_symlink() for path in source.rglob('*')):
        raise ValueError('symlinks are not supported in evidence directories')
    inventory = {str(path.relative_to(source)): {'bytes': path.stat().st_size, 'sha256': digest_file(path)}
                 for path in files}
    identity = hashlib.sha256(json.dumps(inventory, sort_keys=True).encode()).hexdigest()[:12]
    archives.mkdir(parents=True, exist_ok=True)
    archive = archives / f'{source.name}-{identity}.tar.gz'
    # Never overwrite an existing archive. Verify it against the current source below.
    if not archive.exists():
        with tempfile.NamedTemporaryFile(dir=archives, suffix='.tar.gz', delete=False) as temporary:
            staging = Path(temporary.name)
        try:
            with tarfile.open(staging, 'w:gz') as tar:
                for path in files:
                    tar.add(path, arcname=str(path.relative_to(source)), recursive=False)
            verify_archive(staging, inventory)
            staging.replace(archive)
        finally:
            staging.unlink(missing_ok=True)
    verify_archive(archive, inventory)
    return {'filename': archive.name, 'sha256': digest_file(archive), 'bytes': archive.stat().st_size,
            'fileCount': len(files), 'originalBytes': sum(item['bytes'] for item in inventory.values())}


def verify_archive(archive, inventory):
    with tarfile.open(archive, 'r:gz') as tar:
        members = tar.getmembers()
        if len(members) != len(inventory) or {member.name for member in members} != inventory.keys():
            raise ValueError(f'archive inventory mismatch: {archive}')
        for member in members:
            if not member.isfile():
                raise ValueError('archive contains a non-file entry')
            with tar.extractfile(member) as content:
                actual = hashlib.file_digest(content, 'sha256').hexdigest()
            if actual != inventory[member.name]['sha256'] or member.size != inventory[member.name]['bytes']:
                raise ValueError(f'archive checksum mismatch: {member.name}')


def publish(source, destination, archives):
    source, destination, archives = Path(source).resolve(), Path(destination).resolve(), Path(archives).resolve()
    if not source.is_dir() or source == archives or source in archives.parents:
        raise ValueError('source must exist and archives must be outside it')
    if destination != source and (source in destination.parents or destination.exists()):
        raise ValueError('destination must be a new directory outside source')
    if (source/'reproduction.json').exists():
        raise ValueError('this directory already contains published summaries; use the original raw directory')
    report = read_json(source/'report.json', {})
    environment = read_json(source/'environment.json', {})
    summary = {key: value for key, value in report.items() if key not in ('Samples', 'Phases')}
    summary['Phases'] = [{key: value for key, value in phase.items() if key not in ('Seconds', 'ErrorExamples')}
                         for phase in report.get('Phases', [])]
    omni_report = read_json(source/'omni-report.json', {})
    if omni_report:
        summary = {key: value for key, value in omni_report.items() if key not in ('samples', 'phases')}
        summary['Phases'] = [{key: value for key, value in phase.items() if key not in ('Seconds', 'ErrorExamples')}
                             for phase in omni_report.get('phases', [])]
        summary['ReportType'] = 'omni'
    summary['ReportAvailable'] = bool(report or omni_report)
    diagnostics = {}
    for name in sorted(DIAGNOSTICS):
        path = source/name
        if path.exists():
            diagnostics[name] = read_json(path) if path.suffix == '.json' else path.read_text()
    node = environment.get('node', {}).get('status', {})
    info = node.get('nodeInfo', {})
    reproduction = {
        'sourceRevision': environment.get('sourceRevision', report.get('SourceRevision')),
        'sourceTreeSHA256': environment.get('sourceTreeSHA256'),
        'workingTree': environment.get('workingTree'),
        'images': read_json(source/'images.json', {}),
        'node': {key: info[key] for key in ('architecture', 'containerRuntimeVersion', 'kernelVersion',
                                         'kubeletVersion', 'operatingSystem', 'osImage') if key in info},
        'capacity': node.get('capacity'), 'allocatable': node.get('allocatable'),
        'loadDriver': environment.get('loadDriver'), 'routing': environment.get('routing'),
        'profileEnvironment': {key: value for key, value in read_json(source/'profile.json', {}).items()
                               if key not in ('VMCP_ACCEPTANCE_NODE_IP', 'VMCP_ACCEPTANCE_KUBE_NODE',
                                              'VMCP_ACCEPTANCE_OUTPUT', 'VMCP_ACCEPTANCE_NAMESPACE')},
        'workloads': [],
    }
    for item in read_json(source/'manifest.json', {}).get('items', []):
        if item.get('kind') == 'Deployment':
            spec = item['spec']; pod = spec['template']['spec']
            reproduction['workloads'].append({'name': item['metadata']['name'], 'replicas': spec.get('replicas'),
                'securityContext': pod.get('securityContext'), 'containers': [
                    {key: container[key] for key in ('name', 'image', 'resources', 'securityContext', 'args') if key in container}
                    for container in pod['containers']]})
    # The complete original evidence is archived and every byte checked before removing anything.
    reproduction['rawArchive'] = archive_run(source, archives)
    with tempfile.TemporaryDirectory(prefix='vmcp-load-summary-') as temporary:
        staging = Path(temporary)
        for name in ('summary', 'reproduction', 'diagnostics'):
            value = {'summary': summary, 'reproduction': reproduction, 'diagnostics': diagnostics}[name]
            (staging/f'{name}.json').write_text(json.dumps(value, indent=2, ensure_ascii=False)+'\n')
        for name in ('report.md', 'comparison.md'):
            if (source/name).exists():
                text = rewrite_links((source/name).read_text(), source)
                text += '\n\nHam örnekler ve loglar Git dışında tutulur. Arşiv kimliği ve SHA-256: [reproduction.json](reproduction.json). Bkz. [arşiv politikası](../README.md#raw-artifacts).\n'
                (staging/name).write_text(text)
        for name in ('throughput.svg', 'memory.svg'):
            if (source/name).exists():
                shutil.copyfile(source/name, staging/name)
        destination.mkdir(parents=True, exist_ok=True)
        for path in staging.iterdir():
            shutil.copyfile(path, destination/path.name)
    if destination == source:
        for path in source.iterdir():
            if path.name not in PUBLIC_FILES:
                shutil.rmtree(path) if path.is_dir() else path.unlink()
    return reproduction['rawArchive']


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source', type=Path)
    parser.add_argument('--destination', type=Path, help='new summary directory; default: compact source in place')
    parser.add_argument('--archives', type=Path, default=ROOT/'artifacts/benchmark-archives')
    args = parser.parse_args()
    print(json.dumps(publish(args.source, args.destination or args.source, args.archives), indent=2))
