#!/usr/bin/env bash

set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$ROOT_DIR"
export GOTOOLCHAIN=local CGO_ENABLED=0

# Build first so the pinned toolchain is checked before any Go command can fetch.
bash scripts/build-cli.sh --all
# Native tests must not inherit a caller's cross-compilation target.
GOOS="$(go env GOHOSTOS)"
GOARCH="$(go env GOHOSTARCH)"
export GOOS GOARCH
formatting="$(gofmt -l cmd internal)"
if [[ -n "$formatting" ]]; then
  printf 'Go files need gofmt:\n%s\n' "$formatting" >&2
  exit 1
fi
go vet ./...
go test ./... -count=1
python3 -B tests/fixtures/go_migration/foundation.py "$ROOT_DIR/.build/selfishell"
python3 -B tests/fixtures/go_migration/foundation.py "$ROOT_DIR/.build/targets/$GOOS-$GOARCH/bin/selfishell"
bash tests/go_migration_test.bash --phase config
