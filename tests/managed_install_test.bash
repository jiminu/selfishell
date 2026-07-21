#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

source "$ROOT_DIR/tests/test_helper.bash"

setup_managed_home() {
  setup_test_home
  export ORIGINAL_TEST_PATH="$PATH"
  export XDG_CONFIG_HOME="$HOME/.config"
  export XDG_STATE_HOME="$HOME/.local/state"
  export XDG_CACHE_HOME="$HOME/.cache"
  export SELFISHELL_TEST_SYSTEM_NAME=Linux
  export SELFISHELL_TEST_MACHINE_ARCH=x86_64
  export SELFISHELL_TEST_OS_RELEASE_FILE="$TEST_ROOT/os-release"
  export SELFISHELL_TEST_PROC_VERSION_FILE="$TEST_ROOT/proc-version"
  printf 'ID=ubuntu\n' >"$SELFISHELL_TEST_OS_RELEASE_FILE"
  printf 'Linux microsoft WSL2\n' >"$SELFISHELL_TEST_PROC_VERSION_FILE"

  mkdir -p "$TEST_ROOT/bin"
  printf '#!/usr/bin/env bash\nexit 0\n' >"$TEST_ROOT/bin/chsh"
  chmod +x "$TEST_ROOT/bin/chsh"
  export PATH="$TEST_ROOT/bin:$PATH"

  # Conflict tests below assert directly against $SELFISHELL_STATE_DIR (e.g.
  # its backups/ subdirectory and resource state files). These globals are
  # derived purely from the XDG_*/HOME values exported above, so scoping the
  # sourcing to this file keeps every other test's HOME/XDG isolation intact.
  source "$ROOT_DIR/lib/paths.sh"
  selfishell_initialize_paths
  export SELFISHELL_CONFIG_DIR SELFISHELL_STATE_DIR SELFISHELL_CACHE_DIR SELFISHELL_RESOURCE_STATE_DIR
}

teardown_managed_home() {
  if [[ -n "${ORIGINAL_TEST_PATH:-}" ]]; then
    export PATH="$ORIGINAL_TEST_PATH"
    unset ORIGINAL_TEST_PATH
  fi
  unset XDG_CONFIG_HOME XDG_STATE_HOME XDG_CACHE_HOME
  unset SELFISHELL_TEST_SYSTEM_NAME SELFISHELL_TEST_MACHINE_ARCH
  unset SELFISHELL_TEST_OS_RELEASE_FILE SELFISHELL_TEST_PROC_VERSION_FILE
  unset SELFISHELL_CONFIG_DIR SELFISHELL_STATE_DIR SELFISHELL_CACHE_DIR SELFISHELL_RESOURCE_STATE_DIR
  teardown_test_home
}

run_selfishell() {
  bash "$ROOT_DIR/bin/selfishell" "$@"
}

test_every_neovim_configuration_file_is_managed() {
  local declared_sources
  local source_file

  declared_sources="$(
    SELFISHELL_CONFIG_DIR="$XDG_CONFIG_HOME/selfishell" SELFISHELL_ROOT="$ROOT_DIR" \
      bash -c 'source "$1/lib/resources.sh"; selfishell_managed_resources' _ "$ROOT_DIR" |
      cut -f4
  )"

  while IFS= read -r source_file; do
    if ! grep -Fqx "$source_file" <<<"$declared_sources"; then
      fail "Neovim configuration file is not a managed resource: $source_file"
    fi
  done < <(find "$ROOT_DIR/common/nvim" -type f -print | sort)
}

test_install_copies_configuration_and_tracks_resources() {
  local state_count

  printf 'original zshrc' >"$HOME/.zshrc"
  run_selfishell install --skip-packages --yes >/dev/null

  [[ -f "$HOME/.zshrc" && ! -L "$HOME/.zshrc" ]] || fail "Zsh startup file is not user-owned"
  grep -Fqx '# >>> Selfishell initialize >>>' "$HOME/.zshrc" || fail "Zsh loader start marker is missing"
  grep -Fqx 'original zshrc' "$HOME/.zshrc" || fail "Original Zsh configuration was not preserved"
  assert_symlink_to "$XDG_CONFIG_HOME/selfishell/zsh/zshenv" "$HOME/.zshenv"
  assert_symlink_to "$XDG_CONFIG_HOME/selfishell/starship.toml" "$XDG_CONFIG_HOME/starship.toml"
  assert_symlink_to "$XDG_CONFIG_HOME/selfishell/vim/vimrc" "$XDG_CONFIG_HOME/vim/vimrc"
  cmp -s "$ROOT_DIR/common/common.zsh" "$XDG_CONFIG_HOME/selfishell/zsh/common.zsh" ||
    fail "Common Zsh configuration was not copied"
  cmp -s "$ROOT_DIR/common/zshenv" "$XDG_CONFIG_HOME/selfishell/zsh/zshenv" ||
    fail "zshenv was not copied"
  cmp -s "$ROOT_DIR/common/runtime.zsh" "$XDG_CONFIG_HOME/selfishell/zsh/runtime.zsh" ||
    fail "Runtime Zsh module was not copied"
  cmp -s "$ROOT_DIR/common/mise.toml" "$XDG_CONFIG_HOME/selfishell/mise/selfishell.toml" ||
    fail "mise configuration was not copied"
  cmp -s "$ROOT_DIR/common/vimrc" "$XDG_CONFIG_HOME/selfishell/vim/vimrc" ||
    fail "Vim configuration was not copied"
  cmp -s "$ROOT_DIR/common/completion.zsh" "$XDG_CONFIG_HOME/selfishell/zsh/completion.zsh" ||
    fail "Completion Zsh module was not copied"
  cmp -s "$ROOT_DIR/common/interactive.zsh" "$XDG_CONFIG_HOME/selfishell/zsh/interactive.zsh" ||
    fail "Interactive Zsh module was not copied"
  cmp -s "$ROOT_DIR/common/update-notice.zsh" "$XDG_CONFIG_HOME/selfishell/zsh/update-notice.zsh" ||
    fail "Update notice Zsh module was not copied"
  [[ "$(sed -n '1p' "$XDG_STATE_HOME/selfishell/resources/user-zshrc.state")" == 2 ]] ||
    fail "Zsh loader state version was not recorded"
  [[ "$(sed -n '2p' "$XDG_STATE_HOME/selfishell/resources/user-zshrc.state")" == block ]] ||
    fail "Zsh loader was not recorded as a managed block"

  state_count="$(find "$XDG_STATE_HOME/selfishell/resources" -type f -name '*.state' | wc -l)"
  # 14 zsh/starship/mise/vim resources + 4 user link resources
  # = 18 state files for a fresh Ubuntu minimal install (ghostty and nvim are developer-only).
  [[ "$state_count" -eq 18 ]] || fail "Expected state for every managed Ubuntu minimal resource (got $state_count)"
}

