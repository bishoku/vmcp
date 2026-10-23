# Benchmark result retention

Commit benchmark scripts, profiles, analysis reports and small curated results.
New MicroK8s runs export only these files per run:

| File | Purpose |
|---|---|
| `report.md`, `comparison.md` | Findings, thresholds, caveats and comparisons |
| `summary.json` | Phase TPS, latency, error/drop counts, resources, consistency, functional checks and overall acceptance |
| `reproduction.json` | Source/tree/binary context, dirty working-tree listing, image IDs, software/resource configuration, profile and raw archive SHA-256 |
| `diagnostics.json` | Selected PostgreSQL settings, counters and query plans used to support the analysis |
| `throughput.svg`, `memory.svg` | Small reviewable charts |

Per-second samples, individual errors, Kubernetes snapshots, SQL time series and
full logs are excluded from Git. Failed runs retain their failed gates; a failed
or missing check is never converted into a successful result. A setup attempt
without `report.json` has `ReportAvailable: false`, without an invented verdict.

The existing Compose `report.json` fixtures remain unchanged: report regression
tests and historical comparisons consume them. This cleanup applies to the new
MicroK8s runs, not to source fixtures or application configuration.

Source/tree/binary hashes identify the measured dirty build. The working-tree
listing is not a patch and cannot reconstruct historical uncommitted source by
itself; do not claim that a HEAD checkout exactly reproduces a dirty build.

## Raw artifacts

Raw evidence is local and is **not uploaded** by the benchmark runner:

- `artifacts/benchmark-runs/<run>/`: complete evidence for future runs.
- `artifacts/benchmark-archives/<run>-<inventory-hash>.tar.gz`: verified compressed originals.
- Each run's `reproduction.json` records the archive filename, SHA-256, compressed
  size, original file count and original byte count.

The 2026-10-23 cleanup archived all 509 original MicroK8s files (43,195,491 bytes)
before compacting the result folders. Eight compressed archives total 4,259,984
bytes. Every archived member's name, size and SHA-256 was compared with its source
before any raw file was removed. Original Markdown/JSON/graphs are also preserved
inside those archives. The two top-level analysis reports remain in Git.

To inspect an archive, use its filename and expected SHA-256 from the corresponding
`reproduction.json`. From the repository root, for example:

```sh
sha256sum artifacts/benchmark-archives/<archive-filename>.tar.gz
# Compare with reproduction.json before opening. Extract into a new empty folder.
mkdir -p artifacts/benchmark-restored/<run>
tar -xzf artifacts/benchmark-archives/<archive-filename>.tar.gz \
  -C artifacts/benchmark-restored/<run>
```

Keep these archives in an appropriate private artifact store or backup if they
must survive loss of this checkout. Raw output can contain host details, errors,
request data or credentials; review/redact before any external upload. No remote
artifact store or retention policy is provisioned by this repository.

The MicroK8s runner automatically exports future completed runs. To compact another
finished raw directory manually without altering its original files:

```sh
python3 benchmark/kubernetes/publish.py artifacts/benchmark-runs/<run> \
  --destination benchmark/results/<run>
```

The destination must be new. Omitting `--destination` compacts the source directory
in place only after verifying its archive. Do not run this against an active load
test. Git ignore rules allow only curated MicroK8s result filenames, preventing
accidental addition of new logs and snapshots.
