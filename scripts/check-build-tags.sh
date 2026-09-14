#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

check_tmp="$(mktemp -d)"
trap 'rm -rf "$check_tmp"' EXIT

# An unrelated build error must not make a missing guard pass this check.
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./internal/devmock/
check_guard() {
  local target="$1" tags="$2" arch="$3" cgo="$4"
  if GOOS="$target" GOARCH="$arch" CGO_ENABLED="$cgo" go build -tags "$tags" ./internal/devmock/ >"$check_tmp/output" 2>&1; then
    echo "devmock unexpectedly compiled for $target with tags $tags" >&2
    exit 1
  fi
  if ! grep -q 'undefined: devmockMustNotBeCompiledIntoWindowsOrProductionBuilds' "$check_tmp/output"; then
    cat "$check_tmp/output" >&2
    echo "Build failed for a reason other than the devmock guard" >&2
    exit 1
  fi
}
check_guard windows devmock amd64 0
# Production must be checked on a non-Windows host, so the Windows guard
# cannot conceal a broken production guard. Wails needs native cgo here.
host="$(go env GOHOSTOS)"
if [ "$host" != windows ]; then
  check_guard "$host" production,devmock "$(go env GOHOSTARCH)" 1
fi
