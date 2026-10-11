#!/usr/bin/env bash
# Render docs/ for GitHub Wiki. Usage: scripts/prepare-wiki.sh <output-dir>
set -euo pipefail
exec python3 "$(dirname "$0")/prepare-wiki.py" "${1:?usage: prepare-wiki.sh <output-dir>}"
