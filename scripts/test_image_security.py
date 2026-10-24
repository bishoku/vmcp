"""Failure-path contracts for multi-platform release scanning."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).with_name('scan-image.sh').resolve()


class ImageSecurityTest(unittest.TestCase):
    def run_scan(self, fail='', missing='', platforms=('linux/amd64', 'linux/arm64')):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            trivy = root / 'trivy'
            trivy.write_text('''#!/usr/bin/env python3
import json,os,pathlib,sys
args=sys.argv[1:]
platform=args[args.index('--platform')+1]
with open(os.environ['CALLS'],'a') as file: file.write(json.dumps(args)+'\\n')
if platform != os.environ['MISSING']:
    pathlib.Path(args[args.index('--output')+1]).write_text(json.dumps({'Results':[]}))
sys.exit(1 if platform == os.environ['FAIL'] else 0)
''')
            trivy.chmod(0o755)
            env = dict(os.environ, PATH=str(root)+os.pathsep+os.environ['PATH'],
                       CALLS=str(root/'calls'), FAIL=fail, MISSING=missing)
            result = subprocess.run(['bash', str(SCRIPT), 'example/image@sha256:'+'a'*64,
                                     str(root/'reports'), *platforms], env=env, capture_output=True, text=True)
            calls = [json.loads(line) for line in (root/'calls').read_text().splitlines()] if (root/'calls').exists() else []
            reports = sorted(p.name for p in (root/'reports').glob('*.json'))
            return result, calls, reports

    def test_both_platforms_clean(self):
        result, calls, reports = self.run_scan()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(calls), 2)
        self.assertEqual(reports, ['linux-amd64.json', 'linux-arm64.json'])
        for args in calls:
            self.assertIn('--ignore-unfixed=false', args)
            self.assertEqual(args[args.index('--severity')+1], 'HIGH,CRITICAL')
            self.assertEqual(args[args.index('--scanners')+1], 'vuln')

    def test_first_failure_still_scans_second_platform(self):
        result, calls, reports = self.run_scan(fail='linux/amd64')
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(len(calls), 2)
        self.assertEqual(len(reports), 2)

    def test_arm64_failure_blocks_release(self):
        result, _, _ = self.run_scan(fail='linux/arm64')
        self.assertNotEqual(result.returncode, 0)

    def test_missing_report_is_failure_even_when_scanner_exits_zero(self):
        result, calls, _ = self.run_scan(missing='linux/amd64')
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(len(calls), 2)
        self.assertIn('No vulnerability report', result.stderr)

    def test_unsupported_platform_is_rejected(self):
        result, calls, _ = self.run_scan(platforms=('unknown/unknown',))
        self.assertEqual(result.returncode, 2)
        self.assertEqual(calls, [])


class SignatureVisibilityTest(unittest.TestCase):
    def check(self, codes):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            tool = root/'cosign'
            tool.write_text("""#!/usr/bin/env python3
import json,os,pathlib,sys
path=pathlib.Path(os.environ['CALLS'])
calls=path.read_text().splitlines() if path.exists() else []
with path.open('a') as file:file.write(json.dumps(sys.argv[1:])+'\\n')
codes=json.loads(os.environ['CODES'])
code=codes[min(len(calls),len(codes)-1)]
print('no signatures found' if code==10 else 'certificate identity mismatch' if code else 'verified')
sys.exit(code)
""")
            tool.chmod(0o755)
            env = dict(os.environ, PATH=str(root)+os.pathsep+os.environ['PATH'],
                CALLS=str(root/'calls'), CODES=json.dumps(codes),
                VMCP_SIGNATURE_VERIFY_ATTEMPTS='3', VMCP_SIGNATURE_VERIFY_DELAY_SECONDS='0')
            result = subprocess.run(['bash',str(SCRIPT.with_name('verify-image-signature.sh')),
                'example/image@sha256:'+'a'*64,'https://github.com/example/release@refs/tags/v1.0.1'],
                env=env, capture_output=True, text=True)
            calls = [json.loads(line) for line in (root/'calls').read_text().splitlines()]
            return result,calls

    def test_registry_visibility_is_retried_without_weakening_identity(self):
        result,calls=self.check([10,10,0])
        self.assertEqual(result.returncode,0,result.stderr)
        self.assertEqual(len(calls),3)
        for args in calls:
            self.assertEqual(args[args.index('--certificate-oidc-issuer')+1], 'https://token.actions.githubusercontent.com')
            self.assertEqual(args[args.index('--certificate-identity')+1], 'https://github.com/example/release@refs/tags/v1.0.1')
            self.assertFalse(any('insecure' in arg for arg in args))

    def test_identity_or_certificate_failure_is_never_retried(self):
        result,calls=self.check([1,0])
        self.assertEqual(result.returncode,1)
        self.assertEqual(len(calls),1)

    def test_unsigned_image_still_fails_after_bounded_window(self):
        result,calls=self.check([10])
        self.assertEqual(result.returncode,10)
        self.assertEqual(len(calls),3)

    def test_other_tool_failure_is_not_retried(self):
        result,calls=self.check([127,0])
        self.assertEqual(result.returncode,127)
        self.assertEqual(len(calls),1)


if __name__ == '__main__':
    unittest.main()