test_install_switches_login_shell_to_zsh() {
  local fake_bin
  local chsh_arguments

  fake_bin="$TEST_ROOT/bin"
  mkdir -p "$fake_bin"
  cat >"$fake_bin/chsh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >"$HOME/chsh-args"
EOF
  chmod +x "$fake_bin/chsh"

  printf 'original zshrc' >"$HOME/.zshrc"
  export SHELL="/bin/bash"
  PATH="$fake_bin:/usr/bin:/bin" run_selfishell install --skip-packages --yes >/dev/null

  chsh_arguments="$(<"$HOME/chsh-args")"
  [[ "$chsh_arguments" == -s* ]] || fail "Install did not request a Zsh login shell"
  [[ "$chsh_arguments" == *zsh* ]] || fail "Install did not request a Zsh login shell"
}

test_developer_install_includes_neovim_configuration() {
  printf 'original zshrc' >"$HOME/.zshrc"
  run_selfishell install --profile developer --skip-packages --yes >/dev/null

  assert_symlink_to "$XDG_CONFIG_HOME/selfishell/nvim" "$XDG_CONFIG_HOME/nvim"
  assert_symlink_to "$XDG_CONFIG_HOME/selfishell/mise/selfishell.toml" "$XDG_CONFIG_HOME/mise/conf.d/selfishell.toml"
  cmp -s "$ROOT_DIR/common/nvim/init.lua" "$XDG_CONFIG_HOME/selfishell/nvim/init.lua" ||
    fail "Neovim init.lua was not installed for the developer profile"
  cmp -s "$ROOT_DIR/common/nvim/lua/config/options.lua" "$XDG_CONFIG_HOME/selfishell/nvim/lua/config/options.lua" ||
    fail "Neovim options module was not installed for the developer profile"
  cmp -s "$ROOT_DIR/common/nvim/lua/config/treesitter.lua" "$XDG_CONFIG_HOME/selfishell/nvim/lua/config/treesitter.lua" ||
    fail "Neovim Tree-sitter module was not installed for the developer profile"
  cmp -s "$ROOT_DIR/common/nvim/lua/plugins/lsp.lua" "$XDG_CONFIG_HOME/selfishell/nvim/lua/plugins/lsp.lua" ||
    fail "Neovim lsp plugin was not installed for the developer profile"
}

test_macos_install_includes_ghostty_configuration() {
  export SELFISHELL_TEST_SYSTEM_NAME=Darwin
  mkdir -p "$XDG_CONFIG_HOME/ghostty"
  printf 'font-size = 14\n' >"$XDG_CONFIG_HOME/ghostty/config"

  run_selfishell install --profile minimal --skip-packages --yes >/dev/null

  [[ -f "$XDG_CONFIG_HOME/ghostty/config" && ! -L "$XDG_CONFIG_HOME/ghostty/config" ]] ||
    fail "Ghostty config is not user-owned"
  grep -Fqx '# >>> Selfishell ghostty >>>' "$XDG_CONFIG_HOME/ghostty/config" ||
    fail "Ghostty include start marker is missing"
  grep -Fqx "config-file = $XDG_CONFIG_HOME/selfishell/ghostty/config" "$XDG_CONFIG_HOME/ghostty/config" ||
    fail "Ghostty include line is missing"
  grep -Fqx 'font-size = 14' "$XDG_CONFIG_HOME/ghostty/config" ||
    fail "Original Ghostty configuration was not preserved"
  cmp -s "$ROOT_DIR/mac/config.ghostty" "$XDG_CONFIG_HOME/selfishell/ghostty/config" ||
    fail "Ghostty configuration was not copied"
  assert_file_content '1' "$XDG_STATE_HOME/selfishell/ghostty"
}

test_legacy_ghostty_link_state_is_rejected_without_changes() {
  export SELFISHELL_TEST_SYSTEM_NAME=Darwin
  local target="$XDG_CONFIG_HOME/ghostty/config"
  local managed_source="$XDG_CONFIG_HOME/selfishell/ghostty/config"
  local state_file="$XDG_STATE_HOME/selfishell/resources/user-ghostty.state"
  local status

  run_selfishell install --profile minimal --skip-packages --yes >/dev/null

  rm "$target"
  ln -s "$managed_source" "$target"
  printf '2\nlink\nactive\n%s\n%s\n-\n-\n' "$target" "$managed_source" >"$state_file"

  set +e
  run_selfishell install --profile minimal --skip-packages --yes >/dev/null 2>"$TEST_ROOT/stderr"
  status=$?
  set -e

  [[ "$status" -eq 1 ]] || fail "Legacy Ghostty link state should stop installation"
  assert_symlink_to "$managed_source" "$target"
  [[ "$(sed -n '2p' "$state_file")" == link ]] || fail "Legacy state was changed"
  grep -Fq "Run 'selfishell uninstall --restore --yes', then reinstall." "$TEST_ROOT/stderr" ||
    fail "Legacy state error did not explain how to recover"
}

