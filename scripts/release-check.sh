#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/lib/common.sh"

version="${1:-}"
output_dir="${2:-$ROOT_DIR/dist}"

selfishell_version_is_valid "$version" || {
  printf 'Usage: scripts/release-check.sh VERSION [OUTPUT_DIR]\n' >&2
  exit 2
}

bash "$ROOT_DIR/scripts/check.sh"
bash "$ROOT_DIR/scripts/build-release.sh" --version "$version" --output "$output_dir"

printf 'Release candidate %s passed verification.\n' "$version"
