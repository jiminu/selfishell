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

test_version_available_reads_release_metadata() {
  local release_root output

  setup_test_home
  release_root="$TEST_ROOT/releases"
  mkdir -p "$release_root/latest/download"
  printf '1.2.3\n' >"$release_root/latest/download/VERSION"

  output="$(SELFISHELL_RELEASE_ROOT="file://$release_root" bash "$ROOT_DIR/bin/selfishell" version --available)"

  [[ "$output" == 1.2.3 ]] || fail "Available release version was not reported"
  teardown_test_home
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

test_removed_self_update_command_is_rejected() {
  local output
  local status

  set +e
  output="$(bash "$ROOT_DIR/bin/selfishell" self-update 2>&1)"
  status=$?
  set -e

  [[ "$status" -eq 2 ]] || fail "Removed self-update command should return exit code 2"
  [[ "$output" == *'Unknown command: self-update'* ]] ||
    fail "Removed self-update command should be reported as unknown"
}

test_removed_legacy_install_command_is_rejected() {
  local status

  set +e
  bash "$ROOT_DIR/bin/selfishell" legacy-install >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -eq 2 ]] || fail "Removed legacy-install command should return usage error"
}

test_update_rejects_conflicting_scopes() {
  local status

  set +e
  bash "$ROOT_DIR/bin/selfishell" update --cli-only --tools-only >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -eq 2 ]] || fail "Conflicting update scopes should return exit code 2"
}

test_update_rejects_version_for_tools_only() {
  local status

  set +e
  bash "$ROOT_DIR/bin/selfishell" update --tools-only --version 1.0.0 >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -eq 2 ]] || fail "Tools-only version selection should return exit code 2"
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

test_doctor_reports_preserved_legacy_runtime_managers() {
  local output

  setup_test_home
  mkdir -p "$HOME/.nvm" "$HOME/.pyenv" "$HOME/.local/state/selfishell" "$TEST_ROOT/bin"
  printf 'developer\n' >"$HOME/.local/state/selfishell/profile"
  printf 'ID=ubuntu\n' >"$TEST_ROOT/os-release"
  printf 'Linux version 6.8.0\n' >"$TEST_ROOT/proc-version"
  printf '#!/usr/bin/env bash\nexit 0\n' >"$TEST_ROOT/bin/apt"
  chmod +x "$TEST_ROOT/bin/apt"

  set +e
  output="$(
    PATH="$TEST_ROOT/bin:/usr/bin:/bin" \
      SELFISHELL_TEST_SYSTEM_NAME=Linux \
      SELFISHELL_TEST_MACHINE_ARCH=x86_64 \
      SELFISHELL_TEST_OS_RELEASE_FILE="$TEST_ROOT/os-release" \
      SELFISHELL_TEST_PROC_VERSION_FILE="$TEST_ROOT/proc-version" \
      bash "$ROOT_DIR/bin/selfishell" doctor 2>&1
  )"
  set -e

  [[ "$output" == *"Legacy runtime manager detected: $HOME/.nvm (preserved; mise is active)"* ]] ||
    fail "Doctor did not report preserved NVM data"
  [[ "$output" == *"Legacy runtime manager detected: $HOME/.pyenv (preserved; mise is active)"* ]] ||
    fail "Doctor did not report preserved pyenv data"
  teardown_test_home
}

test_commands_reject_extra_arguments() {
  local status

  set +e
  bash "$ROOT_DIR/bin/selfishell" version extra >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -eq 2 ]] || fail "Extra arguments should return exit code 2"
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