test_legacy_ghostty_link_can_be_uninstalled_for_manual_transition() {
  export SELFISHELL_TEST_SYSTEM_NAME=Darwin
  local target="$XDG_CONFIG_HOME/ghostty/config"
  local managed_source="$XDG_CONFIG_HOME/selfishell/ghostty/config"
  local state_file="$XDG_STATE_HOME/selfishell/resources/user-ghostty.state"

  run_selfishell install --profile minimal --skip-packages --yes >/dev/null

  rm "$target"
  ln -s "$managed_source" "$target"
  printf '2\nlink\nactive\n%s\n%s\n-\n-\n' "$target" "$managed_source" >"$state_file"

  run_selfishell uninstall --yes >/dev/null
  [[ ! -e "$state_file" ]] || fail "Legacy Ghostty link state was not removed"
  [[ ! -e "$target" ]] || fail "Legacy Ghostty link was not removed"

  run_selfishell install --profile minimal --skip-packages --yes >/dev/null

  [[ -f "$target" && ! -L "$target" ]] || fail "Reinstall did not create a user-owned Ghostty config"
  grep -Fqx '# >>> Selfishell ghostty >>>' "$target" || fail "Reinstalled Ghostty config is missing the include block"
  [[ "$(sed -n '2p' "$state_file")" == block ]] || fail "Reinstalled Ghostty resource was not recorded as a block"
}

test_macos_install_reuses_declined_ghostty_choice() {
  export SELFISHELL_TEST_SYSTEM_NAME=Darwin
  mkdir -p "$XDG_STATE_HOME/selfishell"
  printf '0\n' >"$XDG_STATE_HOME/selfishell/ghostty"

  run_selfishell install --profile minimal --skip-packages --yes >/dev/null

  [[ ! -e "$XDG_CONFIG_HOME/selfishell/ghostty/config" ]] ||
    fail "A saved declined Ghostty choice was ignored"
  [[ ! -e "$XDG_CONFIG_HOME/ghostty/config" ]] ||
    fail "A saved declined Ghostty choice created a user config file"
  assert_file_content '0' "$XDG_STATE_HOME/selfishell/ghostty"
}

test_user_ghostty_changes_survive_reinstall_and_uninstall_exactly() {
  export SELFISHELL_TEST_SYSTEM_NAME=Darwin
  local target="$XDG_CONFIG_HOME/ghostty/config"
  local prefix="$TEST_ROOT/ghostty-prefix"
  local suffix="$TEST_ROOT/ghostty-suffix"
  local expected="$TEST_ROOT/expected-ghostty-config"
  local modified="$TEST_ROOT/modified-ghostty-config"

  mkdir -p "$(dirname "$target")"
  printf 'font-size = 14\n' >"$target"
  printf 'window-padding-x = 20\n' >"$prefix"
  printf '\ncursor-style = bar\n' >"$suffix"
  cat "$prefix" "$target" "$suffix" >"$expected"

  run_selfishell install --profile minimal --skip-packages --yes >/dev/null
  cat "$prefix" "$target" >"$modified"
  mv "$modified" "$target"
  cat "$suffix" >>"$target"

  run_selfishell update --tools-only --dry-run >/dev/null
  run_selfishell install --profile minimal --skip-packages --yes >/dev/null
  run_selfishell uninstall --yes >/dev/null

  cmp -s "$expected" "$target" || fail "Uninstall changed user Ghostty config bytes outside the block"
}

test_local_zsh_extension_is_retired_and_preserved() {
  local output

  mkdir -p "$XDG_CONFIG_HOME/selfishell"
  printf 'export SELFISHELL_COMPANY_TEST=loaded\n' >"$XDG_CONFIG_HOME/selfishell/local.zsh"
  run_selfishell install --skip-packages --yes >/dev/null

  output="$(HOME="$HOME" XDG_CONFIG_HOME="$XDG_CONFIG_HOME" zsh -dfc 'source "$HOME/.zshrc" >/dev/null 2>&1; print "${SELFISHELL_COMPANY_TEST-}"')"
  [[ -z "$output" ]] || fail "Retired local.zsh was still loaded"
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
  [[ "$(grep -Fc '# >>> Selfishell initialize >>>' "$HOME/.zshrc")" -eq 1 ]] ||
    fail "A second installation duplicated the Zsh loader"
}

test_user_zsh_changes_survive_reinstall_and_uninstall_exactly() {
  local expected="$TEST_ROOT/expected-zshrc"
  local modified="$TEST_ROOT/modified-zshrc"

  printf 'export BEFORE=1\r\nalias tail=true' >"$HOME/.zshrc"
  run_selfishell install --skip-packages --yes >/dev/null
  printf 'alias PREFIX=true\n' >"$modified"
  cat "$HOME/.zshrc" >>"$modified"
  mv "$modified" "$HOME/.zshrc"
  printf '\nexport AFTER=1' >>"$HOME/.zshrc"
  printf 'alias PREFIX=true\nexport BEFORE=1\r\nalias tail=true\nexport AFTER=1' >"$expected"

  run_selfishell update --tools-only --dry-run >/dev/null
  run_selfishell install --skip-packages --yes >/dev/null
  run_selfishell uninstall --yes >/dev/null

  cmp -s "$expected" "$HOME/.zshrc" || fail "Uninstall changed user Zsh bytes outside the loader"
}

test_malformed_loader_is_rejected_without_changes() {
  local original="$TEST_ROOT/original-zshrc"
  local status

  printf '# >>> Selfishell initialize >>>\nuser content\n' >"$HOME/.zshrc"
  cp "$HOME/.zshrc" "$original"

  set +e
  run_selfishell install --skip-packages --yes >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -eq 1 ]] || fail "Malformed loader should stop installation"
  cmp -s "$original" "$HOME/.zshrc" || fail "Rejected malformed loader was changed"
  [[ ! -e "$XDG_CONFIG_HOME/selfishell" ]] || fail "Rejected loader created configuration"
  [[ ! -e "$XDG_STATE_HOME/selfishell" ]] || fail "Rejected loader created state"
}

test_unrelated_zshrc_symlink_is_rejected_without_changes() {
  local status

  printf 'dotfiles zshrc\n' >"$TEST_ROOT/dotfiles-zshrc"
  ln -s "$TEST_ROOT/dotfiles-zshrc" "$HOME/.zshrc"

  set +e
  run_selfishell install --skip-packages --yes >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -eq 1 ]] || fail "Zsh symlink should stop installation"
  assert_symlink_to "$TEST_ROOT/dotfiles-zshrc" "$HOME/.zshrc"
  assert_file_content 'dotfiles zshrc' "$TEST_ROOT/dotfiles-zshrc"
  [[ ! -e "$XDG_CONFIG_HOME/selfishell" ]] || fail "Rejected symlink created configuration"
  [[ ! -e "$XDG_STATE_HOME/selfishell" ]] || fail "Rejected symlink created state"
}

