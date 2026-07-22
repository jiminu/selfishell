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
  printf '%s\n' "$output" | awk '/^Would install .* (apt packages|Homebrew|direct package|mise tools)/'
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
  [[ "$output" != *'kubectl'* ]] || fail "Minimal profile included Kubernetes tools"
  [[ "$output" != *'build-essential'* ]] || fail "Minimal profile included compiler tooling"
  [[ "$output" != *'tree-sitter-cli'* ]] || fail "Minimal profile included Tree-sitter tooling"
  [[ "$full_output" != *'Neovim plugins'* ]] || fail "Minimal profile included Neovim plugin setup"
}

test_developer_includes_development_tools() {
  local output full_output
  output="$(run_profile_dry_run developer)"
  full_output="$(bash "$ROOT_DIR/bin/selfishell" install --profile developer --dry-run)"

  [[ "$output" == *'required apt packages: zsh git curl ca-certificates vim fzf zoxide ripgrep jq build-essential'* ]] ||
    fail "Developer profile required apt packages are incomplete"
  [[ "$output" == *'optional apt packages: eza bat'* ]] ||
    fail "Developer profile optional apt packages are incomplete"
  [[ "$output" == *'direct package: mise'* ]] ||
    fail "Developer profile is missing development tools"
  [[ "$output" == *'required mise tools: neovim@0.12.4 tree-sitter@0.26.11 node@24.18.0 python@3.13.14'* ]] ||
    fail "Developer profile is missing mise runtimes"
  [[ "$output" != *'kubectl'* && "$output" != *'java'* && "$output" != *'kubectx'* ]] ||
    fail "Developer profile still includes Kubernetes or Java tools"
  [[ "$output" == *'build-essential'* && "$output" == *'tree-sitter@0.26.11'* ]] ||
    fail "Developer profile is missing Tree-sitter build tooling"
  [[ "$full_output" == *'Neovim plugins'* ]] || fail "Developer profile is missing Neovim plugin setup"
}

test_developer_mise_profile_matches_managed_config() {
  local name version

  while read -r name version; do
    grep -Fqx "$name = \"$version\"" "$ROOT_DIR/common/mise.toml" ||
      fail "mise config does not match developer selector: $name@$version"
  done < <(awk '$1 == "package" && $4 == "mise" { split($5, fields, "@"); print fields[1], substr($5, length(fields[1]) + 2) }' "$ROOT_DIR/profiles/developer.conf")
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

test_local_profile_adds_private_package() {
  local output
  local local_profile="$TEST_ROOT/company.conf"

  printf 'package ubuntu optional apt company-cli\n' >"$local_profile"
  output="$(bash "$ROOT_DIR/bin/selfishell" install --profile minimal --local-profile "$local_profile" --dry-run)"

  [[ "$output" == *'optional apt packages:'* && "$output" == *'company-cli'* ]] ||
    fail "Local profile package was not included"
}

test_offline_mode_skips_package_operations() {
  local output
  export SELFISHELL_OFFLINE=1
  output="$(bash "$ROOT_DIR/bin/selfishell" install --profile developer --dry-run)"

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
