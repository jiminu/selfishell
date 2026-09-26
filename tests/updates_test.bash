#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/tests/test_helper.bash"
source "$ROOT_DIR/tests/cli_runner.bash"
source "$ROOT_DIR/lib/common.sh"
source "$ROOT_DIR/lib/paths.sh"
source "$ROOT_DIR/lib/dependencies.sh"

setup_update_home() {
  setup_test_home
  export XDG_STATE_HOME="$HOME/.local/state"
  export SELFISHELL_TEST_SYSTEM_NAME=Linux
  export SELFISHELL_TEST_MACHINE_ARCH=x86_64
  export SELFISHELL_TEST_OS_RELEASE_FILE="$TEST_ROOT/os-release"
  export SELFISHELL_TEST_PROC_VERSION_FILE="$TEST_ROOT/proc-version"
  printf 'ID=ubuntu\n' >"$SELFISHELL_TEST_OS_RELEASE_FILE"
  printf 'Linux version 6.8.0\n' >"$SELFISHELL_TEST_PROC_VERSION_FILE"
}

teardown_update_home() {
  unset XDG_CONFIG_HOME XDG_STATE_HOME SELFISHELL_DEPENDENCIES_FILE
  unset SELFISHELL_TEST_SYSTEM_NAME SELFISHELL_TEST_MACHINE_ARCH
  unset SELFISHELL_TEST_OS_RELEASE_FILE SELFISHELL_TEST_PROC_VERSION_FILE
  teardown_test_home
}

test_tools_update_synchronizes_packages() {
  local output
  export XDG_CONFIG_HOME="$HOME/.config"
  mkdir -p "$XDG_STATE_HOME/selfishell"
  printf '1\n' >"$XDG_STATE_HOME/selfishell/configured"

  output="$(run_selfishell update --tools-only --dry-run)"
  [[ "$output" == *'Would install required apt packages:'* ]] ||
    fail "Tools update did not synchronize package-manager packages"
  [[ "$output" == *'git'* ]] || fail "Tools update did not include the environment packages"
  [[ "$output" == *'Neovim plugins'* ]] || fail "Tools update omitted Neovim plugin setup"
  [[ "$output" == *'Would prune unused mise versions'* ]] || fail "Tools update omitted default mise cleanup"
}

test_tools_update_skip_packages_avoids_package_operations() {
  local output
  export XDG_CONFIG_HOME="$HOME/.config"
  mkdir -p "$XDG_STATE_HOME/selfishell"
  printf '1\n' >"$XDG_STATE_HOME/selfishell/configured"

  output="$(run_selfishell update --tools-only --skip-packages --dry-run)"
  [[ "$output" == *'Skipping package and tool installation.'* ]] ||
    fail "--skip-packages did not report skipping package and tool installation: $output"
  [[ "$output" != *'apt packages'* ]] ||
    fail "--skip-packages still touched apt packages: $output"
  [[ "$output" != *'prune'* ]] || fail "--skip-packages still planned cleanup: $output"
}

# Keep lifecycle orchestration real; replace only the installing operations.
run_update_cleanup_fixture() {
  bash -euo pipefail -c '
    export SELFISHELL_ROOT="$1"
    for module in common paths platform package_manifest installers packages releases commands/update; do
      source "$1/lib/$module.sh"
    done
    confirm_action() { return 0; }
    managed_preflight_zsh_loader() { return 0; }
    managed_preflight_block_target() { return 0; }
    packages_install() {
      printf "packages\n"
      SELFISHELL_SKIPPED_OPTIONAL_PACKAGES=()
      [[ "$2" != 1 ]] || return 0
      [[ "$FAIL_PHASE" != optional ]] || SELFISHELL_SKIPPED_OPTIONAL_PACKAGES=(bat)
      [[ "$FAIL_PHASE" != packages ]]
    }
    install_managed_configuration() { printf "configuration\n"; [[ "$FAIL_PHASE" != configuration ]]; }
    install_neovim_plugins() { printf "plugins\n"; [[ "$FAIL_PHASE" != plugins ]]; }
    selfishell_mise_command() { printf "mise\n"; }
    mise() {
      case "$*" in
        *"settings get ignored_config_paths") printf "[]\n" ;;
        *"config ls --tracked-configs") [[ "$FAIL_PHASE" != tracking ]] || return 0; printf "%s/config/shared/mise.toml\n" "$SELFISHELL_ROOT" ;;
        *"config ls") [[ "$FAIL_PHASE" != retention ]] ;;
        *"prune --tools --yes"*) printf "pruned\n"; [[ "$FAIL_PHASE" != prune ]] ;;
        *) return 91 ;;
      esac
    }
    update_tools_and_configuration 1 0 1 "$2" 0
  ' _ "$ROOT_DIR" "${1:-0}"
}

test_update_prunes_only_after_successful_setup() {
  local output phase status
  mkdir -p "$XDG_STATE_HOME/selfishell"
  printf '1\n' >"$XDG_STATE_HOME/selfishell/configured"
  output="$(FAIL_PHASE=none run_update_cleanup_fixture)"
  [[ "$output" == *$'packages\nconfiguration\nplugins\npruned\n'* ]] || fail "Cleanup did not follow successful setup: $output"
  for phase in packages configuration plugins; do
    status=0
    output="$(FAIL_PHASE="$phase" run_update_cleanup_fixture 2>&1)" || status=$?
    [[ "$status" != 0 && "$output" != *pruned* ]] || fail "Failed $phase phase still pruned or succeeded: $output"
  done
  output="$(FAIL_PHASE=optional run_update_cleanup_fixture 2>&1)"
  [[ "$output" != *pruned* && "$output" == *'Skipping mise cleanup'* ]] || fail "Partial tool update still pruned: $output"
  output="$(FAIL_PHASE=none run_update_cleanup_fixture 1)"
  [[ "$output" != *pruned* ]] || fail "Configuration-only update pruned tools"
  output="$(FAIL_PHASE=prune run_update_cleanup_fixture 2>&1)"
  [[ "$output" == *'Could not prune'* && "$output" == *'synchronized.'* ]] || fail "Cleanup failure hid successful update: $output"
  for phase in tracking retention; do
    output="$(FAIL_PHASE="$phase" run_update_cleanup_fixture 2>&1)"
    [[ "$output" != *pruned* && "$output" == *'Could not prune'* ]] || fail "Failed retention preflight still pruned: $output"
  done
}

run_discovered_tests setup_update_home teardown_update_home
