#!/usr/bin/env bash
set -euo pipefail
: "${VMCP_TEST_DATABASE_URL:?Set VMCP_TEST_DATABASE_URL to a local disposable PostgreSQL instance}"
vmcp_benchmark_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$vmcp_benchmark_root/core"
go test -run '^$' \
  -bench 'Benchmark(PreparedCatalog|PrepareSnapshotMetadata|AuditBatch|OperationsWithScrapes|PersistentToolCalls)$' \
  -benchmem -benchtime="${VMCP_BENCH_TIME:-1s}" -count="${VMCP_BENCH_COUNT:-3}" \
  "$@" ./internal/config ./internal/audit ./internal/telemetry ./internal/e2e
