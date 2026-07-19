#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

source "$ROOT_DIR/tests/test_helper.bash"

setup_managed_home() {
  setup_test_home
  export XDG_CONFIG_HOME="$HOME/.config"
  export XDG_STATE_HOME="$HOME/.local/state"
  export XDG_CACHE_HOME="$HOME/.cache"
  export SELFISHELL_TEST_SYSTEM_NAME=Linux
  export SELFISHELL_TEST_MACHINE_ARCH=x86_64
  export SELFISHELL_TEST_OS_RELEASE_FILE="$TEST_ROOT/os-release"
  export SELFISHELL_TEST_PROC_VERSION_FILE="$TEST_ROOT/proc-version"
  printf 'ID=ubuntu\n' >"$SELFISHELL_TEST_OS_RELEASE_FILE"
  printf 'Linux microsoft WSL2\n' >"$SELFISHELL_TEST_PROC_VERSION_FILE"
}

teardown_managed_home() {
  unset XDG_CONFIG_HOME XDG_STATE_HOME XDG_CACHE_HOME
  unset SELFISHELL_TEST_SYSTEM_NAME SELFISHELL_TEST_MACHINE_ARCH
  unset SELFISHELL_TEST_OS_RELEASE_FILE SELFISHELL_TEST_PROC_VERSION_FILE
  teardown_test_home
}

run_selfishell() {
  bash "$ROOT_DIR/bin/selfishell" "$@"
}

test_install_copies_configuration_and_tracks_resources() {
  local state_count

  printf 'original zshrc' >"$HOME/.zshrc"
  run_selfishell install --skip-packages --yes >/dev/null

  assert_symlink_to "$XDG_CONFIG_HOME/selfishell/zsh/zshrc" "$HOME/.zshrc"
  assert_symlink_to "$XDG_CONFIG_HOME/selfishell/zsh/zshenv" "$HOME/.zshenv"
  assert_symlink_to "$XDG_CONFIG_HOME/selfishell/starship.toml" "$XDG_CONFIG_HOME/starship.toml"
  # The nvim directory itself is now the managed symlink target.
  assert_symlink_to "$XDG_CONFIG_HOME/selfishell/nvim" "$XDG_CONFIG_HOME/nvim"
  cmp -s "$ROOT_DIR/common/nvim/init.lua" "$XDG_CONFIG_HOME/selfishell/nvim/init.lua" ||
    fail "Neovim init.lua was not installed"
  cmp -s "$ROOT_DIR/common/nvim/lua/config/options.lua" "$XDG_CONFIG_HOME/selfishell/nvim/lua/config/options.lua" ||
    fail "Neovim options module was not installed"
  cmp -s "$ROOT_DIR/common/nvim/lua/plugins/lsp.lua" "$XDG_CONFIG_HOME/selfishell/nvim/lua/plugins/lsp.lua" ||
    fail "Neovim lsp plugin was not installed"
  cmp -s "$ROOT_DIR/common/common.zsh" "$XDG_CONFIG_HOME/selfishell/zsh/common.zsh" ||
    fail "Common Zsh configuration was not copied"
  cmp -s "$ROOT_DIR/common/zshenv" "$XDG_CONFIG_HOME/selfishell/zsh/zshenv" ||
    fail "zshenv was not copied"
  cmp -s "$ROOT_DIR/common/runtime.zsh" "$XDG_CONFIG_HOME/selfishell/zsh/runtime.zsh" ||
    fail "Runtime Zsh module was not copied"
  cmp -s "$ROOT_DIR/common/mise.toml" "$XDG_CONFIG_HOME/selfishell/mise/config.toml" ||
    fail "mise configuration was not copied"
  cmp -s "$ROOT_DIR/common/completion.zsh" "$XDG_CONFIG_HOME/selfishell/zsh/completion.zsh" ||
    fail "Completion Zsh module was not copied"
  cmp -s "$ROOT_DIR/common/interactive.zsh" "$XDG_CONFIG_HOME/selfishell/zsh/interactive.zsh" ||
    fail "Interactive Zsh module was not copied"
  cmp -s "$ROOT_DIR/common/update-notice.zsh" "$XDG_CONFIG_HOME/selfishell/zsh/update-notice.zsh" ||
    fail "Update notice Zsh module was not copied"
  [[ -n "$(find "$HOME" -maxdepth 1 -name '.zshrc.backup.*' -print -quit)" ]] ||
    fail "Original Zsh configuration was not backed up"
  [[ "$(sed -n '6p' "$XDG_STATE_HOME/selfishell/resources/user-zshrc.state")" == "$HOME"/.zshrc.backup.* ]] ||
    fail "Zsh backup path was not recorded in state"

  state_count="$(find "$XDG_STATE_HOME/selfishell/resources" -type f -name '*.state' | wc -l)"
  # 13 zsh/starship/mise/aliases resources + 12 nvim file resources + 4 user link resources
  # = 29 state files for a fresh Ubuntu install (ghostty is macOS-only).
  [[ "$state_count" -eq 29 ]] || fail "Expected state for every managed Ubuntu resource (got $state_count)"
}

