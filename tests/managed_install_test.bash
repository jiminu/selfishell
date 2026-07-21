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

  mkdir -p "$TEST_ROOT/bin"
  printf '#!/usr/bin/env bash\nexit 0\n' >"$TEST_ROOT/bin/chsh"
  chmod +x "$TEST_ROOT/bin/chsh"
  export PATH="$TEST_ROOT/bin:$PATH"
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
  [[ ! -L "$XDG_CONFIG_HOME/ghostty/config" ]] ||
    fail "A saved declined Ghostty choice created a dangling user link"
  assert_file_content '0' "$XDG_STATE_HOME/selfishell/ghostty"
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

  bash -c 'source "$1/lib/managed.sh"; managed_zsh_loader_block' _ "$ROOT_DIR" >"$loader"
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
  checksum="$(bash -c 'source "$1/lib/managed.sh"; managed_zsh_loader_block' _ "$ROOT_DIR" | cksum | awk '{print $1 ":" $2}')"
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

test_mise_config_global_install_developer_and_minimal() {
  run_selfishell install --profile developer --skip-packages --yes >/dev/null
  [[ -f "$XDG_CONFIG_HOME/mise/config.toml" ]] || fail "mise/config.toml was not created for developer profile"
  [[ -f "$XDG_STATE_HOME/selfishell/resources/mise-config-global.state" ]] || fail "mise-config-global.state was not recorded"

  local template_checksum
  template_checksum="$(cksum <"$ROOT_DIR/common/mise-global-config.toml" | awk '{print $1 ":" $2}')"
  local state_checksum
  state_checksum="$(sed -n '7p' "$XDG_STATE_HOME/selfishell/resources/mise-config-global.state")"
  [[ "$state_checksum" == "$template_checksum" ]] || fail "Registered state checksum does not match template"

  run_selfishell uninstall --restore --yes >/dev/null
  [[ ! -e "$XDG_CONFIG_HOME/mise/config.toml" ]] || fail "uninstall did not clean up empty config.toml"

  run_selfishell install --profile minimal --skip-packages --yes >/dev/null
  [[ ! -e "$XDG_CONFIG_HOME/mise/config.toml" ]] || fail "config.toml should not be created for minimal profile"
  [[ ! -e "$XDG_STATE_HOME/selfishell/resources/mise-config-global.state" ]] || fail "mise-config-global state should not exist for minimal profile"
}

test_mise_config_global_preserves_existing_file() {
  mkdir -p "$XDG_CONFIG_HOME/mise"
  printf 'existing user config\n' >"$XDG_CONFIG_HOME/mise/config.toml"

  run_selfishell install --profile developer --skip-packages --yes >/dev/null
  assert_file_content 'existing user config' "$XDG_CONFIG_HOME/mise/config.toml"

  local expected_checksum
  expected_checksum="$(cksum <"$XDG_CONFIG_HOME/mise/config.toml" | awk '{print $1 ":" $2}')"
  local state_checksum
  state_checksum="$(sed -n '7p' "$XDG_STATE_HOME/selfishell/resources/mise-config-global.state")"
  [[ "$state_checksum" == "$expected_checksum" ]] || fail "Existing file checksum was not recorded in state"
}

test_mise_config_global_update_allows_modification() {
  run_selfishell install --profile developer --skip-packages --yes >/dev/null
  printf 'modified by user\n' >"$XDG_CONFIG_HOME/mise/config.toml"

  run_selfishell install --profile developer --skip-packages --yes >/dev/null || fail "Reinstall failed after user modification"
  assert_file_content 'modified by user' "$XDG_CONFIG_HOME/mise/config.toml"
}

test_mise_config_global_uninstall_preservation() {
  run_selfishell install --profile developer --skip-packages --yes >/dev/null
  run_selfishell uninstall --restore --yes >/dev/null
  [[ ! -e "$XDG_CONFIG_HOME/mise/config.toml" ]] || fail "config.toml should be removed on uninstall if unchanged"

  run_selfishell install --profile developer --skip-packages --yes >/dev/null
  printf 'modified content\n' >"$XDG_CONFIG_HOME/mise/config.toml"
  run_selfishell uninstall --restore --yes >/dev/null
  [[ -f "$XDG_CONFIG_HOME/mise/config.toml" ]] || fail "modified config.toml should be preserved on uninstall"
  assert_file_content 'modified content' "$XDG_CONFIG_HOME/mise/config.toml"
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
