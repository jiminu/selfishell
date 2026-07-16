#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/tests/test_helper.bash"

test_increments_stable_patch_version() {
  local version
  version="$(bash "$ROOT_DIR/scripts/next-patch-version.sh" --current 1.9.41)"
  [[ "$version" == 1.9.42 ]] || fail "Expected 1.9.42, got $version"
}

test_rejects_prerelease_as_patch_base() {
  local status
  set +e
  bash "$ROOT_DIR/scripts/next-patch-version.sh" --current 1.0.0-beta.1 >/dev/null 2>&1
  status=$?
  set -e
  [[ "$status" -ne 0 ]] || fail "A prerelease should not be used as the stable patch base"
}

test_increments_stable_patch_version
printf 'PASS: test_increments_stable_patch_version\n'
test_rejects_prerelease_as_patch_base
printf 'PASS: test_rejects_prerelease_as_patch_base\n'