test_macos_install_includes_ghostty_configuration() {
  export SELFISHELL_TEST_SYSTEM_NAME=Darwin

  run_selfishell install --profile minimal --skip-packages --yes >/dev/null

  assert_symlink_to "$XDG_CONFIG_HOME/selfishell/ghostty/config" "$XDG_CONFIG_HOME/ghostty/config"
  cmp -s "$ROOT_DIR/mac/config.ghostty" "$XDG_CONFIG_HOME/selfishell/ghostty/config" ||
    fail "Ghostty configuration was not copied"
  assert_file_content '1' "$XDG_STATE_HOME/selfishell/ghostty"
}

test_macos_install_reuses_declined_ghostty_choice() {
  export SELFISHELL_TEST_SYSTEM_NAME=Darwin
  mkdir -p "$XDG_STATE_HOME/selfishell"
  printf '0\n' >"$XDG_STATE_HOME/selfishell/ghostty"

  run_selfishell install --profile minimal --skip-packages --yes >/dev/null

  [[ ! -e "$XDG_CONFIG_HOME/selfishell/ghostty/config" ]] ||
    fail "A saved declined Ghostty choice was ignored"
  assert_file_content '0' "$XDG_STATE_HOME/selfishell/ghostty"
}

test_local_zsh_extension_is_preserved() {
  local output

  mkdir -p "$XDG_CONFIG_HOME/selfishell"
  printf 'export SELFISHELL_COMPANY_TEST=loaded\n' >"$XDG_CONFIG_HOME/selfishell/local.zsh"
  run_selfishell install --skip-packages --yes >/dev/null

  output="$(HOME="$HOME" XDG_CONFIG_HOME="$XDG_CONFIG_HOME" zsh -dfc 'source "$HOME/.zshrc" >/dev/null 2>&1; print "$SELFISHELL_COMPANY_TEST"')"
  [[ "$output" == "loaded" ]] || fail "Local Zsh extension was not loaded"
  assert_file_content 'export SELFISHELL_COMPANY_TEST=loaded' "$XDG_CONFIG_HOME/selfishell/local.zsh"
}

test_install_is_idempotent() {
  local first_backup_count
  local second_backup_count

  printf 'original zshrc' >"$HOME/.zshrc"
  run_selfishell install --skip-packages --yes >/dev/null
  first_backup_count="$(find "$HOME" -name '*.backup.*' | wc -l)"
  run_selfishell install --skip-packages --yes >/dev/null
  second_backup_count="$(find "$HOME" -name '*.backup.*' | wc -l)"

  [[ "$second_backup_count" -eq "$first_backup_count" ]] ||
    fail "A second installation must not create more backups"
}

