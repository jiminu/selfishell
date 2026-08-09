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

# This is a gate over an already-decided release candidate, not a place to
# pick the version: fail before running the (expensive) full check.sh gate
# rather than let build-release.sh silently rewrite the source VERSION file
# to match a mismatched request.
source_version="$(tr -d '[:space:]' <"$ROOT_DIR/VERSION")"
if [[ "$version" != "$source_version" ]]; then
  printf 'VERSION mismatch: requested %s, but VERSION contains %s.\n' "$version" "$source_version" >&2
  printf 'Update VERSION before running the release check.\n' >&2
  exit 1
fi

bash "$ROOT_DIR/scripts/check.sh"
bash "$ROOT_DIR/scripts/build-release.sh" --version "$version" --output "$output_dir" --no-update-source

printf 'Release candidate %s passed verification.\n' "$version"
