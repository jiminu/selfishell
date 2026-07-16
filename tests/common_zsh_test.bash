#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/tests/test_helper.bash"

test_minimal_profile_initializes_git_completion_without_zinit() {
  setup_test_home
  trap teardown_test_home EXIT

  XDG_CACHE_HOME="$HOME/.cache" \
    PATH="/usr/bin:/bin" \
    /bin/zsh -f -c '
      load_nvm() { :; }
      source "$1"
      (( $+functions[_git] ))
      [[ -s "$HOME/.zcompdump" ]]
    ' zsh "$ROOT_DIR/common/common.zsh"
}

test_minimal_profile_initializes_git_completion_without_zinit
printf 'PASS: test_minimal_profile_initializes_git_completion_without_zinit\n'
