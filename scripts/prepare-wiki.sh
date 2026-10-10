#!/usr/bin/env bash
# Copies docs/ into a directory laid out for the GitHub Wiki and rewrites relative
# "Page.md" links to wiki links ("Page"). Usage: scripts/prepare-wiki.sh <output-dir>
set -euo pipefail
out="${1:?usage: prepare-wiki.sh <output-dir>}"
src="$(cd "$(dirname "$0")/../docs" && pwd)"
mkdir -p "$out"
for f in "$src"/*.md; do
  sed -E 's/\]\(([A-Za-z0-9_-]+)\.md(#[^)]*)?\)/](\1\2)/g' "$f" > "$out/$(basename "$f")"
done
echo "wiki pages written to $out"
