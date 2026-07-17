#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/tests/test_helper.bash"

test_complete_release_lifecycle() {
  local initial_version=1.0.0
  local next_version=1.1.0
  local prefix
  local release_store
  local artifacts
  local tool

  setup_test_home
  trap teardown_test_home EXIT
  mkdir -p "$TEST_ROOT/bin"
  for tool in starship fzf zoxide rg nvim; do
    printf '#!/usr/bin/env bash\nexit 0\n' >"$TEST_ROOT/bin/$tool"
    chmod +x "$TEST_ROOT/bin/$tool"
  done
  export PATH="$TEST_ROOT/bin:$PATH"
  mkdir -p "$HOME/.local/share/zinit/zinit.git"
  printf ':\n' >"$HOME/.local/share/zinit/zinit.git/zinit.zsh"
  prefix="$TEST_ROOT/prefix"
  release_store="$TEST_ROOT/releases"
  export XDG_CONFIG_HOME="$HOME/.config"
  export XDG_STATE_HOME="$HOME/.local/state"
  export SELFISHELL_RELEASE_ROOT="file://$release_store"
  printf 'original zshrc\n' >"$HOME/.zshrc"

  for version in "$initial_version" "$next_version"; do
    artifacts="$TEST_ROOT/artifacts-$version"
    mkdir -p "$artifacts" "$release_store/download/v$version"
    bash "$ROOT_DIR/scripts/build-release.sh" --version "$version" --output "$artifacts" >/dev/null
    cp "$artifacts"/* "$release_store/download/v$version/"
  done

  bash "$ROOT_DIR/install.sh" --version "$initial_version" --prefix "$prefix" \
    --setup --yes --profile minimal --skip-packages >/dev/null
  "$prefix/bin/selfishell" doctor >/dev/null
  [[ "$("$prefix/bin/selfishell" version)" == "selfishell $initial_version" ]] || fail "Clean install failed"
  assert_symlink_to "$XDG_CONFIG_HOME/selfishell/zsh/zshrc" "$HOME/.zshrc"

  "$prefix/bin/selfishell" update --cli-only --version "$next_version" --yes >/dev/null
  [[ "$("$prefix/bin/selfishell" version)" == "selfishell $next_version" ]] || fail "Upgrade failed"

  SELFISHELL_RELEASE_ROOT='file:///network-must-not-be-used' \
    "$prefix/bin/selfishell" rollback --yes >/dev/null
  [[ "$("$prefix/bin/selfishell" version)" == "selfishell $initial_version" ]] || fail "Offline rollback failed"

  "$prefix/bin/selfishell" uninstall --restore --yes >/dev/null
  assert_file_content 'original zshrc' "$HOME/.zshrc"
  [[ ! -e "$XDG_CONFIG_HOME/selfishell/zsh/zshrc" ]] || fail "Uninstall left managed configuration"
}

test_complete_release_lifecycle
printf 'PASS: test_complete_release_lifecycle\n'
