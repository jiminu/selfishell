#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

source "$ROOT_DIR/tests/test_helper.bash"
source "$ROOT_DIR/tests/cli_runner.bash"

setup_package_home() {
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

teardown_package_home() {
  unset XDG_CONFIG_HOME XDG_STATE_HOME
  unset SELFISHELL_TEST_SYSTEM_NAME SELFISHELL_TEST_MACHINE_ARCH
  unset SELFISHELL_TEST_OS_RELEASE_FILE SELFISHELL_TEST_PROC_VERSION_FILE
  teardown_test_home
}

run_package_dry_run() {
  local output
  output="$(run_selfishell install --dry-run)"
  printf '%s\n' "$output" | awk '/^Would (install .* (apt packages|Homebrew)|sync .* (direct package|mise tools))/'
}

assert_core_tools() {
  local output="$1"

  grep -Eq '(^|[[:space:]])zsh([[:space:]]|$)' <<<"$output" ||
    fail "Environment omitted the core Zsh environment"
  grep -Eq '(^|[[:space:]])vim([[:space:]]|$)' <<<"$output" ||
    fail "Environment omitted Vim"
  grep -Eq '^Would sync required mise tools:.* starship([[:space:]]|$)' <<<"$output" ||
    fail "Environment omitted mise-managed Starship"
  [[ "$output" != *'direct package: starship'* ]] || fail "Starship still uses the direct installer"
  [[ "$output" == *'direct package: zinit'* ]] || fail "Environment omitted Zinit"
}

test_install_includes_mise_and_neovim() {
  local output
  output="$(run_selfishell install --dry-run)"

  [[ "$output" == *'direct package: mise'* && "$output" == *'required mise tools:'* ]] ||
    fail "Install omitted mise-managed tools"
  [[ "$output" == *'Neovim plugins'* ]] ||
    fail "Install omitted Neovim setup"
}

test_manifest_includes_development_tools() {
  local output full_output macos_output apt_plan homebrew_plan
  local expected_mise_tools actual_mise_tools required_mise_tools optional_mise_tools

  output="$(run_package_dry_run)"
  full_output="$(run_selfishell install --dry-run)"
  # Compare membership against the independently parsed mise.toml; order is irrelevant.
  expected_mise_tools="$(awk '
    /^\[/ { in_tools = ($0 == "[tools]"); next }
    in_tools && NF >= 3 { print $1 }
  ' "$ROOT_DIR/config/shared/mise.toml" | sort)"
  actual_mise_tools="$(printf '%s\n' "$output" |
    sed -E -n 's/^Would sync (required|optional) mise tools: //p' | tr ' ' '\n' | sort)"
  required_mise_tools="$(printf '%s\n' "$output" |
    sed -n 's/^Would sync required mise tools: //p' | tr ' ' '\n' | sort)"
  optional_mise_tools="$(printf '%s\n' "$output" |
    sed -n 's/^Would sync optional mise tools: //p' | tr ' ' '\n' | sort)"

  assert_core_tools "$output"
  [[ "$output" == *'direct package: mise'* ]] ||
    fail "Environment is missing development tools"
  [[ -n "$actual_mise_tools" ]] || fail "Environment did not report required mise tools"
  [[ "$actual_mise_tools" == "$expected_mise_tools" ]] ||
    fail "Environment mise tools do not match config/shared/mise.toml (expected: $expected_mise_tools; got: $actual_mise_tools)"
  [[ "$required_mise_tools" == $'fzf\ngh\njq\nlazygit\nneovim\nnode\npython\nripgrep\nstarship\ntree-sitter\nuv\nzoxide' ]] ||
    fail "Environment required mise tools are incorrect: $required_mise_tools"
  [[ "$optional_mise_tools" == $'bat\neza' ]] ||
    fail "Environment optional mise tools are incorrect: $optional_mise_tools"
  apt_plan="$(printf '%s\n' "$output" | grep 'apt packages:' || true)"
  for tool in starship fzf zoxide ripgrep eza bat jq lazygit; do
    ! grep -Eq "(^|[[:space:]])$tool([[:space:]]|$)" <<<"$apt_plan" ||
      fail "Developer CLI tool remained in the Apt install plan: $tool"
  done

  export SELFISHELL_TEST_SYSTEM_NAME=Darwin
  macos_output="$(run_package_dry_run)"
  grep -Eq '^Would sync required mise tools:.* starship([[:space:]]|$)' <<<"$macos_output" ||
    fail "macOS install omitted mise-managed Starship"
  grep -Eq '^Would sync required mise tools:.* lazygit([[:space:]]|$)' <<<"$macos_output" ||
    fail "macOS install omitted mise-managed Lazygit"
  homebrew_plan="$(printf '%s\n' "$macos_output" | grep 'Homebrew formula' || true)"
  for tool in starship fzf zoxide ripgrep eza bat jq lazygit; do
    ! grep -Eq "(^|[[:space:]])$tool([[:space:]]|$)" <<<"$homebrew_plan" ||
      fail "Developer CLI tool remained in the Homebrew install plan: $tool"
  done
  [[ "$full_output" == *'Neovim plugins'* ]] || fail "Environment is missing Neovim plugin setup"
}

test_macos_includes_fonts_and_opt_in_ghostty() {
  local output
  export SELFISHELL_TEST_SYSTEM_NAME=Darwin
  output="$(run_selfishell install --dry-run)"

  [[ "$output" == *'optional Homebrew cask:'* && "$output" == *'font-'* ]] ||
    fail "macOS environment omitted optional fonts"
  [[ "$output" == *'optional Homebrew cask: ghostty'* ]] ||
    fail "macOS environment omitted opt-in Ghostty"
}

test_package_manifest_read_rejects_option_like_package() {
  local manifest_file="$TEST_ROOT/package-record.conf"
  local status

  # package_manifest_read() validates the declarative package list;
  # exercise it directly rather than through a CLI entry point.
  printf 'package ubuntu required apt --allow-unauthenticated\n' >"$manifest_file"

  set +e
  bash -c '
    source "$1/lib/common.sh"
    source "$1/lib/package_manifest.sh"
    package_manifest_read "$2"
  ' _ "$ROOT_DIR" "$manifest_file" >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -eq 2 ]] || fail "Option-like package name should be rejected"
}

run_discovered_tests setup_package_home teardown_package_home