test_legacy_zshrc_state_is_rejected_without_changes() {
  local state_file="$XDG_STATE_HOME/selfishell/resources/user-zshrc.state"
  local status

  mkdir -p "$(dirname "$state_file")" "$XDG_CONFIG_HOME/selfishell/zsh"
  printf 'legacy managed zshrc\n' >"$XDG_CONFIG_HOME/selfishell/zsh/zshrc"
  ln -s "$XDG_CONFIG_HOME/selfishell/zsh/zshrc" "$HOME/.zshrc"
  printf '1\nlink\nactive\n%s\n%s\n-\n-\n' \
    "$HOME/.zshrc" "$XDG_CONFIG_HOME/selfishell/zsh/zshrc" >"$state_file"

  set +e
  run_selfishell install --skip-packages --yes >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -eq 1 ]] || fail "Legacy Zsh state should stop installation"
  assert_symlink_to "$XDG_CONFIG_HOME/selfishell/zsh/zshrc" "$HOME/.zshrc"
  [[ "$(sed -n '1p' "$state_file")" == 1 ]] || fail "Legacy state was changed"
}

test_legacy_zshrc_can_be_uninstalled_for_manual_transition() {
  local backup="$HOME/.zshrc.backup.legacy"
  local state_file="$XDG_STATE_HOME/selfishell/resources/user-zshrc.state"

  mkdir -p "$(dirname "$state_file")" "$XDG_CONFIG_HOME/selfishell/zsh"
  printf 'restored user zshrc\n' >"$backup"
  printf 'legacy managed zshrc\n' >"$XDG_CONFIG_HOME/selfishell/zsh/zshrc"
  ln -s "$XDG_CONFIG_HOME/selfishell/zsh/zshrc" "$HOME/.zshrc"
  printf '1\nlink\nactive\n%s\n%s\n%s\n-\n' \
    "$HOME/.zshrc" "$XDG_CONFIG_HOME/selfishell/zsh/zshrc" "$backup" >"$state_file"

  run_selfishell uninstall --restore --yes >/dev/null

  assert_file_content 'restored user zshrc' "$HOME/.zshrc"
  [[ ! -e "$state_file" ]] || fail "Legacy Zsh link state was not removed"
}

test_untracked_and_duplicate_loaders_are_rejected() {
  local loader="$TEST_ROOT/loader"
  local status

  bash -c 'source "$1/lib/managed.sh"; managed_block_content user-zshrc' _ "$ROOT_DIR" >"$loader"
  cp "$loader" "$HOME/.zshrc"

  set +e
  run_selfishell install --skip-packages --yes >/dev/null 2>&1
  status=$?
  set -e
  [[ "$status" -eq 1 ]] || fail "Untracked loader should stop installation"

  cat "$loader" "$loader" >"$HOME/.zshrc"
  set +e
  run_selfishell install --skip-packages --yes >/dev/null 2>&1
  status=$?
  set -e
  [[ "$status" -eq 1 ]] || fail "Duplicate loaders should stop installation"
  [[ "$(grep -Fc '# >>> Selfishell initialize >>>' "$HOME/.zshrc")" -eq 2 ]] ||
    fail "Rejected duplicate loaders were changed"
}

test_zshrc_directory_is_rejected_without_changes() {
  local status

  mkdir "$HOME/.zshrc"
  set +e
  run_selfishell install --skip-packages --yes >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -eq 1 ]] || fail "Zsh startup directory should stop installation"
  [[ -d "$HOME/.zshrc" ]] || fail "Rejected Zsh startup directory was changed"
  [[ ! -e "$XDG_CONFIG_HOME/selfishell" ]] || fail "Rejected directory created configuration"
  [[ ! -e "$XDG_STATE_HOME/selfishell" ]] || fail "Rejected directory created state"
}

test_modified_loader_blocks_reinstall_and_uninstall() {
  local modified="$TEST_ROOT/modified-zshrc"
  local status

  run_selfishell install --skip-packages --yes >/dev/null
  sed 's/# <<< Selfishell initialize <<</# <<< Selfishell initialize changed <<</' \
    "$HOME/.zshrc" >"$modified"
  mv "$modified" "$HOME/.zshrc"

  set +e
  run_selfishell install --skip-packages --yes >/dev/null 2>&1
  status=$?
  set -e
  [[ "$status" -eq 1 ]] || fail "Modified loader should block reinstall"

  set +e
  run_selfishell uninstall --yes >/dev/null 2>&1
  status=$?
  set -e
  [[ "$status" -eq 1 ]] || fail "Modified loader should block uninstall"
  grep -Fqx '# <<< Selfishell initialize changed <<<' "$HOME/.zshrc" ||
    fail "Modified loader was not preserved"
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
  [[ "$output" == *'[OK] '"$XDG_CONFIG_HOME"'/selfishell/vim/vimrc'* ]] ||
    fail "Status did not report the Vim resource list"
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

  [[ -f "$HOME/.zshrc" && ! -L "$HOME/.zshrc" ]] || fail "Uninstall dry run changed .zshrc type"
  grep -Fqx '# >>> Selfishell initialize >>>' "$HOME/.zshrc" || fail "Uninstall dry run removed the loader"
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
  [[ -f "$HOME/.zshrc" && ! -L "$HOME/.zshrc" ]] || fail "Rejected uninstall changed .zshrc type"
  assert_file_content 'user modification' "$XDG_CONFIG_HOME/selfishell/zsh/zshrc"
}

test_uninstall_preserves_state_when_removal_fails() {
  local fake_bin="$TEST_ROOT/bin"
  local status

  run_selfishell install --skip-packages --yes >/dev/null
  mkdir -p "$fake_bin"
  cat >"$fake_bin/rm" <<'EOF'
#!/usr/bin/env bash
for argument in "$@"; do
  case "$argument" in
    */selfishell/zsh/zshrc) exit 1 ;;
  esac
