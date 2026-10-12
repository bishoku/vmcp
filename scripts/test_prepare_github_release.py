import copy
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('github_release', Path(__file__).with_name('prepare-github-release.py'))
release = importlib.util.module_from_spec(spec)
spec.loader.exec_module(release)


class GitHubReleaseTests(unittest.TestCase):
    def run_metadata(self):
        return dict(repository={'full_name': 'bishoku/vmcp'}, path='.github/workflows/docker-publish.yml',
                    event='push', status='completed', conclusion='success', head_branch='v0.0.2', head_sha='a' * 40)

    def test_accepts_completed_tag_publication_and_prerelease(self):
        for version in ['v0.0.2', 'v0.0.3-rc.1']:
            run = self.run_metadata()
            run['head_branch'] = version
            self.assertEqual(release.validate_run(run, 'bishoku/vmcp'), (version, 'a' * 40))

    def test_rejects_failed_untrusted_or_non_version_sources(self):
        for updates in [dict(conclusion='failure'), dict(status='in_progress'), dict(event='pull_request'),
                        dict(path='.github/workflows/ci.yml'), dict(repository={'full_name': 'other/vmcp'}),
                        dict(head_branch='main'), dict(head_branch='v01.0.2'), dict(head_sha='main')]:
            with self.subTest(updates=updates), self.assertRaises(ValueError):
                release.validate_run(self.run_metadata() | updates, 'bishoku/vmcp')

    def test_stages_only_complete_consistent_image_inventory(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            manifest = root / 'images.json'
            data = {'version': 'v0.0.2', 'images': [dict(service=s, version='v0.0.2',
                    image=f'barishoku/vmcp-{s}', digest='sha256:' + 'a' * 64) for s in sorted(release.images.SERVICES)]}
            manifest.write_text(json.dumps(data))
            release.stage_manifest(manifest, root / 'valid', 'v0.0.2')
            self.assertIn('barishoku/vmcp-web@sha256:', (root / 'valid/helm-images.yaml').read_text())
            for change in ['version', 'missing', 'duplicate', 'digest']:
                bad = copy.deepcopy(data)
                if change == 'version': bad['version'] = 'v0.0.1'
                if change == 'missing': bad['images'].pop()
                if change == 'duplicate': bad['images'].append(bad['images'][0])
                if change == 'digest': bad['images'][0]['digest'] = 'latest'
                manifest.write_text(json.dumps(bad))
                with self.subTest(change=change), self.assertRaises(ValueError):
                    release.stage_manifest(manifest, root / change, 'v0.0.2')
                self.assertFalse((root / change).exists())


if __name__ == '__main__':
    unittest.main()
