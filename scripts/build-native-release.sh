#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$ROOT_DIR"
export GOTOOLCHAIN=local GOFLAGS='' GOENV=off GOWORK=off GOOS='' GOARCH='' CGO_ENABLED=0 GOAMD64=v1 GOARM64=v8.0 GOEXPERIMENT=none GOPROXY=off GOSUMDB=off
required="$(awk '$1 == "go" { print $2 }' go.mod)"
if ! command -v go >/dev/null 2>&1 || [[ "$(go env GOVERSION 2>/dev/null || :)" != "go$required" ]]; then
  printf 'Building the native release requires Go %s on PATH.\n' "$required" >&2
  exit 1
fi
staging_root="$(mktemp -d "${TMPDIR:-/tmp}/selfishell-release-builder.XXXXXX")"
trap 'rm -rf "$staging_root"' EXIT HUP INT TERM
go build -trimpath -buildvcs=false -o "$staging_root/builder" ./cmd/selfishell-release
"$staging_root/builder" "$ROOT_DIR" "$@"
