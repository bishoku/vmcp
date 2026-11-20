import unittest
from datetime import datetime, timezone
from unittest.mock import Mock

from release_ci import REQUIRED_JOBS, verification_mode


class MainEvidenceTests(unittest.TestCase):
    def setUp(self):
        self.repo = "example/vmcp"
        self.sha = "a" * 40
        self.now = datetime(2026, 11, 20, tzinfo=timezone.utc)
        self.run = {
            "repository": {"full_name": self.repo}, "head_sha": self.sha,
            "head_branch": "main", "event": "push", "path": ".github/workflows/ci.yml",
            "status": "completed", "conclusion": "success", "id": 123,
            "created_at": "2026-11-19T23:00:00Z", "html_url": "https://github.com/example/vmcp/actions/runs/123",
        }
        self.jobs = [{"name": name, "status": "completed", "conclusion": "success"} for name in REQUIRED_JOBS]
        self.list_runs = f"repos/{self.repo}/actions/workflows/ci.yml/runs?branch=main&event=push&head_sha={self.sha}&per_page=1"
        self.list_jobs = f"repos/{self.repo}/actions/runs/123/jobs?filter=latest&per_page=100"
        self.sleep = Mock()

    def decide(self, runs=None, jobs=None, total=None):
        jobs = self.jobs if jobs is None else jobs
        responses = iter([[self.run]] if runs is None else runs)

        def fetch(path):
            if path == self.list_runs:
                return {"workflow_runs": next(responses)}
            self.assertEqual(path, self.list_jobs)
            return {"jobs": jobs, "total_count": len(jobs) if total is None else total}

        return verification_mode(self.repo, self.sha, fetch=fetch, sleep=self.sleep, now=self.now)[0]

    def test_exact_main_evidence_reused(self):
        self.assertEqual(self.decide(), "reuse")

    def test_absent_or_untrusted_evidence_runs_full_ci(self):
        self.assertEqual(self.decide(runs=[[]]), "full")
        for key, value in [("repository", {"full_name": "fork/vmcp"}), ("head_sha", "b"*40),
                           ("head_branch", "feature"), ("event", "pull_request"), ("path", "other.yml")]:
            with self.subTest(key=key):
                self.assertEqual(self.decide(runs=[[dict(self.run, **{key: value})]]), "full")

    def test_wait_for_matching_main_run(self):
        self.assertEqual(self.decide(runs=[[dict(self.run, status="in_progress", conclusion=None)], [self.run]]), "reuse")
        self.sleep.assert_called_once_with(30)

    def test_unsuccessful_main_blocks_publication(self):
        for result in ["failure", "cancelled", "timed_out"]:
            with self.subTest(result=result), self.assertRaises(RuntimeError):
                self.decide(runs=[[dict(self.run, conclusion=result)]])

    def test_skipped_missing_or_partial_checks_require_full_ci(self):
        self.assertEqual(self.decide(jobs=self.jobs[1:]), "full")
        self.assertEqual(self.decide(jobs=[dict(self.jobs[0], conclusion="skipped"), *self.jobs[1:]]), "full")
        self.assertEqual(self.decide(total=101), "full")

    def test_old_evidence_refreshes_security_scans(self):
        self.run["created_at"] = "2026-11-19T00:00:00Z"
        self.assertEqual(self.decide(), "refresh")

    def test_api_error_blocks_publication(self):
        with self.assertRaises(OSError):
            verification_mode(self.repo, self.sha, fetch=Mock(side_effect=OSError("API unavailable")))
