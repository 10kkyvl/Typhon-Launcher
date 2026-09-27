#!/bin/sh
# Compiles the three helpers into OUT (default: the session scratchpad or ./bin-ui).
set -eu
here="$(cd "$(dirname "$0")" && pwd)"
out="${1:-${CLAUDE_SCRATCHPAD:-$PWD/bin-ui}}"
mkdir -p "$out"
for t in winid click scroll; do
  swiftc -O -o "$out/$t" "$here/$t.swift" 2>&1 | grep -v "^$" || true
done
ls -1 "$out"/winid "$out"/click "$out"/scroll
