# Benchmark evidence policy

Only this policy and the reviewed [aggregate summaries](SUMMARY.md) are published here.
See [issue 31 acceptance](../../maintainer/ISSUE31_ACCEPTANCE.md) for the selected
capacity profile, failures, regression coverage and limitations.

Full reports, raw measurements, charts, query plans, environment manifests,
host identifiers, image inventories and logs stay on the local test machine:

- `artifacts/benchmark-runs/`: raw runs.
- `artifacts/benchmark-summaries/`: automatically generated detailed summaries.
- `artifacts/benchmark-archives/`: checksummed archives.
- `artifacts/benchmark-private/`: verified copies of previously tracked results.

These directories are ignored. Keep private backups separately; Git is not their
backup. Public summaries must be manually reviewed and contain only workload
parameters, aggregate metrics, verdicts and limitations. Never turn missing or
failed checks into passes. Do not publish internal addresses, local paths,
credentials, request/response samples, SQL diagnostics or infrastructure dumps.

Renderer tests use a small synthetic fixture under `benchmark/kubernetes/fixtures/`.
Historical comparison is optional: supply a local baseline report to `compare.py`
as its second argument. Without it, baseline columns are unmeasured.

The cleanup removes files from the current tree only. Earlier commits, existing
clones and previously published PR links still retain their old contents; history
has intentionally not been rewritten.
