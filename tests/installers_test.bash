#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/tests/test_helper.bash"
source "$ROOT_DIR/lib/common.sh"
source "$ROOT_DIR/lib/installers.sh"

VIM_ARGUMENTS=""
MISE_ARGUMENTS=""
MISE_CONFIG=""

vim() {
  VIM_ARGUMENTS="$*"
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

test_installs_declared_vim_plugins() {
  install_vim_plugins 0
  [[ "$VIM_ARGUMENTS" == "+PluginInstall +qall" ]] ||
    fail "Vim plugin installation was not invoked"
}

test_skips_vim_when_declared_plugins_exist() {
  VIM_ARGUMENTS=""
  mkdir -p "$HOME/.vim/bundle/Vundle.vim" "$HOME/.vim/bundle/nerdtree" "$HOME/.vim/bundle/vim-code-dark"

  install_vim_plugins 0

  [[ -z "$VIM_ARGUMENTS" ]] || fail "Installed Vim plugins triggered PluginInstall"
}

test_offline_mode_skips_vim_plugins() {
  VIM_ARGUMENTS=""
  SELFISHELL_OFFLINE=1 install_vim_plugins 0
  [[ -z "$VIM_ARGUMENTS" ]] || fail "Offline mode invoked Vim plugin installation"
}

test_vim_plugin_dry_run_is_non_mutating() {
  local output
  VIM_ARGUMENTS=""
  output="$(install_vim_plugins 1)"
  [[ "$output" == "Would install declared Vim plugins." ]] || fail "Vim plugin dry run was not reported"
  [[ -z "$VIM_ARGUMENTS" ]] || fail "Vim plugin dry run invoked Vim"
}

setup_test_home
export SELFISHELL_ROOT="$ROOT_DIR"

test_installs_declared_mise_tools_with_managed_config
printf 'PASS: test_installs_declared_mise_tools_with_managed_config\n'
test_installs_declared_vim_plugins
printf 'PASS: test_installs_declared_vim_plugins\n'
test_skips_vim_when_declared_plugins_exist
printf 'PASS: test_skips_vim_when_declared_plugins_exist\n'
test_offline_mode_skips_vim_plugins
printf 'PASS: test_offline_mode_skips_vim_plugins\n'
test_vim_plugin_dry_run_is_non_mutating
printf 'PASS: test_vim_plugin_dry_run_is_non_mutating\n'

teardown_test_home
