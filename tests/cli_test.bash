#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

source "$ROOT_DIR/tests/test_helper.bash"

test_help_is_default_command() {
  local output
  output="$(bash "$ROOT_DIR/bin/selfishell")"

  [[ "$output" == *'Usage:'* ]] || fail "Default command should show help"
  [[ "$output" == *'selfishell <command>'* ]] || fail "Help should use the canonical command"
}

test_version_reads_version_file() {
  local expected
  local output

  expected="$(<"$ROOT_DIR/VERSION")"
  output="$(bash "$ROOT_DIR/bin/selfishell" version)"
  [[ "$output" == "selfishell $expected" ]] || fail "Unexpected version output: $output"
}

test_sfs_runs_same_cli() {
  local canonical
  local shorthand

  canonical="$(bash "$ROOT_DIR/bin/selfishell" version)"
  shorthand="$(bash "$ROOT_DIR/bin/sfs" version)"
  [[ "$shorthand" == "$canonical" ]] || fail "sfs must invoke the canonical CLI"
}

test_cli_resolves_external_symlink() {
  local output

  setup_test_home
  mkdir -p "$TEST_ROOT/bin"
  ln -s "$ROOT_DIR/bin/selfishell" "$TEST_ROOT/bin/selfishell"
  output="$(bash "$TEST_ROOT/bin/selfishell" version)"
  teardown_test_home

  [[ "$output" == "$(bash "$ROOT_DIR/bin/selfishell" version)" ]] ||
    fail "CLI should resolve its release root through an external symlink"
}

test_unknown_command_returns_usage_error() {
  local output
  local status

  set +e
  output="$(bash "$ROOT_DIR/bin/selfishell" unknown 2>&1)"
  status=$?
  set -e

  [[ "$status" -eq 2 ]] || fail "Unknown command should return exit code 2"
  [[ "$output" == *'Unknown command: unknown'* ]] || fail "Missing unknown command error"
}

test_doctor_rejects_unsupported_platform() {
  local output
  local status

  setup_test_home
  printf 'ID=fedora\n' >"$TEST_ROOT/os-release"
  printf 'Linux version 6.8.0\n' >"$TEST_ROOT/proc-version"

  set +e
  output="$(
    SELFISHELL_TEST_SYSTEM_NAME=Linux \
      SELFISHELL_TEST_MACHINE_ARCH=x86_64 \
      SELFISHELL_TEST_OS_RELEASE_FILE="$TEST_ROOT/os-release" \
      SELFISHELL_TEST_PROC_VERSION_FILE="$TEST_ROOT/proc-version" \
      bash "$ROOT_DIR/bin/selfishell" doctor 2>&1
  )"
  status=$?
  set -e

  teardown_test_home
  [[ "$status" -eq 1 ]] || fail "Unsupported platform should return exit code 1"
  [[ "$output" == *'Ubuntu is the only supported native Linux distribution.'* ]] ||
    fail "Doctor should provide an actionable platform message"
}

test_commands_reject_extra_arguments() {
  local status

  set +e
  bash "$ROOT_DIR/bin/selfishell" version extra >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -eq 2 ]] || fail "Extra arguments should return exit code 2"
}

test_main_is_install_compatibility_wrapper() {
  local output
  local status

  setup_test_home
  printf 'ID=fedora\n' >"$TEST_ROOT/os-release"
  printf 'Linux version 6.8.0\n' >"$TEST_ROOT/proc-version"

  set +e
  output="$(
    SELFISHELL_TEST_SYSTEM_NAME=Linux \
      SELFISHELL_TEST_MACHINE_ARCH=x86_64 \
      SELFISHELL_TEST_OS_RELEASE_FILE="$TEST_ROOT/os-release" \
      SELFISHELL_TEST_PROC_VERSION_FILE="$TEST_ROOT/proc-version" \
      bash "$ROOT_DIR/main.sh" 2>&1
  )"
  status=$?
  set -e

  teardown_test_home
  [[ "$status" -eq 1 ]] || fail "Compatibility wrapper should preserve install failure"
  [[ "$output" == *"Run 'selfishell doctor'"* ]] ||
    fail "main.sh should dispatch through the Selfishell CLI"
}

run_test() {
  local test_name="$1"

  "$test_name"
  printf 'PASS: %s\n' "$test_name"
}

main() {
  local test_name
  local failures=0

  while IFS= read -r test_name; do
    if ! run_test "$test_name"; then
      failures=$((failures + 1))
    fi
  done < <(declare -F | awk '{print $3}' | grep '^test_' | sort)

  if ((failures > 0)); then
    printf '%d test(s) failed\n' "$failures" >&2
    return 1
  fi
}

main "$@"
