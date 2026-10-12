import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('release', Path(__file__).with_name('prepare-image-release.py'))
release = importlib.util.module_from_spec(spec)
spec.loader.exec_module(release)


class ImageReleaseTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.input = self.root / 'digests'
        self.input.mkdir()
        self.output = self.root / 'output'
        for service in release.SERVICES:
            self.write(service)

    def write(self, service, **updates):
        record = {'service': service, 'version': 'v0.0.1', 'image': f'barishoku/vmcp-{service}', 'digest': 'sha256:' + 'a' * 64}
        record.update(updates)
        (self.input / f'{service}.json').write_text(json.dumps(record))

    def test_complete_release_creates_digest_values_without_enabling_optional_workloads(self):
        release.assemble(self.input, self.output, 'v0.0.1')
        values = (self.output / 'helm-images.yaml').read_text()
        self.assertIn('barishoku/vmcp-web@sha256:', values)
        self.assertIn('barishoku/vmcp-migrate@sha256:', values)
        self.assertIn('  kafka:\n    image:', values)
        self.assertNotIn('enabled:', values)
        self.assertEqual(len(json.loads((self.output / 'images.json').read_text())['images']), 9)

    def test_missing_image_does_not_produce_a_partial_release(self):
        (self.input / 'web.json').unlink()
        with self.assertRaises(ValueError):
            release.assemble(self.input, self.output, 'v0.0.1')
        self.assertFalse(self.output.exists())

    def test_mixed_version_wrong_repository_and_invalid_digest_are_rejected(self):
        for updates in [{'version': 'v0.0.2'}, {'image': 'other/vmcp-web'}, {'digest': 'latest'}]:
            with self.subTest(updates=updates):
                self.write('web', **updates)
                with self.assertRaises(ValueError):
                    release.assemble(self.input, self.output, 'v0.0.1')

    def test_duplicate_image_is_rejected(self):
        (self.input / 'duplicate.json').write_text((self.input / 'web.json').read_text())
        with self.assertRaises(ValueError):
            release.assemble(self.input, self.output, 'v0.0.1')

    def test_invalid_tag_is_rejected(self):
        for version in ['main', 'v01.0.1', 'v0.0.1+build', 'v0.0.1;echo bad']:
            with self.subTest(version=version), self.assertRaises(ValueError):
                release.assemble(self.input, self.output, version)


if __name__ == '__main__':
    unittest.main()
