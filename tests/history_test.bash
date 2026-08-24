#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/tests/test_helper.bash"

test_history_module_uses_persistent_extended_history() {
  setup_test_home
  local output

  output="$(
    HOME="$HOME" ZDOTDIR="" /bin/zsh -f -c '
      source "$1"
      [[ "$HISTFILE" == "$HOME/.zsh_history" ]]
      [[ "$HISTSIZE" == 10000 ]]
      [[ "$SAVEHIST" == 10000 ]]
      [[ -o EXTENDED_HISTORY ]]
      [[ -o INC_APPEND_HISTORY_TIME ]]
      [[ -o HIST_IGNORE_SPACE ]]
      print HISTORY_CONFIG_OK
    ' zsh "$ROOT_DIR/config/shared/zsh/history.zsh"
  )"

  [[ "$output" == HISTORY_CONFIG_OK ]] || fail "Zsh history configuration was not applied"
  teardown_test_home
}

test_history_module_is_a_managed_resource() {
  setup_test_home
  local resources
  local config_dir="$HOME/.config/selfishell"

  resources="$(
    SELFISHELL_CONFIG_DIR="$config_dir" SELFISHELL_ROOT="$ROOT_DIR" \
      bash -c 'source "$1/lib/resources.sh"; selfishell_managed_resources' _ "$ROOT_DIR"
  )"

  grep -Fqx $'file\tzsh-history\t'"$config_dir/zsh/history.zsh"$'\t'"$ROOT_DIR/config/shared/zsh/history.zsh" <<<"$resources" ||
    fail "history.zsh is not declared as a managed resource"
  teardown_test_home
}

run_discovered_tests
