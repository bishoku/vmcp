"""Summarize the Go test evidence for the nine expected server profiles."""
import json
import sys
import re
from itertools import product
from pathlib import Path

events = [json.loads(line) for line in Path(sys.argv[1]).read_text().splitlines()]
status = {
    e["Test"]: e["Action"]
    for e in events
    if "Test" in e and e["Action"] in {"pass", "fail", "skip"}
}
control = {}
data = {}
for event in events:
    name = event.get("Test", "")
    line = event.get("Output", "")
    if match := re.search(r"control-plane revision=([0-9-]+)", line):
        control[name] = match[1]
    if match := re.search(r"wire method=tools/call version=([0-9-]+)", line):
        data.setdefault(name.rsplit("/", 2)[0], set()).add(match[1])
print("### Real upstream SDK interoperability\n")
print("| Server | Profile | Observed CP | Observed DP | Virtual legacy | Virtual modern | Omni legacy | Omni modern |")
print("|---|---|---|---|---|---|---|---|")
versions = {
    "go": ["1.8.0"] * 3,
    "typescript": ["1.32.1", "2.4.0", "2.4.0"],
    "fastmcp": ["3.4.11", "4.2.2", "4.2.2"],
}
success = True
for sdk, releases in versions.items():
    for profile, release in zip(("legacy", "modern", "dual"), releases, strict=True):
        prefix = f"TestRealUpstreamServers/{sdk}/{profile}"
        results = [status.get(f"{prefix}/{endpoint}/{client}", "missing")
                   for endpoint, client in product(("virtual", "omni"), ("legacy", "modern"))]
        success &= status.get(prefix) == "pass" and all(r == "pass" for r in results)
        version = "2025-06-18" if profile == "legacy" else "2026-07-28"
        success &= control.get(prefix) == version and data.get(prefix) == {version}
        print("|", " | ".join([f"{sdk} {release}", profile, control.get(prefix, "missing"),
                              ", ".join(sorted(data.get(prefix, {"missing"}))), *results]), "|")
raise SystemExit(0 if success else 1)
