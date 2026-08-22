#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/tests/test_helper.bash"

test_complete_release_lifecycle() {
  local initial_version=0.2.0
  local next_version=0.2.1
  local prefix
  local release_store
  local artifacts
  local tool
  local manifest
  local repository
  local plugin_dir
  local revision

  setup_test_home
  trap teardown_test_home EXIT
  mkdir -p "$TEST_ROOT/bin"
  for tool in starship fzf zoxide rg nvim tree-sitter gcc build-essential; do
    printf '#!/usr/bin/env bash\nexit 0\n' >"$TEST_ROOT/bin/$tool"
    chmod +x "$TEST_ROOT/bin/$tool"
  done
  export PATH="$TEST_ROOT/bin:$PATH"
  mkdir -p "$HOME/.local/share/zinit/zinit.git"
  printf ':\n' >"$HOME/.local/share/zinit/zinit.git/zinit.zsh"
  # A real install provisions the pinned plugins alongside Zinit itself. Model
  # that here so doctor sees a complete state instead of the degraded one it
  # must report. Doctor now compares each checkout's HEAD against the
  # manifest's pinned revision, so these must be real repositories -- real
  # upstream commit hashes can't be reproduced locally, so the manifest is
  # pointed at a rewritten copy that pins whatever commit each local fixture
  # repo actually produces.
  manifest="$TEST_ROOT/dependencies.conf"
  grep -v '^zsh-plugin ' "$ROOT_DIR/dependencies.conf" >"$manifest"
  while read -r _ repository _; do
    plugin_dir="$HOME/.local/share/zinit/plugins/${repository//\//---}"
    mkdir -p "$plugin_dir"
    git -C "$plugin_dir" init --quiet
    git -C "$plugin_dir" config user.email test@example.com
    git -C "$plugin_dir" config user.name test
    git -C "$plugin_dir" commit --quiet --allow-empty -m initial
    revision="$(git -C "$plugin_dir" rev-parse HEAD)"
    printf 'zsh-plugin %s %s all all - - - -\n' "$repository" "$revision" >>"$manifest"
  done < <(grep '^zsh-plugin ' "$ROOT_DIR/dependencies.conf")
  export SELFISHELL_DEPENDENCIES_FILE="$manifest"
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
  "$prefix/bin/selfishell" doctor
  [[ "$("$prefix/bin/selfishell" version)" == "selfishell $initial_version" ]] || fail "Clean install failed"
  [[ -f "$HOME/.zshrc" && ! -L "$HOME/.zshrc" ]] || fail "Install did not preserve a user-owned .zshrc"
  grep -Fqx '# >>> Selfishell initialize >>>' "$HOME/.zshrc" || fail "Install did not add the Zsh loader"

  "$prefix/bin/selfishell" update --cli-only --version "$next_version" --yes >/dev/null
  [[ "$("$prefix/bin/selfishell" version)" == "selfishell $next_version" ]] || fail "Upgrade failed"

  SELFISHELL_RELEASE_ROOT='file:///network-must-not-be-used' \
    "$prefix/bin/selfishell" rollback --yes >/dev/null
  [[ "$("$prefix/bin/selfishell" version)" == "selfishell $initial_version" ]] || fail "Offline rollback failed"

  "$prefix/bin/selfishell" uninstall --restore --yes >/dev/null
  assert_file_content 'original zshrc' "$HOME/.zshrc"
  [[ ! -e "$XDG_CONFIG_HOME/selfishell/zsh/zshrc" ]] || fail "Uninstall left managed configuration"
}

run_discovered_tests '' teardown_test_home
