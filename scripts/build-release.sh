#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$ROOT_DIR"
source "$ROOT_DIR/scripts/release-version.sh"
version=""
usage() { printf 'Usage: scripts/build-release.sh --version VERSION [--output OUTPUT_DIR]\n' >&2; }
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
source "$ROOT_DIR/scripts/go-env.sh"
selfishell_prepare_go "$ROOT_DIR"
staging_root="$(mktemp -d "${TMPDIR:-/tmp}/selfishell-release-builder.XXXXXX")"
trap 'rm -rf "$staging_root"' EXIT HUP INT TERM
go build -trimpath -buildvcs=false -o "$staging_root/builder" ./cmd/selfishell-release
"$staging_root/builder" "$ROOT_DIR" "$@"
