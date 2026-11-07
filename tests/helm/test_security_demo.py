"""Readiness prevents discovery writes racing Keycloak startup."""
import importlib.util
import pathlib
import sys
import unittest
import urllib.error
from unittest.mock import patch, MagicMock

ROOT=pathlib.Path(__file__).resolve().parents[2]
DEMO=ROOT/'deploy/helm/vmcp/quickstart/security'
sys.path.insert(0,str(DEMO))
spec=importlib.util.spec_from_file_location('security_demo_run',DEMO/'run.py')
lab=importlib.util.module_from_spec(spec)
spec.loader.exec_module(lab)
sys.path.pop(0)

class ReadinessTest(unittest.TestCase):
    def test_waits_for_verified_https_before_discovery(self):
        ready=MagicMock()
        ready.__enter__.return_value.status=200
        with patch.object(lab.ssl,'create_default_context') as tls, patch.object(lab.urllib.request,'urlopen',side_effect=[urllib.error.URLError('not ready'),ready]) as request, patch.object(lab.time,'sleep') as sleep:
            lab.wait_for_demo(pathlib.Path('/tmp/demo-ca.pem'),attempts=2)
            tls.assert_called_once_with(cafile='/tmp/demo-ca.pem')
            self.assertEqual(request.call_count,2)
            sleep.assert_called_once_with(2)
            self.assertEqual(request.call_args.kwargs['context'],tls.return_value)

    def test_unavailable_service_stops_setup(self):
        with patch.object(lab.ssl,'create_default_context'), patch.object(lab.urllib.request,'urlopen',side_effect=TimeoutError), patch.object(lab.time,'sleep'):
            with self.assertRaisesRegex(RuntimeError,'did not become ready'):
                lab.wait_for_demo('/tmp/demo-ca.pem',attempts=2)
