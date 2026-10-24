#!/usr/bin/env bash
# Registry referrer indexes may briefly lag a completed signature upload.
set -euo pipefail
if (( $# != 2 )); then
  echo "Usage: $0 IMAGE@DIGEST CERTIFICATE_IDENTITY" >&2
  exit 2
fi
image=$1
identity=$2
attempts=${VMCP_SIGNATURE_VERIFY_ATTEMPTS:-10}
delay=${VMCP_SIGNATURE_VERIFY_DELAY_SECONDS:-10}
if [[ ! "$attempts" =~ ^[1-9][0-9]?$ || ! "$delay" =~ ^(0|[1-9][0-9]?)$ ]] || (( attempts > 30 || delay > 30 )); then
  echo "Invalid bounded signature verification retry configuration" >&2
  exit 2
fi
output=$(mktemp)
trap 'rm -f "$output"' EXIT
for (( attempt=1; attempt<=attempts; attempt++ )); do
  if cosign verify --certificate-oidc-issuer https://token.actions.githubusercontent.com \
      --certificate-identity "$identity" "$image" > "$output" 2>&1; then
    cat "$output"
    exit 0
  else
    code=$?
  fi
  # Wrong identity, invalid certificates, missing log proofs and other errors fail immediately.
  if (( code != 10 || attempt == attempts )) || ! grep -qi 'no signatures found' "$output"; then
    cat "$output" >&2
    exit "$code"
  fi
  echo "Signature not visible in registry yet ($attempt/$attempts); retrying after ${delay}s" >&2
  sleep "$delay"
done