done
exec /bin/rm "$@"
EOF
  chmod +x "$fake_bin/rm"

  set +e
  PATH="$fake_bin:/usr/bin:/bin" run_selfishell uninstall --yes >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -eq 1 ]] || fail "A managed resource removal failure should fail uninstall"
  [[ -r "$XDG_CONFIG_HOME/selfishell/zsh/zshrc" ]] ||
    fail "Failed managed file removal was not preserved"
  [[ -r "$XDG_STATE_HOME/selfishell/resources/zshrc-config.state" ]] ||
    fail "Failed managed resource state was removed"
  assert_file_content 'minimal' "$XDG_STATE_HOME/selfishell/profile"
}

test_pending_loader_state_recovers_on_reinstall() {
  local state_file
  local checksum

  printf 'original zshrc' >"$HOME/.zshrc"
  mkdir -p "$XDG_STATE_HOME/selfishell/resources"
  state_file="$XDG_STATE_HOME/selfishell/resources/user-zshrc.state"
  checksum="$(bash -c 'source "$1/lib/managed.sh"; managed_block_content user-zshrc' _ "$ROOT_DIR" | cksum | awk '{print $1 ":" $2}')"
  printf '2\nblock\npending\n%s\nselfishell-zsh-loader-v1\n-\n%s\n' "$HOME/.zshrc" "$checksum" >"$state_file"

  run_selfishell install --skip-packages --yes >/dev/null
  [[ -f "$HOME/.zshrc" && ! -L "$HOME/.zshrc" ]] || fail "Pending loader did not create regular .zshrc"
  grep -Fqx 'original zshrc' "$HOME/.zshrc" || fail "Pending loader recovery lost user content"
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
  [[ ! -L "$HOME/.zshrc" ]] || fail "User Zsh configuration is still a managed link"
  [[ -r "$XDG_CONFIG_HOME/selfishell/zsh/common.zsh" ]] ||
    fail "Common configuration was not retained"
  [[ -r "$XDG_CONFIG_HOME/selfishell/zsh/update-notice.zsh" ]] ||
    fail "Common configuration modules were not retained"
  [[ -r "$XDG_CONFIG_HOME/selfishell/vim/vimrc" ]] ||
    fail "Vim configuration was not retained"
  HOME="$HOME" XDG_CONFIG_HOME="$XDG_CONFIG_HOME" XDG_CACHE_HOME="$XDG_CACHE_HOME" \
    PATH="/usr/bin:/bin" zsh -dfc 'source "$HOME/.zshrc"' >/dev/null 2>&1 ||
    fail "Zsh configuration depended on the removed checkout"
}

test_mise_config_global_creation_and_no_state() {
  run_selfishell install --profile developer --skip-packages --yes >/dev/null
  [[ -f "$XDG_CONFIG_HOME/mise/config.toml" ]] || fail "config.toml was not created on developer install"
  [[ ! -f "$XDG_STATE_HOME/selfishell/resources/mise-config-global.state" ]] || fail "mise-config-global state should not exist"
}

test_mise_config_global_minimal_profile() {
  run_selfishell install --profile minimal --skip-packages --yes >/dev/null
  [[ ! -e "$XDG_CONFIG_HOME/mise/config.toml" ]] || fail "config.toml should not be created for minimal profile"
}

test_mise_config_global_preserves_existing_types() {
  # 일반 파일
  mkdir -p "$XDG_CONFIG_HOME/mise"
  printf 'user_owned_data_content_bytes\n' >"$XDG_CONFIG_HOME/mise/config.toml"
  run_selfishell install --profile developer --skip-packages --yes >/dev/null
  assert_file_content 'user_owned_data_content_bytes' "$XDG_CONFIG_HOME/mise/config.toml"

  # 일반 symlink
  rm -f "$XDG_CONFIG_HOME/mise/config.toml"
  printf 'link_target_content\n' >"$TEST_ROOT/real_config.toml"
  ln -s "$TEST_ROOT/real_config.toml" "$XDG_CONFIG_HOME/mise/config.toml"
  run_selfishell install --profile developer --skip-packages --yes >/dev/null
  assert_symlink_to "$TEST_ROOT/real_config.toml" "$XDG_CONFIG_HOME/mise/config.toml"

  # symlink-to-directory
  rm -f "$XDG_CONFIG_HOME/mise/config.toml"
  mkdir -p "$TEST_ROOT/some_dir"
  ln -s "$TEST_ROOT/some_dir" "$XDG_CONFIG_HOME/mise/config.toml"
  run_selfishell install --profile developer --skip-packages --yes >/dev/null
  assert_symlink_to "$TEST_ROOT/some_dir" "$XDG_CONFIG_HOME/mise/config.toml"

  # symlink-to-special (dangling)
  rm -rf "$TEST_ROOT/some_dir"
  run_selfishell install --profile developer --skip-packages --yes >/dev/null
  [[ -L "$XDG_CONFIG_HOME/mise/config.toml" ]] || fail "dangling symlink was removed"
  [[ "$(readlink "$XDG_CONFIG_HOME/mise/config.toml")" == "$TEST_ROOT/some_dir" ]] || fail "dangling symlink target changed"
}

test_mise_config_global_idempotency_and_status() {
  local tool
  for tool in zsh git curl ca-certificates vim starship fzf zoxide rg jq build-essential mise neovim tree-sitter node python uv; do
    printf '#!/usr/bin/env bash\nexit 0\n' >"$TEST_ROOT/bin/$tool"
    chmod +x "$TEST_ROOT/bin/$tool"
  done
  mkdir -p "$HOME/.local/share/zinit/zinit.git"
  touch "$HOME/.local/share/zinit/zinit.git/zinit.zsh"

  run_selfishell install --profile developer --skip-packages --yes >/dev/null
  printf 'modified by user 123\n' >"$XDG_CONFIG_HOME/mise/config.toml"
  run_selfishell install --profile developer --skip-packages --yes >/dev/null
  assert_file_content 'modified by user 123' "$XDG_CONFIG_HOME/mise/config.toml"

  local status_out
  local status=0
  status_out="$(run_selfishell status 2>&1)" || status=$?
  ((status == 0)) || fail "status failed after user modified config.toml (exit code $status)"
  [[ "$status_out" != *'config.toml'* ]] || fail "user-owned config.toml should not be reported by status"
}

