"""Choose release verification using GitHub's main push CI evidence."""
import json
import os
import subprocess
import time
from datetime import datetime, timedelta, timezone
from pathlib import Path

REQUIRED_JOBS = {
    "core", "web", "upstream-sdk", "Invocation security demo",
    "Helm and image validation (disposable cluster)", "Image security",
}
MAX_AUDIT_AGE = timedelta(hours=24)


def api(path):
    # An API failure must not disguise a failed main run as missing evidence.
    return json.loads(subprocess.check_output(["gh", "api", path], timeout=60))


def verification_mode(repo, sha, *, fetch=api, sleep=time.sleep, now=None):
    root = f"repos/{repo}/actions"
    query = f"{root}/workflows/ci.yml/runs?branch=main&event=push&head_sha={sha}&per_page=1"
    while True:
        runs = fetch(query)["workflow_runs"]
        if not runs:
            return "full", "No matching main push CI run"
        run = runs[0]
        if (run["repository"]["full_name"] != repo or run["head_sha"] != sha
                or run["head_branch"] != "main" or run["event"] != "push"
                or run["path"] != ".github/workflows/ci.yml"):
            return "full", "Run identity does not match tag commit and main workflow"
        url = run["html_url"]
        if run["status"] == "completed":
            break
        print(f"Waiting for main CI: {url}", flush=True)
        sleep(30)

    if run["conclusion"] != "success":
        raise RuntimeError(f"Main CI concluded {run['conclusion']}: {url}")

    # The latest jobs view includes checks carried forward from partial reruns.
    jobs = fetch(f"{root}/runs/{run['id']}/jobs?filter=latest&per_page=100")
    checks = jobs["jobs"]
    if (len(checks) != jobs["total_count"]
            or not REQUIRED_JOBS.issubset({job["name"] for job in checks})
            or any(job["status"] != "completed" or job["conclusion"] != "success" for job in checks)):
        return "full", f"Incomplete successful job evidence: {url}"
    created = datetime.fromisoformat(run["created_at"].replace("Z", "+00:00"))
    age = (now or datetime.now(timezone.utc)) - created
    return ("refresh" if age >= MAX_AUDIT_AGE else "reuse"), url


if __name__ == "__main__":
    mode, evidence = verification_mode(os.environ["GITHUB_REPOSITORY"], os.environ["RELEASE_SHA"])
    print(f"Release verification: {mode} ({evidence})")
    with Path(os.environ["GITHUB_OUTPUT"]).open("a") as out:
        print(f"mode={mode}", file=out)
    with Path(os.environ["GITHUB_STEP_SUMMARY"]).open("a") as summary:
        print(f"Release verification: **{mode}**. {evidence}", file=summary)
