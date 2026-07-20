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

test_update_readme_version_script() {
  local temp_dir
  temp_dir="$(mktemp -d "${TMPDIR:-/tmp}/selfishell-test-readme-version.XXXXXX")"
  trap 'rm -rf "$temp_dir"' RETURN

  mkdir -p "$temp_dir/docs"
  echo "0.3.5" >"$temp_dir/VERSION"
  echo "curl -fsSL https://raw.githubusercontent.com/jiminu/selfishell/v0.3.0/install.sh | bash" >"$temp_dir/README.md"
  echo "bash -s -- --version 0.3.0" >>"$temp_dir/README.md"
  echo "curl -fsSL https://raw.githubusercontent.com/jiminu/selfishell/v0.3.0/install.sh | bash" >"$temp_dir/docs/INSTALLATION.md"
  echo "bash -s -- --version 0.3.0" >>"$temp_dir/docs/INSTALLATION.md"

  SELFISHELL_TEST_ROOT="$temp_dir" bash "$ROOT_DIR/scripts/update-readme-version.sh" >/dev/null

  # Verify correct replacement
  if ! grep -q "selfishell/v0.3.5/install.sh" "$temp_dir/README.md"; then
    fail "README.md URL not updated to 0.3.5"
  fi
  if ! grep -q -e "--version 0.3.5" "$temp_dir/README.md"; then
    fail "README.md --version arg not updated to 0.3.5"
  fi
  if ! grep -q "selfishell/v0.3.5/install.sh" "$temp_dir/docs/INSTALLATION.md"; then
    fail "docs/INSTALLATION.md URL not updated to 0.3.5"
  fi
  if ! grep -q -e "--version 0.3.5" "$temp_dir/docs/INSTALLATION.md"; then
    fail "docs/INSTALLATION.md --version arg not updated to 0.3.5"
  fi
}

test_increments_stable_patch_version
printf 'PASS: test_increments_stable_patch_version\n'
test_rejects_prerelease_as_patch_base
printf 'PASS: test_rejects_prerelease_as_patch_base\n'
test_update_readme_version_script
printf 'PASS: test_update_readme_version_script\n'
