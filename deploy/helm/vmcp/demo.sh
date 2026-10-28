#!/usr/bin/env bash
set -euo pipefail
command -v python3 >/dev/null 2>&1 || { echo 'demo.sh requires Python 3.9+.' >&2; exit 1; }
if [[ "${1:-}" == help ]]; then
  shift
  set -- --help "$@"
fi
exec python3 "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/quickstart/demo.py" "$@"