test_dry_run_changes_nothing() {
  local output

  printf 'original zshrc' >"$HOME/.zshrc"
  output="$(run_selfishell install --dry-run)"

  assert_file_content 'original zshrc' "$HOME/.zshrc"
  [[ ! -e "$XDG_CONFIG_HOME/selfishell" ]] || fail "Dry run created configuration"
  [[ ! -e "$XDG_STATE_HOME/selfishell" ]] || fail "Dry run created state"
  [[ "$output" == *'Dry run complete; no files were changed.'* ]] ||
    fail "Dry run summary was not printed"
}

test_noninteractive_install_requires_yes() {
  local status

  set +e
  run_selfishell install </dev/null >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -eq 2 ]] || fail "Non-interactive install should require --yes"
  [[ ! -e "$XDG_CONFIG_HOME/selfishell" ]] || fail "Rejected install changed files"
}

test_status_detects_modified_managed_file() {
  local status

  run_selfishell install --skip-packages --yes >/dev/null
  printf 'user modification' >"$XDG_CONFIG_HOME/selfishell/zsh/common.zsh"

  set +e
  run_selfishell status >/dev/null
  status=$?
  set -e

  [[ "$status" -eq 1 ]] || fail "Changed managed file should make status fail"
}

test_status_uses_current_resource_list() {
  local output

  run_selfishell install --skip-packages --yes >/dev/null
  output="$(run_selfishell status)"

  [[ "$output" == *'[OK] '"$XDG_CONFIG_HOME"'/selfishell/zsh/zshrc'* ]] ||
    fail "Status did not report the current Neovim resource list"
  [[ "$output" != *'vim-config'* ]] ||
    fail "Status still reports a removed legacy resource name"
}

test_uninstall_restores_original_files() {
  printf 'original zshrc' >"$HOME/.zshrc"
  run_selfishell install --skip-packages --yes >/dev/null
  run_selfishell uninstall --restore --yes >/dev/null

  assert_file_content 'original zshrc' "$HOME/.zshrc"
  [[ ! -e "$XDG_CONFIG_HOME/selfishell" ]] || fail "Managed configuration remains"
  [[ ! -e "$XDG_STATE_HOME/selfishell" ]] || fail "Managed state remains"
}

test_uninstall_dry_run_changes_nothing() {
  local state_count

  run_selfishell install --skip-packages --yes >/dev/null
  state_count="$(find "$XDG_STATE_HOME/selfishell/resources" -type f -name '*.state' | wc -l)"
  run_selfishell uninstall --restore --dry-run >/dev/null

  assert_symlink_to "$XDG_CONFIG_HOME/selfishell/zsh/zshrc" "$HOME/.zshrc"
  [[ "$(find "$XDG_STATE_HOME/selfishell/resources" -type f -name '*.state' | wc -l)" -eq "$state_count" ]] ||
    fail "Uninstall dry run changed state"
}

test_uninstall_preserves_user_modifications() {
  local status

  printf 'original zshrc' >"$HOME/.zshrc"
  run_selfishell install --skip-packages --yes >/dev/null
  printf 'user modification' >"$XDG_CONFIG_HOME/selfishell/zsh/zshrc"

  set +e
  run_selfishell uninstall --restore --yes >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -eq 1 ]] || fail "Modified managed configuration should block uninstall"
  assert_symlink_to "$XDG_CONFIG_HOME/selfishell/zsh/zshrc" "$HOME/.zshrc"
  assert_file_content 'user modification' "$XDG_CONFIG_HOME/selfishell/zsh/zshrc"
  [[ -n "$(find "$HOME" -maxdepth 1 -name '.zshrc.backup.*' -print -quit)" ]] ||
    fail "Original backup should be preserved after conflict"
}

