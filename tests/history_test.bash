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
      # A bare [[ ]] does not stop a -c script, so each check has to report the
      # mismatch itself: anything printed before the marker breaks the exact
      # comparison below and names what went wrong.
      [[ "$HISTFILE" == "$HOME/.zsh_history" ]] || print -r -- "HISTFILE=$HISTFILE"
      [[ "$HISTSIZE" == 10000 ]] || print -r -- "HISTSIZE=$HISTSIZE"
      [[ "$SAVEHIST" == 10000 ]] || print -r -- "SAVEHIST=$SAVEHIST"
      for option in EXTENDED_HISTORY INC_APPEND_HISTORY_TIME HIST_IGNORE_SPACE \
        HIST_REDUCE_BLANKS; do
        [[ -o $option ]] || print -r -- "unset: $option"
      done
      print HISTORY_CONFIG_OK
    ' zsh "$ROOT_DIR/config/shared/zsh/history.zsh"
  )"

  [[ "$output" == HISTORY_CONFIG_OK ]] ||
    fail "Zsh history configuration was not applied: $output"
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
