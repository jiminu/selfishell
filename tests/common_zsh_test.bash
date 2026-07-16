#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/tests/test_helper.bash"

test_minimal_profile_initializes_git_completion_without_zinit() {
  setup_test_home

  XDG_CACHE_HOME="$HOME/.cache" \
    PATH="/usr/bin:/bin" \
    /bin/zsh -f -c '
      load_nvm() { :; }
      source "$1"
      (( $+functions[_git] ))
      [[ -s "$HOME/.zcompdump" ]]
    ' zsh "$ROOT_DIR/common/common.zsh"

  teardown_test_home
}

test_macos_managed_zsh_adds_default_cli_prefix_to_path() {
  local fake_bin

  setup_test_home
  fake_bin="$TEST_ROOT/bin"
  mkdir -p "$fake_bin" "$HOME/.config/selfishell/zsh"
  printf '#!/usr/bin/env bash\n' >"$fake_bin/brew"
  chmod +x "$fake_bin/brew"
  printf ':\n' >"$HOME/.config/selfishell/zsh/common.zsh"

  XDG_CONFIG_HOME="$HOME/.config" \
    PATH="$fake_bin:/usr/bin:/bin" \
    /bin/zsh -f -c '
      source "$1"
      [[ "$path[1]" == "$HOME/.local/bin" ]]
      [[ "$path[2]" == "$HOME/.rd/bin" ]]
    ' zsh "$ROOT_DIR/mac/.zshrc"

  teardown_test_home
}

test_minimal_profile_initializes_git_completion_without_zinit
printf 'PASS: test_minimal_profile_initializes_git_completion_without_zinit\n'
test_macos_managed_zsh_adds_default_cli_prefix_to_path
printf 'PASS: test_macos_managed_zsh_adds_default_cli_prefix_to_path\n'