test_pending_link_state_recovers_on_reinstall() {
  local state_file
  local temporary_state

  printf 'original zshrc' >"$HOME/.zshrc"
  run_selfishell install --skip-packages --yes >/dev/null
  state_file="$XDG_STATE_HOME/selfishell/resources/user-zshrc.state"
  temporary_state="${state_file}.test"
  awk 'NR == 3 { print "pending"; next } { print }' "$state_file" >"$temporary_state"
  mv "$temporary_state" "$state_file"
  rm "$HOME/.zshrc"

  run_selfishell install --skip-packages --yes >/dev/null
  assert_symlink_to "$XDG_CONFIG_HOME/selfishell/zsh/zshrc" "$HOME/.zshrc"
  [[ "$(sed -n '3p' "$state_file")" == "active" ]] || fail "Pending state was not completed"
}

test_pending_file_state_recovers_before_backup() {
  local target_file="$XDG_CONFIG_HOME/selfishell/zsh/common.zsh"
  local backup_file="${target_file}.backup.interrupted"
  local state_dir="$XDG_STATE_HOME/selfishell/resources"

  mkdir -p "$(dirname "$target_file")" "$state_dir"
  printf 'preexisting managed path' >"$target_file"
  {
    printf '1\nfile\npending\n%s\n-\n%s\n%s\n' \
      "$target_file" \
      "$backup_file" \
      "$(cksum <"$ROOT_DIR/common/common.zsh" | awk '{print $1 ":" $2}')"
  } >"$state_dir/zsh-common.state"

  run_selfishell install --skip-packages --yes >/dev/null

  assert_file_content 'preexisting managed path' "$backup_file"
  cmp -s "$ROOT_DIR/common/common.zsh" "$target_file" ||
    fail "Pending managed file installation did not resume"
  [[ "$(sed -n '3p' "$state_dir/zsh-common.state")" == "active" ]] ||
    fail "Pending file state was not completed"
}

test_install_does_not_depend_on_checkout() {
  local release_root="$TEST_ROOT/release"

  mkdir -p "$release_root"
  cp -R "$ROOT_DIR/bin" "$ROOT_DIR/lib" "$ROOT_DIR/profiles" "$ROOT_DIR/common" "$ROOT_DIR/mac" "$ROOT_DIR/ubuntu" "$release_root/"
  cp "$ROOT_DIR/VERSION" "$release_root/VERSION"

  bash "$release_root/bin/selfishell" install --skip-packages --yes >/dev/null
  rm -rf "$release_root"

  [[ -r "$HOME/.zshrc" ]] || fail "Zsh configuration broke after checkout removal"
  [[ "$(readlink "$HOME/.zshrc")" == "$XDG_CONFIG_HOME/selfishell/zsh/zshrc" ]] ||
    fail "User configuration still points to the checkout"
  [[ -r "$XDG_CONFIG_HOME/selfishell/zsh/common.zsh" ]] ||
    fail "Common configuration was not retained"
  [[ -r "$XDG_CONFIG_HOME/selfishell/zsh/update-notice.zsh" ]] ||
    fail "Common configuration modules were not retained"
  HOME="$HOME" XDG_CONFIG_HOME="$XDG_CONFIG_HOME" XDG_CACHE_HOME="$XDG_CACHE_HOME" \
    PATH="/usr/bin:/bin" zsh -dfc 'source "$HOME/.zshrc"' >/dev/null 2>&1 ||
    fail "Zsh configuration depended on the removed checkout"
}

run_test() {
  local test_name="$1"

  setup_managed_home
  trap 'teardown_managed_home' RETURN
  "$test_name"
  trap - RETURN
  teardown_managed_home
  printf 'PASS: %s\n' "$test_name"
}

main() {
  local test_name
  local failures=0

  while IFS= read -r test_name; do
    if ! run_test "$test_name"; then
      failures=$((failures + 1))
    fi
  done < <(declare -F | awk '{print $3}' | grep '^test_' | sort)

  if ((failures > 0)); then
    printf '%d test(s) failed\n' "$failures" >&2
    return 1
  fi
}

main "$@"