test_mise_config_global_uninstall_preservation() {
  run_selfishell install --profile developer --skip-packages --yes >/dev/null
  run_selfishell uninstall --restore --yes >/dev/null
  [[ -f "$XDG_CONFIG_HOME/mise/config.toml" ]] || fail "config.toml should remain after uninstall"

  mkdir -p "$XDG_CONFIG_HOME/mise"
  printf 'pre_existing_data\n' >"$XDG_CONFIG_HOME/mise/config.toml"
  run_selfishell install --profile developer --skip-packages --yes >/dev/null
  run_selfishell uninstall --restore --yes >/dev/null
  assert_file_content 'pre_existing_data' "$XDG_CONFIG_HOME/mise/config.toml"

  : >"$XDG_CONFIG_HOME/mise/config.toml"
  run_selfishell install --profile developer --skip-packages --yes >/dev/null
  run_selfishell uninstall --restore --yes >/dev/null
  [[ -f "$XDG_CONFIG_HOME/mise/config.toml" ]] || fail "empty config.toml was deleted on uninstall"
}

test_mise_config_global_dry_run_and_directory_error() {
  run_selfishell install --profile developer --skip-packages --dry-run --yes >/dev/null
  [[ ! -e "$XDG_CONFIG_HOME/mise/config.toml" ]] || fail "dry-run created config.toml"

  mkdir -p "$XDG_CONFIG_HOME/mise/config.toml"
  local rc=0
  run_selfishell install --profile developer --skip-packages --yes >/dev/null 2>&1 || rc=$?
  ((rc != 0)) || fail "install did not return error when config.toml is a directory"

  [[ -d "$XDG_CONFIG_HOME/mise/config.toml" ]] || fail "invalid existing directory was changed"
  [[ ! -e "$XDG_CONFIG_HOME/selfishell" ]] || fail "preflight failure created Selfishell configuration"
  [[ ! -e "$XDG_STATE_HOME/selfishell" ]] || fail "preflight failure created Selfishell state"
  [[ ! -L "$XDG_CONFIG_HOME/mise/conf.d/selfishell.toml" ]] || fail "preflight failure created the managed mise link"
}

test_mise_global_config_env_runtime() {
  local fake_bin="$TEST_ROOT/bin"
  local output

  mkdir -p "$fake_bin"

  cat >"$fake_bin/mise" <<'EOF'
#!/usr/bin/env sh
if [ "${1:-}" = "activate" ]; then
  printf ':\n'
fi
EOF
  chmod +x "$fake_bin/mise"

  # shellcheck disable=SC2016
  output="$(
    PATH="$fake_bin:/usr/bin:/bin" \
      MISE_GLOBAL_CONFIG_FILE="$HOME/personal-mise.toml" \
      zsh -dfc '
        _selfishell_command_path() { command -v "$1"; }
        source "$1"
        print -r -- "${MISE_GLOBAL_CONFIG_FILE-}"
      ' zsh "$ROOT_DIR/common/runtime.zsh"
  )"

  [[ "$output" == "$HOME/personal-mise.toml" ]] ||
    fail "runtime modified caller-provided MISE_GLOBAL_CONFIG_FILE"

  # shellcheck disable=SC2016
  output="$(
    env -u MISE_GLOBAL_CONFIG_FILE \
      PATH="$fake_bin:/usr/bin:/bin" \
      zsh -dfc '
        _selfishell_command_path() { command -v "$1"; }
        source "$1"
        [[ -z "${MISE_GLOBAL_CONFIG_FILE+x}" ]]
      ' zsh "$ROOT_DIR/common/runtime.zsh"
  )" || fail "runtime created MISE_GLOBAL_CONFIG_FILE"
}

test_mise_global_config_ownership() {
  run_selfishell install --profile developer --skip-packages --yes >/dev/null
  printf 'node = "24"\n' >>"$XDG_CONFIG_HOME/mise/config.toml"
  local selfishell_toml_content
  selfishell_toml_content="$(<"$XDG_CONFIG_HOME/selfishell/mise/selfishell.toml")"
  [[ "$selfishell_toml_content" != *'node = "24"'* ]] || fail "Selfishell default configuration was mutated by user global config write"
}

# Real (non-dry-run, non-`--skip-packages`) `update` calls reach
# packages_install_profile, which must not touch the network or need root in
# tests. Faking apt-get/dpkg satisfies the apt requirement check without
# `sudo`, and pre-creating the "direct" dependency targets (starship, zinit)
# makes dependency_install treat them as already present. This is host- and
# platform-independent: it works whether the test runs on the Ubuntu or
# macOS CI runner, since it never depends on a real apt-get or network path.
setup_fake_minimal_packages() {
  mkdir -p "$TEST_ROOT/bin"
  printf '#!/usr/bin/env bash\nexit 0\n' >"$TEST_ROOT/bin/apt-get"
  chmod +x "$TEST_ROOT/bin/apt-get"
  printf '#!/usr/bin/env bash\nexit 0\n' >"$TEST_ROOT/bin/dpkg"
  chmod +x "$TEST_ROOT/bin/dpkg"

  mkdir -p "$HOME/.local/bin"
  printf '#!/usr/bin/env bash\nexit 0\n' >"$HOME/.local/bin/starship"
  chmod +x "$HOME/.local/bin/starship"
  mkdir -p "$HOME/.local/share/zinit/zinit.git"
  touch "$HOME/.local/share/zinit/zinit.git/zinit.zsh"
}

# Copies the checkout into its own root so a test can change a managed
# resource's *source* file (to simulate a new Selfishell release) without
# mutating the real repository under test.
build_release_copy() {
  local release_root="$1"

  mkdir -p "$release_root"
  cp -R "$ROOT_DIR/bin" "$ROOT_DIR/lib" "$ROOT_DIR/profiles" "$ROOT_DIR/common" \
    "$ROOT_DIR/mac" "$ROOT_DIR/ubuntu" "$release_root/"
  cp "$ROOT_DIR/VERSION" "$release_root/VERSION"
  cp "$ROOT_DIR/dependencies.conf" "$release_root/dependencies.conf"
}

