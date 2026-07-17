#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/tests/test_helper.bash"
source "$ROOT_DIR/lib/common.sh"
source "$ROOT_DIR/lib/installers.sh"

NVIM_ARGUMENTS=""
MISE_ARGUMENTS=""
MISE_CONFIG=""

nvim() {
  NVIM_ARGUMENTS="$*"
}

mise() {
  MISE_ARGUMENTS="$*"
  MISE_CONFIG="$MISE_GLOBAL_CONFIG_FILE"
}

test_installs_declared_mise_tools_with_managed_config() {
  SELFISHELL_SKIPPED_OPTIONAL_PACKAGES=()
  install_mise_tools required 0 node@24.18.0 python@3.13.14

  [[ "$MISE_ARGUMENTS" == 'install node@24.18.0 python@3.13.14' ]] ||
    fail "mise tools were not installed together"
  [[ "$MISE_CONFIG" == "$ROOT_DIR/common/mise.toml" ]] ||
    fail "mise install did not use the Selfishell config"
}

test_installs_declared_neovim_plugins() {
  NVIM_ARGUMENTS=""
  install_vim_plugins 0
  [[ "$NVIM_ARGUMENTS" == "--headless +Lazy! sync +qa" ]] ||
    fail "Neovim plugin installation was not invoked"
}

test_offline_mode_skips_neovim_plugins() {
  NVIM_ARGUMENTS=""
  SELFISHELL_OFFLINE=1 install_vim_plugins 0
  [[ -z "$NVIM_ARGUMENTS" ]] || fail "Offline mode invoked Neovim plugin installation"
}

test_neovim_plugin_dry_run_is_non_mutating() {
  local output
  NVIM_ARGUMENTS=""
  output="$(install_vim_plugins 1)"
  [[ "$output" == "Would install declared Neovim plugins." ]] || fail "Neovim plugin dry run was not reported"
  [[ -z "$NVIM_ARGUMENTS" ]] || fail "Neovim plugin dry run invoked Neovim"
}

setup_test_home
export SELFISHELL_ROOT="$ROOT_DIR"

test_installs_declared_mise_tools_with_managed_config
printf 'PASS: test_installs_declared_mise_tools_with_managed_config\n'
test_installs_declared_neovim_plugins
printf 'PASS: test_installs_declared_neovim_plugins\n'
test_offline_mode_skips_neovim_plugins
printf 'PASS: test_offline_mode_skips_neovim_plugins\n'
test_neovim_plugin_dry_run_is_non_mutating
printf 'PASS: test_neovim_plugin_dry_run_is_non_mutating\n'

teardown_test_home
