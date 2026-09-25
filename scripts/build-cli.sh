#!/usr/bin/env bash

set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$ROOT_DIR"

case "${*:-}" in
  '' | --all) ;;
  *)
    printf 'Usage: bash scripts/build-cli.sh [--all]\n' >&2
    exit 2
    ;;
esac

required="$(awk '$1 == "go" { print $2 }' go.mod)"
export GOTOOLCHAIN=local
if ! command -v go >/dev/null 2>&1 || [[ "$(go env GOVERSION)" != "go$required" ]]; then
  printf 'Building the development CLI requires Go %s on PATH.\n' "$required" >&2
  exit 1
fi
export CGO_ENABLED=0 GOAMD64=v1 GOARM64=v8.0
host_os="$(go env GOHOSTOS)"
host_arch="$(go env GOHOSTARCH)"
case "$host_os-$host_arch" in
  darwin-amd64 | darwin-arm64 | linux-amd64 | linux-arm64) ;;
  *)
    printf 'Unsupported development host: %s/%s\n' "$host_os" "$host_arch" >&2
    exit 1
    ;;
esac

mkdir -p .build
GOOS="$host_os" GOARCH="$host_arch" go build -trimpath -o .build/selfishell ./cmd/selfishell
printf 'Built native development CLI: .build/selfishell (%s/%s)\n' "$host_os" "$host_arch"

if [[ "${1:-}" == --all ]]; then
  for target_os in darwin linux; do
    for target_arch in amd64 arm64; do
      target=".build/targets/$target_os-$target_arch/bin/selfishell"
      mkdir -p "${target%/*}"
      if [[ "$target_os-$target_arch" == "$host_os-$host_arch" ]]; then
        cp .build/selfishell "$target"
      else
        GOOS="$target_os" GOARCH="$target_arch" go build -trimpath -o "$target" ./cmd/selfishell
      fi
      printf 'Cross-build: %s/%s\n' "$target_os" "$target_arch"
    done
  done
fi
