#!/usr/bin/env bash

set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$ROOT_DIR"
source "$ROOT_DIR/scripts/go-env.sh"
selfishell_prepare_go "$ROOT_DIR"

# Build first so the pinned toolchain is checked before any Go command can fetch.
bash scripts/build-cli.sh --all
# Native tests must not inherit a caller's cross-compilation target.
GOOS="$(go env GOHOSTOS)"
GOARCH="$(go env GOHOSTARCH)"
export GOOS GOARCH
formatting="$(gofmt -l cmd internal tests/integration)"
if [[ -n "$formatting" ]]; then
  printf 'Go files need gofmt:\n%s\n' "$formatting" >&2
  exit 1
fi
go vet ./...
# Fixture builds otherwise start from an empty cache on every run. Keep a warm
# one in the checkout, never in the developer's HOME; CI passes its own.
test_go_cache="${SELFISHELL_TEST_GO_CACHE:-$ROOT_DIR/.build/test-go-cache}"
mkdir -p "$test_go_cache"
SELFISHELL_TEST_CLI="$ROOT_DIR/.build/selfishell" SELFISHELL_TEST_GO_CACHE="$test_go_cache" go test ./... -count=1
