#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/tests/test_helper.bash"
source "$ROOT_DIR/lib/common.sh"

test_semantic_version_validation() {
  local version
  local valid_versions=(
    0.0.0
    1.2.3
    1.2.3-alpha
    1.2.3-alpha.1
    1.2.3-0.3.7
    1.2.3-x.7.z-92
    1.2.3-01alpha
  )
  local invalid_versions=(
    v1.2.3
    01.2.3
    1.02.3
    1.2.03
    1.2
    1.2.3-
    1.2.3-alpha..1
    1.2.3-alpha_1
    1.2.3-01
    1.2.3-alpha.01
    1.2.3+build
  )

  for version in "${valid_versions[@]}"; do
    selfishell_version_is_valid "$version" || fail "Valid semantic version was rejected: $version"
  done
  for version in "${invalid_versions[@]}"; do
    ! selfishell_version_is_valid "$version" || fail "Invalid semantic version was accepted: $version"
  done
}

test_release_scripts_share_version_validation() {
  local before output status
  setup_test_home
  before="$(<"$ROOT_DIR/VERSION")"

  output="$(bash "$ROOT_DIR/scripts/next-patch-version.sh" --current 1.2.3)"
  [[ "$output" == 1.2.4 ]] || fail "Stable patch version was not incremented"

  set +e
  bash "$ROOT_DIR/scripts/next-patch-version.sh" --current 01.2.3 >/dev/null 2>&1
  status=$?
  set -e
  [[ "$status" -ne 0 ]] || fail "Patch version helper accepted a leading zero"

  set +e
  bash "$ROOT_DIR/scripts/release-check.sh" 1.2.3-alpha.01 >/dev/null 2>&1
  status=$?
  set -e
  [[ "$status" -eq 2 ]] || fail "Release check accepted an invalid prerelease"

  set +e
  bash "$ROOT_DIR/scripts/build-release.sh" --version 1.2.3-alpha..1 \
    --output "$TEST_ROOT/dist" --no-update-source >/dev/null 2>&1
  status=$?
  set -e
  [[ "$status" -eq 2 ]] || fail "Release builder accepted an invalid prerelease"
  [[ ! -e "$TEST_ROOT/dist" ]] || fail "Invalid release build created output"
  [[ "$(<"$ROOT_DIR/VERSION")" == "$before" ]] || fail "Invalid release build changed VERSION"
  teardown_test_home
}

test_semantic_version_validation
printf 'PASS: test_semantic_version_validation\n'
test_release_scripts_share_version_validation
printf 'PASS: test_release_scripts_share_version_validation\n'
