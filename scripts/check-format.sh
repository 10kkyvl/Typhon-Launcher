#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

# Include new product files, but not ignored local fixtures or deleted files.
out="$(git ls-files -z --cached --others --exclude-standard -- '*.go' |
  while IFS= read -r -d '' path; do
    if [ -f "$path" ]; then printf '%s\0' "$path"; fi
  done | xargs -0 gofmt -l)"
if [ -n "$out" ]; then
  printf 'gofmt needs to run on:\n%s\n' "$out" >&2
  exit 1
fi
