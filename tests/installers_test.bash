#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/tests/test_helper.bash"
source "$ROOT_DIR/lib/common.sh"
source "$ROOT_DIR/lib/installers.sh"

NVIM_ARGUMENTS=""
MISE_ARGUMENTS=""
MISE_CONFIG=""
GIT_ARGUMENTS=""
NVIM_CALLS=()
TS_ARGUMENTS=""

nvim() {
  NVIM_ARGUMENTS="$*"
  NVIM_CALLS+=("$*")
}

mise() {
  MISE_ARGUMENTS="$*"
  MISE_CONFIG="$MISE_GLOBAL_CONFIG_FILE"
}

git() {
  GIT_ARGUMENTS="$*"
  if [[ "$1" == "clone" ]]; then
    mkdir -p "${@: -1}"
  fi
}

selfishell_nvim_treesitter_languages() {
  printf '%s\n' 'lua vim'
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
  NVIM_CALLS=()
  install_vim_plugins 0
  [[ "${NVIM_CALLS[0]}" == "--headless +Lazy! sync +qa" ]] ||
    fail "Neovim plugin installation was not invoked"
  [[ "${NVIM_CALLS[1]}" == *'TSInstallSync '* ]] ||
    fail "Tree-sitter parser installation was not invoked"
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
  [[ "$output" == *'Would install declared Neovim plugins.'* ]] ||
    fail "Neovim plugin dry run was not reported"
  [[ "$output" == *'Would install lazy.nvim bootstrap repository.'* ]] ||
    fail "lazy.nvim dry run was not reported"
  [[ "$output" == *'Would install Tree-sitter parsers.'* ]] ||
    fail "Tree-sitter dry run was not reported"
  [[ -z "$NVIM_ARGUMENTS" ]] || fail "Neovim plugin dry run invoked Neovim"
}

test_installs_lazy_nvim_before_syncing_plugins() {
  local lazy_path

  lazy_path="$HOME/.local/share/nvim/lazy/lazy.nvim"
  rm -rf "$HOME/.local/share/nvim"
  NVIM_CALLS=()

  install_vim_plugins 0

  [[ -d "$lazy_path" ]] || fail "lazy.nvim was not prepared before plugin sync"
  [[ "$GIT_ARGUMENTS" == clone* ]] || fail "lazy.nvim bootstrap did not use git clone"
  [[ "${NVIM_CALLS[*]}" == *'Lazy! sync'* ]] || fail "Neovim plugin sync did not run"
  [[ "${NVIM_CALLS[*]}" == *'TSInstallSync lua vim'* ]] || fail "Tree-sitter parsers were not prepared"
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
test_installs_lazy_nvim_before_syncing_plugins
printf 'PASS: test_installs_lazy_nvim_before_syncing_plugins\n'

teardown_test_home
