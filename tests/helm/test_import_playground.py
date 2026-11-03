"""Standalone import regressions; no cluster or external API required."""
from contextlib import redirect_stdout
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

from test_playground import MemoryAPI

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / 'deploy/helm/vmcp/quickstart/import_playground.py'
sys.path.insert(0, str(SCRIPT.parent))
spec = importlib.util.spec_from_file_location('import_playground', SCRIPT)
imp = importlib.util.module_from_spec(spec)
spec.loader.exec_module(imp)


class ExistingAPI(MemoryAPI):
    enabled = True

    def request(self, method, path, body=None):
        if method == 'GET' and path == '/api/v1/mock-provider':
            return {'enabled': self.enabled, 'url': 'http://existing-wiremock:8080'}
        if path == '/api/v1/omni/reindex':
            self.reindexed = True
            return {}
        return super().request(method, path, body)


class ImportTests(unittest.TestCase):
    def test_help_without_credentials_or_cluster(self):
        result = subprocess.run([sys.executable, str(SCRIPT), '--help'], capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('--state-file', result.stdout)

    def test_import_persists_keys_and_rerun_preserves_records(self):
        api = ExistingAPI()
        with tempfile.TemporaryDirectory() as tmp:
            state = Path(tmp) / 'state.json'
            args = ['--api-url', 'http://control:8081', '--state-file', str(state),
                    '--issuer', 'https://sso/realms/demo', '--omni']
            with patch.dict(os.environ, {'VMCP_ADMIN_TOKEN': 'test'}), \
                    patch.object(imp.playground, 'API', return_value=api), redirect_stdout(io.StringIO()):
                self.assertEqual(imp.main(args), 0)
                self.assertEqual(len(json.loads(state.read_text())['keys']), 10)
                self.assertEqual(state.stat().st_mode & 0o777, 0o600)
                self.assertTrue(all(u['url'] == 'http://existing-wiremock:8080' for u in api.data['upstreams']))
                self.assertTrue(all(t['issuer'] == 'https://sso/realms/demo' for t in api.data['teams']))
                self.assertTrue(all(s['omniEnabled'] for s in api.data['virtual-servers']))
                self.assertTrue(api.reindexed)
                self.assertEqual(api.data['identity-bindings'], [])
                api.writes.clear()
                imp.main(args)
                self.assertEqual(api.writes, [])
                with self.assertRaisesRegex(ValueError, 'different API URL'):
                    imp.main(['--api-url', 'http://other', '--state-file', str(state)])

    def test_disabled_provider_does_not_create_resources_or_state(self):
        api = ExistingAPI()
        api.enabled = False
        with tempfile.TemporaryDirectory() as tmp:
            state = Path(tmp) / 'state.json'
            with patch.dict(os.environ, {'VMCP_ADMIN_TOKEN': 'test'}), \
                    patch.object(imp.playground, 'API', return_value=api):
                with self.assertRaisesRegex(ValueError, 'VMCP_WIREMOCK_URL'):
                    imp.main(['--api-url', 'http://control', '--state-file', str(state)])
            self.assertEqual(api.writes, [])
            self.assertFalse(state.exists())
