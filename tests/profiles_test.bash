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
  unset XDG_CONFIG_HOME XDG_STATE_HOME
  unset SELFISHELL_TEST_SYSTEM_NAME SELFISHELL_TEST_MACHINE_ARCH
  unset SELFISHELL_TEST_OS_RELEASE_FILE SELFISHELL_TEST_PROC_VERSION_FILE
  teardown_test_home
}

run_profile_dry_run() {
  local output
  output="$(bash "$ROOT_DIR/bin/selfishell" install --profile "$1" --dry-run)"
  printf '%s\n' "$output" | awk '/^Would (install .* (apt packages|Homebrew)|sync .* (direct package|mise tools))/'
}

test_default_profile_is_developer() {
  local output
  output="$(bash "$ROOT_DIR/bin/selfishell" install --dry-run)"

  [[ "$output" == *'vim'* && "$output" == *'starship'* ]] ||
    fail "Default install omitted the minimal profile foundation"
  [[ "$output" == *'direct package: mise'* && "$output" == *'required mise tools:'* ]] ||
    fail "Default install did not select the developer profile"
  [[ "$output" == *'Neovim plugins'* ]] ||
    fail "Default install omitted the developer Neovim setup"
}

test_minimal_includes_shell_tools_and_excludes_larger_profiles() {
  local output full_output
  output="$(run_profile_dry_run minimal)"
  full_output="$(bash "$ROOT_DIR/bin/selfishell" install --profile minimal --dry-run)"

  [[ "$output" == *'zsh git curl ca-certificates vim'* ]] ||
    fail "Minimal required apt packages are incomplete"
  [[ "$output" != *'optional apt packages:'* ]] ||
    fail "Minimal profile should not have optional packages"
  [[ "$output" != *'fzf'* && "$output" != *'zoxide'* && "$output" != *'ripgrep'* ]] ||
    fail "Minimal profile should not include advanced shell tools"
  [[ "$output" == *'direct package: starship'* ]] || fail "Minimal profile is missing Starship"
  [[ "$output" == *'direct package: zinit'* ]] || fail "Minimal profile is missing Zinit"
  [[ "$output" != *'jq'* ]] || fail "Minimal profile included developer JSON tooling"
  [[ "$output" != *'direct package: mise'* ]] || fail "Minimal profile included developer runtimes"
  [[ "$output" != *'build-essential'* ]] || fail "Minimal profile included compiler tooling"
  [[ "$full_output" != *'Neovim plugins'* ]] || fail "Minimal profile included Neovim plugin setup"
}

test_developer_includes_development_tools() {
  local output full_output
  local expected_mise_tools actual_mise_tools

  output="$(run_profile_dry_run developer)"
  full_output="$(bash "$ROOT_DIR/bin/selfishell" install --profile developer --dry-run)"
  # profiles/developer.conf declares developer-profile membership, while
  # common/mise.toml is the sole source of truth for exact versions; the
  # tool name appearing in both is intentional (different
  # responsibilities), not duplication to collapse into one file. Expected
  # tools come from mise.toml rather than profiles/developer.conf itself,
  # since the latter would make this a self-referential check against the
  # very file that also produces $output. Compared as a sorted set rather
  # than the literal dry-run line, since profiles/developer.conf's package
  # order doesn't match mise.toml's [tools] order -- this still catches a
  # tool missing from either file or an extra tool only
  # profiles/developer.conf declares.
  expected_mise_tools="$(awk '
    /^\[/ { in_tools = ($0 == "[tools]"); next }
    in_tools && NF >= 3 { print $1 }
  ' "$ROOT_DIR/common/mise.toml" | sort)"
  actual_mise_tools="$(printf '%s\n' "$output" |
    sed -n 's/^Would sync required mise tools: //p' | tr ' ' '\n' | sort)"

  [[ "$output" == *'required apt packages: zsh git curl ca-certificates vim fzf zoxide ripgrep jq build-essential'* ]] ||
    fail "Developer profile required apt packages are incomplete"
  [[ "$output" == *'optional apt packages: eza bat'* ]] ||
    fail "Developer profile optional apt packages are incomplete"
  [[ "$output" == *'direct package: mise'* ]] ||
    fail "Developer profile is missing development tools"
  [[ -n "$actual_mise_tools" ]] || fail "Developer profile did not report required mise tools"
  [[ "$actual_mise_tools" == "$expected_mise_tools" ]] ||
    fail "Developer profile mise tools do not match common/mise.toml (expected: $expected_mise_tools; got: $actual_mise_tools)"
  [[ "$full_output" == *'Neovim plugins'* ]] || fail "Developer profile is missing Neovim plugin setup"
}

test_minimal_macos_includes_fonts_and_opt_in_ghostty() {
  local output
  export SELFISHELL_TEST_SYSTEM_NAME=Darwin
  output="$(bash "$ROOT_DIR/bin/selfishell" install --profile minimal --dry-run)"

  [[ "$output" == *'optional Homebrew cask: font-meslo-lg-nerd-font font-noto-sans-cjk-kr'* ]] ||
    fail "Minimal macOS profile is missing fonts"
  [[ "$output" == *'optional Homebrew cask: ghostty'* ]] ||
    fail "Ghostty was not included in the macOS dry run"
}

test_unknown_profile_returns_usage_error() {
  local status

  set +e
  bash "$ROOT_DIR/bin/selfishell" install --profile unknown --dry-run >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -eq 2 ]] || fail "Unknown profile should return exit code 2"
}

test_profile_read_file_rejects_option_like_package() {
  local profile_file="$TEST_ROOT/profile-record.conf"
  local status

  # profile_read_file() is the shared parser built-in profiles use too;
  # exercise it directly rather than through a CLI entry point.
  printf 'package ubuntu required apt --allow-unauthenticated\n' >"$profile_file"

  set +e
  bash -c '
    source "$1/lib/common.sh"
    source "$1/lib/profiles.sh"
    profile_read_file "$2"
  ' _ "$ROOT_DIR" "$profile_file" >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -eq 2 ]] || fail "Option-like package name should be rejected"
}

run_discovered_tests setup_profile_home teardown_profile_home
