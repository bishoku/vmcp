import json
from pathlib import Path
import tarfile
import tempfile
import unittest
from unittest.mock import patch
import publish


class PublishEvidenceTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.source = self.root/'microk8s-example'; self.source.mkdir()
        self.archives = self.root/'archives'
        report = {'Pass': False, 'Failures': ['revocation timed out'], 'Samples': [{'large': 'sample'}],
                  'Phases': [{'CompletedRPS': 998.7, 'Errors': 3, 'P99MS': 485.4,
                              'slo_pass': True, 'Seconds': [1, 2], 'ErrorExamples': ['raw']}],
                  'Consistency': None, 'BinarySHA256': 'binary'}
        (self.source/'report.json').write_text(json.dumps(report))
        (self.source/'environment.json').write_text(json.dumps({'sourceRevision': 'commit',
            'sourceTreeSHA256': 'tree', 'workingTree': ' M core/example.go',
            'node': {'status': {'nodeInfo': {'architecture': 'amd64', 'machineID': 'private-machine'}}}}))
        (self.source/'profile.json').write_text(json.dumps({'VMCP_ACCEPTANCE_RPS': '1000', 'VMCP_ACCEPTANCE_NODE_IP': 'private-address'}))
        (self.source/'postgres-effective-settings.json').write_text('{"shared_buffers":"1GB"}')
        (self.source/'report.md').write_text('[raw](report.json) [SQL](postgres-samples.jsonl) [settings](postgres-effective-settings.json) [plot](throughput.svg)')
        (self.source/'throughput.svg').write_text('<svg/>')
        (self.source/'postgres-samples.jsonl').write_text('raw evidence\n')
        (self.source/'run.log').write_text('full log\n')

    def test_compaction_preserves_failed_gates_hashes_and_verifiable_originals(self):
        originals = {p.name: p.read_bytes() for p in self.source.iterdir()}
        archive = publish.publish(self.source, self.source, self.archives)
        with tarfile.open(self.archives/archive['filename']) as tar:
            self.assertEqual({member.name: tar.extractfile(member).read() for member in tar.getmembers()}, originals)
        self.assertEqual(publish.digest_file(self.archives/archive['filename']), archive['sha256'])
        self.assertFalse((self.source/'run.log').exists())
        summary = json.loads((self.source/'summary.json').read_text())
        self.assertFalse(summary['Pass']); self.assertTrue(summary['Phases'][0]['slo_pass'])
        self.assertEqual(summary['Failures'], ['revocation timed out'])
        self.assertIsNone(summary['Consistency'])
        self.assertNotIn('Samples', summary); self.assertNotIn('Seconds', summary['Phases'][0])
        self.assertEqual(summary['Phases'][0]['CompletedRPS'], 998.7)
        reproduction = json.loads((self.source/'reproduction.json').read_text())
        self.assertEqual(reproduction['sourceTreeSHA256'], 'tree')
        self.assertNotIn('machineID', reproduction['node'])
        self.assertNotIn('VMCP_ACCEPTANCE_NODE_IP', reproduction['profileEnvironment'])
        markdown = (self.source/'report.md').read_text()
        self.assertNotIn('](report.json)', markdown)
        self.assertIn('](diagnostics.json)', markdown)
        self.assertIn('](throughput.svg)', markdown)

    def test_failed_archive_verification_never_removes_original_files(self):
        originals = {p.name: p.read_bytes() for p in self.source.iterdir()}
        with patch.object(publish, 'verify_archive', side_effect=ValueError('corrupted archive')):
            with self.assertRaises(ValueError):
                publish.publish(self.source, self.source, self.archives)
        self.assertEqual({p.name: p.read_bytes() for p in self.source.iterdir()}, originals)

    def test_default_runner_can_export_summary_without_modifying_raw_directory(self):
        destination = self.root/'public'
        publish.publish(self.source, destination, self.archives)
        self.assertTrue((self.source/'run.log').exists())
        self.assertFalse((destination/'run.log').exists())
        self.assertTrue((destination/'summary.json').exists())
        with self.assertRaises(ValueError):
            publish.publish(self.source, destination, self.archives)

    def test_incomplete_run_does_not_invent_a_pass(self):
        (self.source/'report.json').unlink()
        publish.publish(self.source, self.source, self.archives)
        summary = json.loads((self.source/'summary.json').read_text())
        self.assertFalse(summary['ReportAvailable']); self.assertNotIn('Pass', summary)

    def test_symlink_is_rejected_before_archive_or_cleanup(self):
        (self.source/'link').symlink_to(self.source/'run.log')
        with self.assertRaises(ValueError):
            publish.publish(self.source, self.source, self.archives)
        self.assertTrue((self.source/'run.log').exists())


if __name__ == '__main__':
    unittest.main()
