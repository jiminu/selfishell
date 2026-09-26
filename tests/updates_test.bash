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

test_mise_cleanup_preserves_current_and_project_versions_not_rollback_only() {
  local mise_binary version dir before after module
  mise_binary="$(command -v mise)" || skip "mise is unavailable for cleanup integration"
  [[ -x "$mise_binary" && -f "$mise_binary" ]] || skip "mise executable is unavailable"
  export XDG_CONFIG_HOME="$HOME/.config" XDG_DATA_HOME="$HOME/.local/share" XDG_CACHE_HOME="$HOME/.cache"
  export MISE_DATA_DIR="$XDG_DATA_HOME/mise" MISE_CACHE_DIR="$XDG_CACHE_HOME/mise" MISE_STATE_DIR="$XDG_STATE_HOME/mise"
  export MISE_OFFLINE=1 MISE_TRUSTED_CONFIG_PATHS="$HOME/project with spaces"
  unset MISE_GLOBAL_CONFIG_FILE MISE_IGNORED_CONFIG_PATHS __MISE_DIFF __MISE_SESSION
  for module in package_manifest platform packages installers releases; do
    # shellcheck disable=SC1090 # Load the same modules as the CLI.
    source "$ROOT_DIR/lib/$module.sh"
  done
  export SELFISHELL_ROOT="$HOME/selfishell/releases/2.0.0"
  mkdir -p "$SELFISHELL_ROOT/config/shared" "$HOME/selfishell/releases/1.0.0/config/shared" "$HOME/project with spaces"
  # Exercise symlinked parents and repeated separators on every platform.
  ln -s "$HOME/selfishell" "$HOME/release alias"
  SELFISHELL_ROOT="$HOME/release alias//releases/2.0.0"
  ln -s releases/2.0.0 "$HOME/selfishell/current"
  ln -s releases/1.0.0 "$HOME/selfishell/previous"
  for version in 1.0.0 2.0.0; do
    mkdir -p "$HOME/selfishell/releases/$version/bin"
    touch "$HOME/selfishell/releases/$version/bin/selfishell"
    chmod +x "$HOME/selfishell/releases/$version/bin/selfishell"
    printf '%s\n' "$version" >"$HOME/selfishell/releases/$version/VERSION"
  done
  printf '[tools]\nnode = "24.18.0"\n' >"$SELFISHELL_ROOT/config/shared/mise.toml"
  printf '[tools]\nnode = "24.13.0"\n' >"$HOME/selfishell/releases/1.0.0/config/shared/mise.toml"
  printf '[tools]\nnode = "22.0.0"\n' >"$HOME/project with spaces/mise.toml"
  printf 'package all required mise node\npackage macos optional mise go\n' >"$SELFISHELL_ROOT/packages.conf"
  package_manifest_load
  # shellcheck disable=SC2034 # Consumed by packages_prune_mise.
  SELFISHELL_SKIPPED_OPTIONAL_PACKAGES=()
  # Real mise inventory/prune with inert installed-version fixtures; no downloads.
  for version in 20.0.0 22.0.0 24.13.0 24.18.0; do
    dir="$MISE_DATA_DIR/installs/node/$version/bin"
    mkdir -p "$dir"
    printf '#!/bin/sh\nexit 0\n' >"$dir/node"
    chmod +x "$dir/node"
  done
  mkdir -p "$MISE_DATA_DIR/installs/go/1.20.0/bin"
  printf '#!/bin/sh\nexit 0\n' >"$MISE_DATA_DIR/installs/go/1.20.0/bin/go"
  chmod +x "$MISE_DATA_DIR/installs/go/1.20.0/bin/go"
  "$mise_binary" -C "$HOME/project with spaces" config ls >/dev/null
  cd "$HOME/project with spaces"
  before="$(find "$HOME" -print | sort)"
  packages_prune_mise ubuntu-wsl 1
  after="$(find "$HOME" -print | sort)"
  [[ "$before" == "$after" ]] || fail "Cleanup dry-run changed the filesystem"
  printf 'invalid\n' >"$HOME/selfishell/releases/1.0.0/VERSION"
  if packages_prune_mise ubuntu-wsl 0; then
    fail "Cleanup accepted a corrupt rollback release"
  fi
  [[ -d "$MISE_DATA_DIR/installs/node/20.0.0" ]] || fail "Retention failure deleted tools"
  printf '1.0.0\n' >"$HOME/selfishell/releases/1.0.0/VERSION"
  MISE_TRUSTED_CONFIG_PATHS="$TEST_ROOT" "$mise_binary" -C "$HOME/selfishell/releases/1.0.0/config/shared" config ls >/dev/null
  if MISE_IGNORED_CONFIG_PATHS="$HOME/selfishell/releases/1.0.0" packages_prune_mise ubuntu-wsl 0; then
    fail "Cleanup accepted ignored rollback pins despite existing tracking metadata"
  fi
  [[ -x "$MISE_DATA_DIR/installs/node/24.13.0/bin/node" ]] || fail "Ignored rollback pin was removed"
  packages_prune_mise ubuntu-wsl 0
  [[ ! -e "$MISE_DATA_DIR/installs/node/20.0.0" ]] || fail "Unused managed version survived cleanup"
  [[ ! -e "$MISE_DATA_DIR/installs/node/24.13.0" ]] || fail "Rollback-only version survived cleanup"
  for version in 22.0.0 24.18.0; do
    [[ -x "$MISE_DATA_DIR/installs/node/$version/bin/node" ]] || fail "Needed node@$version was removed"
  done
  [[ -d "$MISE_DATA_DIR/installs/go/1.20.0" ]] || fail "Another platform's tool was removed"
  assert_file_content '[tools]
node = "24.13.0"' "$HOME/selfishell/releases/1.0.0/config/shared/mise.toml"
  [[ "$(readlink "$HOME/selfishell/previous")" == releases/1.0.0 ]] || fail "Cleanup changed CLI rollback"
  [[ -z "${MISE_IGNORED_CONFIG_PATHS:-}" ]] || fail "Cleanup leaked ignored paths into the shell"
  # The same rollback version must survive when a project still needs it.
  dir="$MISE_DATA_DIR/installs/node/24.13.0/bin"
  mkdir -p "$dir"
  printf '#!/bin/sh\nexit 0\n' >"$dir/node"
  chmod +x "$dir/node"
  printf '[tools]\nnode = ["22.0.0", "24.13.0"]\n' >"$HOME/project with spaces/mise.toml"
  packages_prune_mise ubuntu-wsl 0
  [[ -x "$dir/node" ]] || fail "Rollback version needed by a project was removed"
  mv "$HOME/selfishell/releases/1.0.0/config/shared/mise.toml" "$TEST_ROOT/previous.toml"
  ln -s "$HOME/project with spaces/mise.toml" "$HOME/selfishell/releases/1.0.0/config/shared/mise.toml"
  if packages_prune_mise ubuntu-wsl 0; then
    fail "Cleanup accepted a rollback config linked to a project"
  fi
  [[ -x "$MISE_DATA_DIR/installs/node/22.0.0/bin/node" ]] || fail "Linked project version was removed"
  rm "$HOME/selfishell/releases/1.0.0/config/shared/mise.toml"
  mv "$TEST_ROOT/previous.toml" "$HOME/selfishell/releases/1.0.0/config/shared/mise.toml"
  # A previous link pointing at current must not exclude the active config.
  release_atomic_link releases/2.0.0 "$HOME/selfishell/previous"
  [[ "$(readlink "$HOME/selfishell/previous")" == releases/2.0.0 ]] || fail "Fixture did not point previous at current"
  packages_prune_mise ubuntu-wsl 0
  [[ -x "$MISE_DATA_DIR/installs/node/24.18.0/bin/node" ]] || fail "Cleanup excluded the current release"
  # A literal path must not turn into multiple ignored paths or a glob.
  release_atomic_link releases/1.0.0 "$HOME/selfishell/previous"
  mv "$HOME/selfishell" "$HOME/selfishell:unsafe"
  SELFISHELL_ROOT="$HOME/selfishell:unsafe/releases/2.0.0"
  if packages_prune_mise ubuntu-wsl 0; then
    fail "Cleanup accepted a rollback path containing mise's list separator"
  fi
  # Empty scope must never turn into mise's global prune.
  printf 'package all required apt git\n' >"$SELFISHELL_ROOT/packages.conf"
  package_manifest_load
  packages_prune_mise ubuntu 0
  [[ -d "$MISE_DATA_DIR/installs/go/1.20.0" ]] || fail "Empty scope pruned unrelated tools"
}

run_discovered_tests setup_update_home teardown_update_home
