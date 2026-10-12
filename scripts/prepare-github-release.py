"""Validate the successful image run and stage its public release manifests."""
import argparse
import importlib.util
import json
from pathlib import Path
import re
import tempfile

spec = importlib.util.spec_from_file_location('images', Path(__file__).with_name('prepare-image-release.py'))
images = importlib.util.module_from_spec(spec)
spec.loader.exec_module(images)


def validate_run(run, repository):
    if (run.get('repository', {}).get('full_name') != repository
            or run.get('path') != '.github/workflows/docker-publish.yml'
            or run.get('event') != 'push'
            or run.get('status') != 'completed'
            or run.get('conclusion') != 'success'):
        raise ValueError('source must be a successful tag push to the Docker publishing workflow in this repository')
    version, sha = run.get('head_branch', ''), run.get('head_sha', '')
    if not re.fullmatch(r'v(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?:-[0-9A-Za-z]+(?:[.-][0-9A-Za-z]+)*)?', version) or len(version) > 128:
        raise ValueError('source run must refer to a version tag')
    if not re.fullmatch(r'[0-9a-f]{40}', sha):
        raise ValueError('invalid source commit')
    return version, sha


def stage_manifest(manifest, output, version):
    data = json.loads(Path(manifest).read_text())
    if data['version'] != version:
        raise ValueError('artifact version does not match source run')
    with tempfile.TemporaryDirectory() as temp:
        for index, record in enumerate(data['images']):
            Path(temp, f'{index}.json').write_text(json.dumps(record))
        # Regenerate the overlay from validated digests instead of trusting artifact YAML.
        images.assemble(temp, output, version)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest='command', required=True)
    run = sub.add_parser('validate-run')
    run.add_argument('metadata', type=Path)
    run.add_argument('--repository', required=True)
    run.add_argument('--github-output', type=Path, required=True)
    stage = sub.add_parser('stage')
    stage.add_argument('manifest', type=Path)
    stage.add_argument('output', type=Path)
    stage.add_argument('--version', required=True)
    args = parser.parse_args()
    if args.command == 'validate-run':
        version, sha = validate_run(json.loads(args.metadata.read_text()), args.repository)
        with args.github_output.open('a') as stream:
            stream.write(f'version={version}\nsha={sha}\n')
    else:
        stage_manifest(args.manifest, args.output, args.version)
