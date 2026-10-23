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


if __name__ == '__main__':
    unittest.main()
