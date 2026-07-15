#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

source "$ROOT_DIR/tests/test_helper.bash"

setup_release_home() {
  local version

  setup_test_home
  version="$(<"$ROOT_DIR/VERSION")"
  export SELFISHELL_RELEASE_ROOT="file://$TEST_ROOT/releases"
  export SELFISHELL_BOOTSTRAP_OS=Linux
  export SELFISHELL_BOOTSTRAP_ARCH=x86_64
  export XDG_CONFIG_HOME="$HOME/.config"
  export XDG_STATE_HOME="$HOME/.local/state"
  export SELFISHELL_TEST_SYSTEM_NAME=Linux
  export SELFISHELL_TEST_MACHINE_ARCH=x86_64
  export SELFISHELL_TEST_OS_RELEASE_FILE="$TEST_ROOT/os-release"
  export SELFISHELL_TEST_PROC_VERSION_FILE="$TEST_ROOT/proc-version"
  printf 'ID=ubuntu\n' >"$SELFISHELL_TEST_OS_RELEASE_FILE"
  printf 'Linux version 6.8.0\n' >"$SELFISHELL_TEST_PROC_VERSION_FILE"

  mkdir -p "$TEST_ROOT/artifacts" "$TEST_ROOT/releases/download/v$version" "$TEST_ROOT/releases/latest/download"
  bash "$ROOT_DIR/scripts/build-release.sh" --version "$version" --output "$TEST_ROOT/artifacts" >/dev/null
  cp "$TEST_ROOT/artifacts"/* "$TEST_ROOT/releases/download/v$version/"
  cp "$TEST_ROOT/artifacts/VERSION" "$TEST_ROOT/releases/latest/download/VERSION"
}

teardown_release_home() {
  unset SELFISHELL_RELEASE_ROOT SELFISHELL_BOOTSTRAP_OS SELFISHELL_BOOTSTRAP_ARCH
  unset XDG_CONFIG_HOME XDG_STATE_HOME SELFISHELL_OFFLINE
  unset SELFISHELL_TEST_SYSTEM_NAME SELFISHELL_TEST_MACHINE_ARCH
  unset SELFISHELL_TEST_OS_RELEASE_FILE SELFISHELL_TEST_PROC_VERSION_FILE
  teardown_test_home
}

run_bootstrap() {
  bash "$ROOT_DIR/install.sh" --prefix "$TEST_ROOT/prefix" "$@"
}

test_builds_all_platform_architecture_artifacts() {
  local version
  version="$(<"$ROOT_DIR/VERSION")"

  for artifact in \
    "selfishell-$version-linux-amd64.tar.gz" \
    "selfishell-$version-linux-arm64.tar.gz" \
    "selfishell-$version-macos-amd64.tar.gz" \
    "selfishell-$version-macos-arm64.tar.gz"; do
    [[ -f "$TEST_ROOT/artifacts/$artifact" ]] || fail "Missing release artifact: $artifact"
  done
  [[ -s "$TEST_ROOT/artifacts/SHA256SUMS" ]] || fail "Missing release checksums"
}

test_installs_exact_version_and_cli_links() {
  local version
  version="$(<"$ROOT_DIR/VERSION")"

  run_bootstrap --version "$version" >/dev/null

  assert_symlink_to "releases/$version" "$TEST_ROOT/prefix/share/selfishell/current"
  assert_symlink_to "$TEST_ROOT/prefix/share/selfishell/current/bin/selfishell" "$TEST_ROOT/prefix/bin/selfishell"
  assert_symlink_to selfishell "$TEST_ROOT/prefix/bin/sfs"
  [[ "$("$TEST_ROOT/prefix/bin/selfishell" version)" == "selfishell $version" ]] ||
    fail "Installed CLI reports the wrong version"
}

test_latest_uses_published_version_file() {
  local version
  version="$(<"$ROOT_DIR/VERSION")"

  run_bootstrap >/dev/null
  [[ "$(<"$TEST_ROOT/prefix/share/selfishell/current/VERSION")" == "$version" ]] ||
    fail "Latest installation selected the wrong version"
}

test_checksum_mismatch_preserves_active_release() {
  local version
  local archive
  local active_before
  local status

  version="$(<"$ROOT_DIR/VERSION")"
  archive="$TEST_ROOT/releases/download/v$version/selfishell-$version-linux-amd64.tar.gz"
  run_bootstrap --version "$version" >/dev/null
  active_before="$(readlink "$TEST_ROOT/prefix/share/selfishell/current")"
  printf 'corruption' >>"$archive"

  set +e
  run_bootstrap --version "$version" >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -eq 1 ]] || fail "Checksum mismatch should fail"
  [[ "$(readlink "$TEST_ROOT/prefix/share/selfishell/current")" == "$active_before" ]] ||
    fail "Checksum failure changed the active release"
}

test_specific_version_never_falls_back_to_latest() {
  local status

  set +e
  run_bootstrap --version 9.9.9 >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -ne 0 ]] || fail "Missing exact version should fail"
  [[ ! -e "$TEST_ROOT/prefix/share/selfishell/current" ]] ||
    fail "Exact version failure unexpectedly installed latest"
}

test_bootstrap_installs_cli_only_by_default() {
  run_bootstrap >/dev/null

  [[ -x "$TEST_ROOT/prefix/bin/selfishell" ]] || fail "CLI was not installed"
  [[ ! -e "$XDG_CONFIG_HOME/selfishell" ]] || fail "Bootstrap changed user configuration"
}

test_setup_is_explicit_and_can_run_offline() {
  export SELFISHELL_OFFLINE=1
  run_bootstrap --setup --yes >/dev/null

  assert_symlink_to "$XDG_CONFIG_HOME/selfishell/zsh/zshrc" "$HOME/.zshrc"
}

test_missing_bin_path_prints_actionable_message() {
  local output
  output="$(PATH=/usr/bin:/bin run_bootstrap)"

  [[ "$output" == *"Add $TEST_ROOT/prefix/bin to PATH"* ]] ||
    fail "Missing PATH guidance was not printed"
}

test_refuses_to_replace_non_link_cli_path() {
  local status

  mkdir -p "$TEST_ROOT/prefix/bin"
  printf 'user file' >"$TEST_ROOT/prefix/bin/selfishell"
  set +e
  run_bootstrap >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -eq 1 ]] || fail "Non-link CLI path should block installation"
  assert_file_content 'user file' "$TEST_ROOT/prefix/bin/selfishell"
  [[ ! -e "$TEST_ROOT/prefix/share/selfishell/current" ]] ||
    fail "Link preflight failure changed the active release"
}

run_test() {
  local test_name="$1"

  setup_release_home
  trap 'teardown_release_home' RETURN
  "$test_name"
  trap - RETURN
  teardown_release_home
  printf 'PASS: %s\n' "$test_name"
}

main() {
  local test_name

  while IFS= read -r test_name; do
    run_test "$test_name"
  done < <(declare -F | awk '{print $3}' | grep '^test_' | sort)
}

main "$@"
