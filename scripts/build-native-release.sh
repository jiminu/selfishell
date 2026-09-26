#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$ROOT_DIR"
source "$ROOT_DIR/lib/common.sh"
version=""
usage() { printf 'Usage: scripts/build-native-release.sh --version VERSION [--output OUTPUT_DIR]\n' >&2; }
args=("$@")
while (("$#" > 0)); do
  case "$1" in
    --version | --output)
      (($# >= 2)) || {
        usage
        exit 2
      }
      [[ "$1" == --version ]] && version="$2"
      shift 2
      ;;
    *)
      printf 'Unknown option: %s\n' "$1" >&2
      usage
      exit 2
      ;;
  esac
done
if ! selfishell_version_is_valid "$version"; then
  printf 'A valid semantic version is required.\n' >&2
  usage
  exit 2
fi
set -- "${args[@]}"
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