test_managed_file_interactive_overwrite_yes() {
  run_selfishell install --profile minimal --skip-packages --yes >/dev/null

  local target_file="$XDG_CONFIG_HOME/selfishell/vim/vimrc"
  printf 'user_modified_data\n' >"$target_file"

  # First "y" answers the initial "Install Selfishell configuration?"
  # confirmation (read from FD 0, before the resource loop remaps it);
  # the second "y" answers the managed-file conflict prompt (read from FD 3,
  # a copy of the original stdin taken before that remap).
  printf 'y\ny\n' | SELFISHELL_TEST_TTY=1 run_selfishell install --profile minimal --skip-packages >/dev/null

  cmp -s "$ROOT_DIR/common/vimrc" "$target_file" ||
    fail "Modified managed file was not overwritten with the default"

  local conflict_backup
  conflict_backup="$(find "$XDG_STATE_HOME/selfishell/backups" -name 'vimrc.backup.*' 2>/dev/null | head -1)"
  [[ -n "$conflict_backup" ]] || fail "No conflict backup was created for the overwritten file"
  assert_file_content 'user_modified_data' "$conflict_backup"
}

test_managed_file_interactive_skip_preserves_state_and_continues() {
  run_selfishell install --profile minimal --skip-packages --yes >/dev/null

  local completion_target="$XDG_CONFIG_HOME/selfishell/zsh/completion.zsh"
  local completion_state="$XDG_STATE_HOME/selfishell/resources/zsh-completion.state"
  local saved_state="$TEST_ROOT/zsh-completion.state.before"

  printf 'user_modified_completion\n' >"$completion_target"
  cp "$completion_state" "$saved_state"

  local rc=0
  printf 'y\nn\n' | SELFISHELL_TEST_TTY=1 run_selfishell install --profile minimal --skip-packages >/dev/null || rc=$?

  ((rc == 0)) || fail "Install failed after skipping a modified managed file (exit code $rc)"
  assert_file_content 'user_modified_completion' "$completion_target"
  cmp -s "$saved_state" "$completion_state" || fail "Skipping a conflict must not change its resource state"
  cmp -s "$ROOT_DIR/common/vimrc" "$XDG_CONFIG_HOME/selfishell/vim/vimrc" ||
    fail "Later managed resources were not installed after a skip"
  [[ ! -d "$XDG_STATE_HOME/selfishell/backups" ]] ||
    fail "Skipping a modified file must not create a conflict backup"
}

test_managed_file_yes_flag_preserves_modification() {
  run_selfishell install --profile minimal --skip-packages --yes >/dev/null

  local target_file="$XDG_CONFIG_HOME/selfishell/vim/vimrc"
  local state_file="$XDG_STATE_HOME/selfishell/resources/vimrc.state"
  local saved_state="$TEST_ROOT/vimrc.state.before"

  printf 'user_modified_data\n' >"$target_file"
  cp "$state_file" "$saved_state"

  local rc=0
  run_selfishell install --profile minimal --skip-packages --yes >/dev/null 2>"$TEST_ROOT/stderr" || rc=$?

  ((rc != 0)) || fail "--yes must not silently overwrite a modified managed file"
  assert_file_content 'user_modified_data' "$target_file"
  cmp -s "$saved_state" "$state_file" || fail "A refused --yes conflict must not change the resource state"
  grep -Fq 'Managed file was modified; preserving it' "$TEST_ROOT/stderr" ||
    fail "--yes conflict did not report a preserving error"
  [[ ! -d "$XDG_STATE_HOME/selfishell/backups" ]] ||
    fail "--yes conflict must not create a conflict backup"
}

test_managed_link_conflict_still_aborts() {
  run_selfishell install --profile minimal --skip-packages --yes >/dev/null

  local link_path="$XDG_CONFIG_HOME/starship.toml"
  local state_file="$XDG_STATE_HOME/selfishell/resources/user-starship.state"
  local saved_state="$TEST_ROOT/user-starship.state.before"

  assert_symlink_to "$XDG_CONFIG_HOME/selfishell/starship.toml" "$link_path"
  cp "$state_file" "$saved_state"
  rm "$link_path"
  printf 'replaced_by_user\n' >"$link_path"

  local rc=0
  run_selfishell install --profile minimal --skip-packages --yes >/dev/null 2>"$TEST_ROOT/stderr" || rc=$?

  ((rc != 0)) || fail "A replaced managed link must still abort installation"
  assert_file_content 'replaced_by_user' "$link_path"
  cmp -s "$saved_state" "$state_file" || fail "A replaced managed link must not change its resource state"
  grep -Fq 'Managed link was replaced; preserving it' "$TEST_ROOT/stderr" ||
    fail "Replaced link did not report a preserving error"
}

test_managed_file_dry_run_conflict_changes_nothing() {
  run_selfishell install --profile minimal --skip-packages --yes >/dev/null

  local target_file="$XDG_CONFIG_HOME/selfishell/vim/vimrc"
  local state_file="$XDG_STATE_HOME/selfishell/resources/vimrc.state"
  local saved_target="$TEST_ROOT/vimrc.before"
  local saved_state="$TEST_ROOT/vimrc.state.before"

  printf 'user_modified_data\n' >"$target_file"
  cp "$target_file" "$saved_target"
  cp "$state_file" "$saved_state"
  [[ ! -d "$XDG_STATE_HOME/selfishell/backups" ]] || fail "Backups directory already existed before the dry run"

  local output
  output="$(run_selfishell update --tools-only --dry-run)"

  cmp -s "$saved_target" "$target_file" || fail "Dry run changed the conflicting managed file"
  cmp -s "$saved_state" "$state_file" || fail "Dry run changed the resource state"
  [[ ! -d "$XDG_STATE_HOME/selfishell/backups" ]] || fail "Dry run created a backups directory"
  [[ "$output" == *"Conflict: modified managed file: $target_file"* ]] ||
    fail "Dry run did not report the managed file conflict"
  [[ "$output" == *'Would require an overwrite or skip decision.'* ]] ||
    fail "Dry run did not describe the pending decision"
}

