#!/usr/bin/env bash
# Expensive checks run here before push, or in CI's manual full mode.
set -euo pipefail
cd "$(dirname "$0")/.."
export PATH="$(go env GOPATH)/bin:$PATH"

mode="${1:-all}"
if [ "$mode" != all ] && [ "$mode" != lint ]; then
  echo "Usage: bash scripts/check-local.sh [all|lint]" >&2
  exit 2
fi
host="$(go env GOHOSTOS)"
if [ "$(go env GOOS)" != "$host" ]; then
  echo "Run local checks with the host GOOS; Windows analysis is selected below." >&2
  exit 2
fi
if [ "$mode" = all ] && [ "$host" != darwin ]; then
  echo "Full local checks require macOS. Use lint here, and CI for Windows execution." >&2
  exit 2
fi
if ! golangci-lint --version | grep -q 'version 2.13.1 '; then
  echo "Install the pinned linter with: wails3 task lint:install" >&2
  exit 1
fi

bash scripts/check-format.sh
go vet . ./internal/... ./cmd/... ./tools/...

check_tmp="$(mktemp -d)"
trap 'rm -rf "$check_tmp"' EXIT
baseline="$check_tmp/lintbaseline"
if [ "$host" = windows ]; then baseline="$baseline.exe"; fi
go build -o "$baseline" ./tools/lintbaseline
"$baseline"
if [ "$host" != windows ]; then
  "$baseline" -tags devmock
  # Run a native helper; GOOS selects the linter's Windows Go files.
  GOOS=windows GOARCH=amd64 CGO_ENABLED=0 "$baseline" -goos windows
fi
if [ "$host" = linux ]; then
  # Linux retains historical findings; additionally reject new findings.
  lint_base="${LINT_BASE:-origin/main}"
  git rev-parse --verify "$lint_base^{commit}" >/dev/null
  golangci-lint run --new-from-rev="$lint_base" --whole-files=false ./...
  golangci-lint run --new-from-rev="$lint_base" --whole-files=false --build-tags devmock ./internal/...
fi
if [ "$mode" = lint ]; then exit 0; fi

GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o "$check_tmp/typhon.exe" .
bash scripts/check-build-tags.sh
go build -o /dev/null .
go build -tags devmock -o /dev/null .
CGO_ENABLED=1 go test -race ./...
CGO_ENABLED=1 go test -race -tags devmock ./...
wails3 task darwin:package:zip
npm --prefix frontend run check
npm --prefix frontend test
