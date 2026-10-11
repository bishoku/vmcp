#!/usr/bin/env python3
"""Reject silent skips and unsuccessful/incomplete JSON test runs in release CI."""
import collections
import json
import sys
counts = collections.Counter()
packages = {}
no_tests = set()
for line in open(sys.argv[1], encoding="utf-8"):
    event = json.loads(line)
    if "[no test files]" in event.get("Output", ""):
        no_tests.add(event["Package"])
    if event.get("Test"):
        counts[event["Action"]] += 1
    elif event["Action"] in ("start", "pass", "fail", "skip"):
        packages[event["Package"]] = event["Action"]
if counts["fail"] or counts["skip"] or not counts["pass"] or any(v != "pass" and not (v == "skip" and k in no_tests) for k, v in packages.items()):
    sys.exit(f"Release test gate failed: {dict(counts)}; packages={packages}")
print(f"Passed {counts['pass']} Go tests; no skips; {len(packages)} packages")
