#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

source "$ROOT_DIR/tests/test_helper.bash"

setup_profile_home() {
  setup_test_home
  export XDG_CONFIG_HOME="$HOME/.config"
  export XDG_STATE_HOME="$HOME/.local/state"
  export SELFISHELL_TEST_SYSTEM_NAME=Linux
  export SELFISHELL_TEST_MACHINE_ARCH=x86_64
  export SELFISHELL_TEST_OS_RELEASE_FILE="$TEST_ROOT/os-release"
  export SELFISHELL_TEST_PROC_VERSION_FILE="$TEST_ROOT/proc-version"
  printf 'ID=ubuntu\n' >"$SELFISHELL_TEST_OS_RELEASE_FILE"
  printf 'Linux version 6.8.0\n' >"$SELFISHELL_TEST_PROC_VERSION_FILE"
}

teardown_profile_home() {
  unset XDG_CONFIG_HOME XDG_STATE_HOME SELFISHELL_OFFLINE SELFISHELL_LOCAL_PROFILE
  unset SELFISHELL_TEST_SYSTEM_NAME SELFISHELL_TEST_MACHINE_ARCH
  unset SELFISHELL_TEST_OS_RELEASE_FILE SELFISHELL_TEST_PROC_VERSION_FILE
  teardown_test_home
}

run_profile_dry_run() {
  local output
  output="$(bash "$ROOT_DIR/bin/selfishell" install --profile "$1" --dry-run)"
  printf '%s\n' "$output" | awk '/^Would install .* (apt packages|Homebrew|direct package)/'
}

test_minimal_excludes_developer_and_kubernetes_packages() {
  local output
  output="$(run_profile_dry_run minimal)"

  [[ "$output" == *'zsh git curl ca-certificates'* ]] || fail "Minimal apt packages are incomplete"
  [[ "$output" == *'direct package: starship'* ]] || fail "Minimal profile is missing Starship"
  [[ "$output" != *'fzf'* ]] || fail "Minimal profile included developer tools"
  [[ "$output" != *'kubectl'* ]] || fail "Minimal profile included Kubernetes tools"
}

test_developer_inherits_minimal_only() {
  local output
  output="$(run_profile_dry_run developer)"

  [[ "$output" == *'fzf'* && "$output" == *'direct package: pyenv'* ]] ||
    fail "Developer profile is missing development tools"
  [[ "$output" != *'kubectl'* ]] || fail "Developer profile included Kubernetes tools"
}

test_kubernetes_inherits_developer() {
  local output
  output="$(run_profile_dry_run kubernetes)"

  [[ "$output" == *'fzf'* && "$output" == *'kubectl'* ]] ||
    fail "Kubernetes profile inheritance is incomplete"
  [[ "$output" != *'ghostty'* ]] || fail "Kubernetes profile included full-only tools"
}

test_full_macos_includes_desktop_packages() {
  local output
  export SELFISHELL_TEST_SYSTEM_NAME=Darwin
  output="$(run_profile_dry_run full)"

  [[ "$output" == *'Homebrew cask: ghostty font-meslo-lg-nerd-font font-noto-sans-cjk-kr'* ]] ||
    fail "Full macOS profile is missing desktop packages"
}

test_local_profile_adds_private_package() {
  local output
  local local_profile="$TEST_ROOT/company.conf"

  printf 'package ubuntu optional apt company-cli\n' >"$local_profile"
  output="$(bash "$ROOT_DIR/bin/selfishell" install --profile minimal --local-profile "$local_profile" --dry-run)"

  [[ "$output" == *'optional apt packages: company-cli'* ]] ||
    fail "Local profile package was not included"
}

test_offline_mode_skips_package_operations() {
  local output
  export SELFISHELL_OFFLINE=1
  output="$(bash "$ROOT_DIR/bin/selfishell" install --profile full --dry-run)"

  [[ "$output" == *'Skipping package installation.'* ]] || fail "Offline mode did not skip packages"
  [[ "$output" != *'Would install required apt packages'* ]] ||
    fail "Offline mode attempted package operations"
}

test_unknown_profile_returns_usage_error() {
  local status

  set +e
  bash "$ROOT_DIR/bin/selfishell" install --profile unknown --dry-run >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -eq 2 ]] || fail "Unknown profile should return exit code 2"
}

test_local_profile_rejects_option_like_package() {
  local local_profile="$TEST_ROOT/company.conf"
  local status

  printf 'package ubuntu required apt --allow-unauthenticated\n' >"$local_profile"
  set +e
  bash "$ROOT_DIR/bin/selfishell" install --profile minimal --local-profile "$local_profile" --dry-run >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -eq 2 ]] || fail "Option-like local package should be rejected"
}

run_test() {
  local test_name="$1"

  setup_profile_home
  trap 'teardown_profile_home' RETURN
  "$test_name"
  trap - RETURN
  teardown_profile_home
  printf 'PASS: %s\n' "$test_name"
}

main() {
  local test_name

  while IFS= read -r test_name; do
    run_test "$test_name"
  done < <(declare -F | awk '{print $3}' | grep '^test_' | sort)
}

main "$@"
