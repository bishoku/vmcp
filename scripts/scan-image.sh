#!/usr/bin/env bash
# Scan every requested platform, preserve evidence, then fail closed.
set -euo pipefail
if (( $# < 3 )); then
  echo "Usage: $0 IMAGE REPORT_DIRECTORY PLATFORM [PLATFORM ...]" >&2
  exit 2
fi
image=$1
reports=$2
shift 2
mkdir -p "$reports"
failed=0
for platform in "$@"; do
  case "$platform" in
    linux/amd64|linux/arm64) ;;
    *) echo "Unsupported scan platform: $platform" >&2; exit 2 ;;
  esac
  report="$reports/${platform//\//-}.json"
  if ! trivy image --platform "$platform" --scanners vuln \
      --severity HIGH,CRITICAL --ignore-unfixed=false --exit-code 1 \
      --timeout 10m --format json --output "$report" "$image"; then
    failed=1
  fi
  # A failed download/scan must never be treated as an empty clean result.
  if [[ ! -s "$report" ]]; then
    echo "No vulnerability report produced for $image ($platform)" >&2
    failed=1
  fi
done
exit "$failed"