test_original_backup_survives_overwrite_and_uninstall_restore() {
  local target_file="$XDG_CONFIG_HOME/selfishell/vim/vimrc"

  mkdir -p "$(dirname "$target_file")"
  printf 'original-before-install\n' >"$target_file"

  run_selfishell install --profile minimal --skip-packages --yes >/dev/null

  local state_file="$XDG_STATE_HOME/selfishell/resources/vimrc.state"
  local original_backup
  original_backup="$(sed -n '6p' "$state_file")"
  [[ "$original_backup" != "-" ]] || fail "Installation backup was not recorded for a pre-existing file"
  assert_file_content 'original-before-install' "$original_backup"

  printf 'user-modification-after-install\n' >"$target_file"
  printf 'y\ny\n' | SELFISHELL_TEST_TTY=1 run_selfishell install --profile minimal --skip-packages >/dev/null

  [[ "$(sed -n '6p' "$state_file")" == "$original_backup" ]] ||
    fail "Overwriting a conflict must keep the original installation backup"
  cmp -s "$ROOT_DIR/common/vimrc" "$target_file" || fail "Overwrite did not install the default managed file"

  local conflict_backup
  conflict_backup="$(find "$XDG_STATE_HOME/selfishell/backups" -name 'vimrc.backup.*' 2>/dev/null | head -1)"
  [[ -n "$conflict_backup" ]] || fail "No conflict backup was created for the overwrite"
  assert_file_content 'user-modification-after-install' "$conflict_backup"

  run_selfishell uninstall --restore --yes >/dev/null
  assert_file_content 'original-before-install' "$target_file"
}

test_update_tools_only_overwrites_modified_managed_file() {
  local release_root="$TEST_ROOT/release"

  build_release_copy "$release_root"
  setup_fake_minimal_packages

  bash "$release_root/bin/selfishell" install --profile minimal --skip-packages --yes >/dev/null

  local target_file="$XDG_CONFIG_HOME/selfishell/vim/vimrc"
  printf 'user_modified_vimrc\n' >"$target_file"
  printf '" a newer default vimrc\n' >>"$release_root/common/vimrc"

  printf 'y\ny\n' | SELFISHELL_TEST_TTY=1 bash "$release_root/bin/selfishell" update --tools-only >/dev/null

  cmp -s "$release_root/common/vimrc" "$target_file" ||
    fail "update --tools-only did not install the new managed file version"

  local conflict_backup
  conflict_backup="$(find "$XDG_STATE_HOME/selfishell/backups" -name 'vimrc.backup.*' 2>/dev/null | head -1)"
  [[ -n "$conflict_backup" ]] || fail "update --tools-only did not create a conflict backup"
  assert_file_content 'user_modified_vimrc' "$conflict_backup"

  local expected_checksum recorded_checksum
  expected_checksum="$(cksum <"$release_root/common/vimrc" | awk '{print $1 ":" $2}')"
  recorded_checksum="$(sed -n '7p' "$XDG_STATE_HOME/selfishell/resources/vimrc.state")"
  [[ "$recorded_checksum" == "$expected_checksum" ]] ||
    fail "Resource state checksum was not updated to the new source checksum"
}

test_update_tools_only_skips_modified_managed_file_and_continues() {
  local release_root="$TEST_ROOT/release"

  build_release_copy "$release_root"
  setup_fake_minimal_packages

  bash "$release_root/bin/selfishell" install --profile minimal --skip-packages --yes >/dev/null

  local completion_target="$XDG_CONFIG_HOME/selfishell/zsh/completion.zsh"
  local completion_state="$XDG_STATE_HOME/selfishell/resources/zsh-completion.state"
  local saved_state="$TEST_ROOT/zsh-completion.state.before"

  printf 'user_modified_completion\n' >"$completion_target"
  cp "$completion_state" "$saved_state"
  printf '" a newer default vimrc\n' >>"$release_root/common/vimrc"

  local rc=0
  printf 'y\nn\n' | SELFISHELL_TEST_TTY=1 bash "$release_root/bin/selfishell" update --tools-only >/dev/null || rc=$?

  ((rc == 0)) || fail "update --tools-only failed after skipping a modified managed file (exit code $rc)"
  assert_file_content 'user_modified_completion' "$completion_target"
  cmp -s "$saved_state" "$completion_state" || fail "Skipping a conflict must not change its resource state"
  cmp -s "$release_root/common/vimrc" "$XDG_CONFIG_HOME/selfishell/vim/vimrc" ||
    fail "update --tools-only did not continue updating later managed resources after a skip"
}

test_update_tools_only_yes_preserves_modified_file() {
  setup_fake_minimal_packages
  run_selfishell install --profile minimal --skip-packages --yes >/dev/null

  local target_file="$XDG_CONFIG_HOME/selfishell/vim/vimrc"
  local state_file="$XDG_STATE_HOME/selfishell/resources/vimrc.state"
  local saved_state="$TEST_ROOT/vimrc.state.before"

  printf 'user_modified_data\n' >"$target_file"
  cp "$state_file" "$saved_state"

  local rc=0
  run_selfishell update --tools-only --yes </dev/null >/dev/null 2>"$TEST_ROOT/stderr" || rc=$?

  ((rc != 0)) || fail "update --tools-only --yes must not silently overwrite a modified managed file"
  assert_file_content 'user_modified_data' "$target_file"
  cmp -s "$saved_state" "$state_file" || fail "Refused conflict must not change the resource state"
  grep -Fq 'Managed file was modified; preserving it' "$TEST_ROOT/stderr" ||
    fail "Non-interactive update conflict did not report a preserving error"
  [[ ! -d "$XDG_STATE_HOME/selfishell/backups" ]] ||
    fail "Non-interactive update conflict must not create a conflict backup"
}

run_test() {
  local test_name="$1"
  local rc=0

  setup_managed_home
  "$test_name" || rc=$?
  teardown_managed_home

  if ((rc == 0)); then
    printf 'PASS: %s\n' "$test_name"
    return 0
  else
    printf 'FAIL: %s (exit code %d)\n' "$test_name" "$rc" >&2
    return 1
  fi
}

main() {
  local test_name
  local failures=0
  local test_list=()

  while IFS= read -r test_name; do
    if [[ -n "$test_name" ]]; then
      test_list+=("$test_name")
    fi
  done < <(declare -F | awk '{print $3}' | grep '^test_' | sort)

  printf 'Total tests found: %d\n' "${#test_list[@]}"

  for test_name in "${test_list[@]}"; do
    if ! run_test "$test_name"; then
      failures=$((failures + 1))
    fi
  done

  if ((failures > 0)); then
    printf '%d test(s) failed\n' "$failures" >&2
    return 1
  fi
}

main "$@"
