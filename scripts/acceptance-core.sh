#!/usr/bin/env bash
set -euo pipefail
vmcp_acceptance_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
vmcp_acceptance_tmp="$(mktemp -d /tmp/vmcp-acceptance.XXXXXX)"
vmcp_acceptance_project="vmcp-acceptance-$(date +%s)-$$"
vmcp_acceptance_compose="$vmcp_acceptance_root/benchmark/acceptance/compose.yaml"
export VMCP_ACCEPTANCE_OUTPUT="${VMCP_ACCEPTANCE_OUTPUT:-$vmcp_acceptance_root/benchmark/results/acceptance-$(date +%Y%m%d-%H%M%S)}"
mkdir -p "$VMCP_ACCEPTANCE_OUTPUT"
export VMCP_ACCEPTANCE_OUTPUT="$(cd "$VMCP_ACCEPTANCE_OUTPUT" && pwd)"
cleanup() {
  # Also cover an externally interrupted/timed-out test runner. PID reuse is
  # guarded by checking the exact private executable path before signaling.
  for vmcp_acceptance_pid_file in "$VMCP_ACCEPTANCE_OUTPUT"/acceptance-node-*.pid; do
    [[ -f "$vmcp_acceptance_pid_file" ]] || continue
    vmcp_acceptance_pid="$(cat "$vmcp_acceptance_pid_file")"
    [[ "$vmcp_acceptance_pid" =~ ^[0-9]+$ ]] || continue
    vmcp_acceptance_command="$(ps -o command= -p "$vmcp_acceptance_pid" 2>/dev/null || true)"
    if [[ "$vmcp_acceptance_command" == "$vmcp_acceptance_tmp/data-plane"* ]]; then
      kill -KILL "$vmcp_acceptance_pid" 2>/dev/null || true
    fi
    rm -f "$vmcp_acceptance_pid_file"
  done
  docker compose -p "$vmcp_acceptance_project" -f "$vmcp_acceptance_compose" down -v --remove-orphans >/dev/null 2>&1 || true
  rm -rf "$vmcp_acceptance_tmp"
  if [[ -f "$VMCP_ACCEPTANCE_OUTPUT/report.json" ]]; then
    python3 "$vmcp_acceptance_root/benchmark/acceptance/report.py" "$VMCP_ACCEPTANCE_OUTPUT/report.json" >/dev/null || true
  fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
# A unique Compose project owns every container/volume removed by this script.
docker compose -p "$vmcp_acceptance_project" -f "$vmcp_acceptance_compose" up -d --wait
export VMCP_ACCEPTANCE_CONTAINERS="$(docker compose -p "$vmcp_acceptance_project" -f "$vmcp_acceptance_compose" ps -q | tr '\n' ' ')"
export VMCP_TEST_DATABASE_URL="postgres://acceptance:acceptance-only@127.0.0.1:${VMCP_ACCEPTANCE_PG_PORT:-55434}/acceptance?sslmode=disable"
export VMCP_ACCEPTANCE_REDIS_URL="redis://127.0.0.1:${VMCP_ACCEPTANCE_REDIS_PORT:-56381}/0"
export VMCP_ACCEPTANCE_BINARY="$vmcp_acceptance_tmp/data-plane"
export VMCP_RUN_ACCEPTANCE=1
cd "$vmcp_acceptance_root/core"
go build -o "$VMCP_ACCEPTANCE_BINARY" ./cmd/data-plane
# The child node processes are built without race instrumentation for valid timings.
vmcp_acceptance_test_flags=()
if [[ "${VMCP_ACCEPTANCE_RACE:-0}" == "1" ]]; then
  vmcp_acceptance_test_flags+=(-race)
fi
go test "${vmcp_acceptance_test_flags[@]}" -count=1 -timeout=20m -run '^TestScaleAcceptance$' -v ./internal/e2e 2>&1 | tee "$VMCP_ACCEPTANCE_OUTPUT/run.log"
