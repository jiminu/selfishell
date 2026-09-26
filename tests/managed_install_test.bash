#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

source "$ROOT_DIR/tests/test_helper.bash"
source "$ROOT_DIR/tests/cli_runner.bash"

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

  # Conflict tests below assert against $SELFISHELL_STATE_DIR directly. These
  # globals derive purely from the XDG_*/HOME values above, so scoping the
  # sourcing here keeps every other test's isolation intact.
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

test_malformed_managed_file_state_variants_are_rejected_without_changes() {
  run_selfishell install --skip-packages --yes >/dev/null

  local target="$XDG_CONFIG_HOME/selfishell/vim/vimrc"
  local state_file="$XDG_STATE_HOME/selfishell/resources/vimrc.state"
  local before_content
  local label
  local status

  before_content="$(<"$target")"

  for label in truncated-1-line truncated-6-lines empty-type unknown-type \
    unknown-status empty-target unsupported-version no-final-newline symlinked-state; do
    case "$label" in
      truncated-1-line) printf '2\n' >"$state_file" ;;
      truncated-6-lines) printf '2\nfile\nactive\n%s\nref\nbackup\n' "$target" >"$state_file" ;;
      empty-type) printf '2\n\nactive\n%s\nref\nbackup\nchecksum\n' "$target" >"$state_file" ;;
      unknown-type) printf '2\nbogus\nactive\n%s\nref\nbackup\nchecksum\n' "$target" >"$state_file" ;;
      unknown-status) printf '2\nfile\nbogus\n%s\nref\nbackup\nchecksum\n' "$target" >"$state_file" ;;
      empty-target) printf '2\nfile\nactive\n\nref\nbackup\nchecksum\n' >"$state_file" ;;
      unsupported-version) printf '3\nfile\nactive\n%s\nref\nbackup\nchecksum\n' "$target" >"$state_file" ;;
      no-final-newline) printf '2\nfile\nactive\n%s\nref\nbackup\nchecksum' "$target" >"$state_file" ;;
      # A well-formed state file is still rejected when the state *path*
      # itself is a symlink, so a redirected state read/write can't be used
      # to smuggle another resource's (or an attacker's) state in its place.
      symlinked-state)
        printf '2\nfile\nactive\n%s\nref\nbackup\nchecksum\n' "$target" >"$TEST_ROOT/symlinked-state-target"
        rm -f "$state_file"
        ln -s "$TEST_ROOT/symlinked-state-target" "$state_file"
        ;;
    esac

    set +e
    run_selfishell install --skip-packages --yes >/dev/null 2>"$TEST_ROOT/stderr"
    status=$?
    set -e

    [[ "$status" -ne 0 ]] || fail "Malformed state ($label) should stop install"
    [[ "$(<"$target")" == "$before_content" ]] ||
      fail "Malformed state ($label) changed the existing target bytes"
    [[ ! -d "$XDG_STATE_HOME/selfishell/backups" ]] ||
      fail "Malformed state ($label) created a backup"
    grep -Fq 'vimrc.state' "$TEST_ROOT/stderr" ||
      fail "Malformed state ($label) error did not mention the state path"
  done
}

test_malformed_state_blocks_uninstall_without_removing_managed_block() {
  run_selfishell install --skip-packages --yes >/dev/null

  local state_file="$XDG_STATE_HOME/selfishell/resources/user-zshrc.state"
  local target="$HOME/.zshrc"
  local before_content
  local status

  before_content="$(<"$target")"
  printf '2\n' >"$state_file"

  set +e
  run_selfishell uninstall --yes >/dev/null 2>"$TEST_ROOT/stderr"
  status=$?
  set -e

  [[ "$status" -ne 0 ]] || fail "Malformed state should stop uninstall"
  [[ "$(<"$target")" == "$before_content" ]] ||
    fail "Malformed state changed the target during a blocked uninstall"
  grep -Fqx '# >>> Selfishell initialize >>>' "$target" ||
    fail "Malformed state let uninstall silently skip removing the managed block"
  grep -Fq 'user-zshrc.state' "$TEST_ROOT/stderr" ||
    fail "Malformed state uninstall error did not mention the state path"
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

test_managed_file_replaced_by_same_content_symlink_is_preserved() {
  local target="$XDG_CONFIG_HOME/selfishell/vim/vimrc"
  local personal="$TEST_ROOT/personal-vimrc"
  local state="$SELFISHELL_RESOURCE_STATE_DIR/vimrc.state"
  local operation rc

  source "$ROOT_DIR/lib/common.sh"
  source "$ROOT_DIR/lib/managed.sh"
  source "$ROOT_DIR/lib/commands/status.sh"
  managed_install_file vimrc "$ROOT_DIR/config/shared/vimrc" "$target" 0 1 >/dev/null
  cp "$state" "$TEST_ROOT/original.state"
  mv "$target" "$personal"
  ln -s "$personal" "$target"

  SELFISHELL_STATUS_RESOURCE_COUNT=0
  SELFISHELL_STATUS_RESULT=0
  status_resource vimrc >"$TEST_ROOT/status"
  ((SELFISHELL_STATUS_RESULT != 0)) || fail "Status accepted a replaced managed file symlink"

  for operation in install preflight uninstall; do
    rc=0
    case "$operation" in
      install) managed_install_file vimrc "$ROOT_DIR/config/shared/vimrc" "$target" 0 1 >/dev/null 2>&1 || rc=$? ;;
      preflight) managed_validate_uninstall_resource vimrc >/dev/null 2>&1 || rc=$? ;;
      uninstall) managed_uninstall_resource vimrc 1 0 >/dev/null 2>&1 || rc=$? ;;
    esac
    ((rc != 0)) || fail "$operation accepted a replaced managed file symlink"
    assert_symlink_to "$personal" "$target"
    cmp -s "$personal" "$ROOT_DIR/config/shared/vimrc" || fail "$operation changed the personal file"
    cmp -s "$state" "$TEST_ROOT/original.state" || fail "$operation changed resource state"
  done
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
  local state_count output

  # A backed-up original makes the dry run preview a restore onto the link it would remove.
  mkdir -p "$XDG_CONFIG_HOME"
  printf 'user starship\n' >"$XDG_CONFIG_HOME/starship.toml"
  run_selfishell install --skip-packages --yes >/dev/null
  state_count="$(find "$XDG_STATE_HOME/selfishell/resources" -type f -name '*.state' | wc -l)"
  output="$(run_selfishell uninstall --restore --dry-run)" ||
    fail "Uninstall dry run failed with a backup to restore: $output"

  [[ "$output" == *"Would restore: $XDG_CONFIG_HOME/starship.toml.backup."*" -> $XDG_CONFIG_HOME/starship.toml"* ]] ||
    fail "Uninstall dry run did not preview the restore: $output"
  assert_symlink_to "$XDG_CONFIG_HOME/selfishell/starship.toml" "$XDG_CONFIG_HOME/starship.toml"
  [[ -f "$HOME/.zshrc" && ! -L "$HOME/.zshrc" ]] || fail "Uninstall dry run changed .zshrc type"
  grep -Fqx '# >>> Selfishell initialize >>>' "$HOME/.zshrc" || fail "Uninstall dry run removed the loader"
  [[ "$(find "$XDG_STATE_HOME/selfishell/resources" -type f -name '*.state' | wc -l)" -eq "$state_count" ]] ||
    fail "Uninstall dry run changed state"
}

test_uninstall_explains_an_interrupted_install() {
  local state_file="$XDG_STATE_HOME/selfishell/resources/user-vimrc.state"
  local rc=0

  run_selfishell install --skip-packages --yes >/dev/null
  # An install interrupted after recording pending state, before adding the block.
  sed -i.bak '3s/.*/pending/' "$state_file" && rm -f "$state_file.bak"
  printf 'user vimrc\n' >"$HOME/.vimrc"

  run_selfishell uninstall --yes >/dev/null 2>"$TEST_ROOT/stderr" || rc=$?
  ((rc != 0)) || fail "Uninstall removed resources around an unfinished install"
  grep -Fq "An interrupted install left this unfinished; run 'selfishell install', then uninstall again." \
    "$TEST_ROOT/stderr" || fail "Uninstall did not explain the interrupted install: $(<"$TEST_ROOT/stderr")"
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
  assert_file_content '1' "$XDG_STATE_HOME/selfishell/configured"
}

test_uninstall_stops_when_the_resource_list_fails() {
  local fake_bin="$TEST_ROOT/bin"
  local status

  run_selfishell install --skip-packages --yes >/dev/null
  mkdir -p "$fake_bin"
  # The list's producer emits one name, then fails.
  cat >"$fake_bin/cut" <<'EOF'
#!/usr/bin/env bash
/usr/bin/cut "$@" | head -n 1
exit 42
EOF
  chmod +x "$fake_bin/cut"

  set +e
  PATH="$fake_bin:/usr/bin:/bin" run_selfishell uninstall --yes >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -ne 0 ]] || fail "Uninstall succeeded on a partial resource list"
  [[ -r "$XDG_CONFIG_HOME/selfishell/zsh/zshrc" ]] ||
    fail "Uninstall removed a resource from a partial list"
  grep -Fqx '# >>> Selfishell initialize >>>' "$HOME/.zshrc" ||
    fail "Uninstall removed the loader from a partial list"
  assert_file_content '1' "$XDG_STATE_HOME/selfishell/configured"
}

test_uninstall_removes_ghostty_block_before_ghostty_defaults() {
  export SELFISHELL_TEST_SYSTEM_NAME=Darwin
  local fake_bin="$TEST_ROOT/bin"
  local block_target="$XDG_CONFIG_HOME/ghostty/config.ghostty"
  local defaults_target="$XDG_CONFIG_HOME/selfishell/ghostty/config.ghostty"
  local block_state="$XDG_STATE_HOME/selfishell/resources/user-ghostty.state"
  local defaults_state="$XDG_STATE_HOME/selfishell/resources/ghostty-config.state"
  local status

  run_selfishell install --skip-packages --yes >/dev/null
  [[ -f "$defaults_target" ]] || fail "Ghostty defaults were not installed"
  grep -Fqx '# >>> Selfishell ghostty >>>' "$block_target" || fail "Ghostty block was not installed"

  mkdir -p "$fake_bin"
  cat >"$fake_bin/rm" <<'EOF'
#!/usr/bin/env bash
for argument in "$@"; do
  case "$argument" in
    */selfishell/ghostty/config.ghostty) exit 1 ;;
  esac
done
exec /bin/rm "$@"
EOF
  chmod +x "$fake_bin/rm"

  set +e
  PATH="$fake_bin:/usr/bin:/bin" run_selfishell uninstall --yes >/dev/null 2>"$TEST_ROOT/stderr"
  status=$?
  set -e

  [[ "$status" -ne 0 ]] || fail "Uninstall should report the Ghostty defaults removal failure"
  [[ ! -e "$block_state" ]] || fail "The user-facing Ghostty block was not removed before the failure"
  ! grep -Fqx '# >>> Selfishell ghostty >>>' "$block_target" ||
    fail "The Ghostty block was not actually removed from the user file"
  [[ -e "$defaults_state" ]] || fail "The Ghostty defaults state should remain after a failed removal"
  [[ -f "$defaults_target" ]] || fail "The Ghostty defaults file should remain after a failed removal"
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
    printf '2\nfile\npending\n%s\n-\n%s\n%s\n' \
      "$target_file" \
      "$backup_file" \
      "$(cksum <"$ROOT_DIR/config/shared/zsh/common.zsh" | awk '{print $1 ":" $2}')"
  } >"$state_dir/zsh-common.state"

  run_selfishell install --skip-packages --yes >/dev/null

  assert_file_content 'preexisting managed path' "$backup_file"
  cmp -s "$ROOT_DIR/config/shared/zsh/common.zsh" "$target_file" ||
    fail "Pending managed file installation did not resume"
  [[ "$(sed -n '3p' "$state_dir/zsh-common.state")" == "active" ]] ||
    fail "Pending file state was not completed"
}

test_active_file_state_with_changed_source_reports_updated() {
  local target_file="$XDG_CONFIG_HOME/selfishell/zsh/common.zsh"
  local state_dir="$XDG_STATE_HOME/selfishell/resources"
  local stdout

  mkdir -p "$(dirname "$target_file")" "$state_dir"
  printf 'previously installed content' >"$target_file"
  {
    printf '2\nfile\nactive\n%s\n-\n-\n%s\n' \
      "$target_file" \
      "$(cksum <"$target_file" | awk '{print $1 ":" $2}')"
  } >"$state_dir/zsh-common.state"

  stdout="$(run_selfishell install --skip-packages --yes)"

  cmp -s "$ROOT_DIR/config/shared/zsh/common.zsh" "$target_file" ||
    fail "An active file state with a changed source was not resynced"
  grep -Fq "Updated managed file: $target_file" <<<"$stdout" ||
    fail "A resource whose source changed since an active install should be reported as updated"
  ! grep -Fq "Installed managed file: $target_file" <<<"$stdout" ||
    fail "A resource whose source changed since an active install must not be reported as a fresh install"
}

test_active_file_state_with_missing_target_reports_installed() {
  local target_file="$XDG_CONFIG_HOME/selfishell/zsh/common.zsh"
  local state_dir="$XDG_STATE_HOME/selfishell/resources"
  local stdout

  mkdir -p "$(dirname "$target_file")" "$state_dir"
  {
    printf '2\nfile\nactive\n%s\n-\n-\n%s\n' \
      "$target_file" \
      "$(cksum <"$ROOT_DIR/config/shared/zsh/common.zsh" | awk '{print $1 ":" $2}')"
  } >"$state_dir/zsh-common.state"
  rm -f "$target_file"

  stdout="$(run_selfishell install --skip-packages --yes)"

  cmp -s "$ROOT_DIR/config/shared/zsh/common.zsh" "$target_file" ||
    fail "An active file state with a missing target was not recreated"
  grep -Fq "Installed managed file: $target_file" <<<"$stdout" ||
    fail "An active state whose target is missing should be reported as a fresh install"
  ! grep -Fq "Updated managed file: $target_file" <<<"$stdout" ||
    fail "An active state whose target is missing must not be reported as an update"
}

test_install_does_not_depend_on_checkout() {
  local release_root="$TEST_ROOT/release"

  mkdir -p "$release_root"
  cp -R "$ROOT_DIR/bin" "$ROOT_DIR/lib" "$ROOT_DIR/packages.conf" "$ROOT_DIR/config" "$release_root/"
  printf '0.0.0-test.1\n' >"$release_root/VERSION"
  cp "$ROOT_DIR/dependencies.conf" "$release_root/"

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

test_mise_config_global_preserves_existing_types() {
  source "$ROOT_DIR/lib/common.sh"
  source "$ROOT_DIR/lib/commands/install.sh"

  # Exercise the create-once boundary directly; the configuration
  # installation tests above cover command wiring.
  mkdir -p "$XDG_CONFIG_HOME/mise"
  printf 'user_owned_data_content_bytes\n' >"$XDG_CONFIG_HOME/mise/config.toml"
  install_mise_global_config 0 >/dev/null
  assert_file_content 'user_owned_data_content_bytes' "$XDG_CONFIG_HOME/mise/config.toml"

  rm -f "$XDG_CONFIG_HOME/mise/config.toml"
  printf 'link_target_content\n' >"$TEST_ROOT/real_config.toml"
  ln -s "$TEST_ROOT/real_config.toml" "$XDG_CONFIG_HOME/mise/config.toml"
  install_mise_global_config 0 >/dev/null
  assert_symlink_to "$TEST_ROOT/real_config.toml" "$XDG_CONFIG_HOME/mise/config.toml"
  assert_file_content 'link_target_content' "$TEST_ROOT/real_config.toml"

  rm -f "$XDG_CONFIG_HOME/mise/config.toml"
  mkdir -p "$TEST_ROOT/some_dir"
  ln -s "$TEST_ROOT/some_dir" "$XDG_CONFIG_HOME/mise/config.toml"
  install_mise_global_config 0 >/dev/null
  assert_symlink_to "$TEST_ROOT/some_dir" "$XDG_CONFIG_HOME/mise/config.toml"

  # Removing the referent leaves a dangling link, which is still user data.
  rm -rf "$TEST_ROOT/some_dir"
  install_mise_global_config 0 >/dev/null
  assert_symlink_to "$TEST_ROOT/some_dir" "$XDG_CONFIG_HOME/mise/config.toml"
  [[ ! -e "$SELFISHELL_RESOURCE_STATE_DIR" ]] || fail "Create-once file acquired managed state"
}

test_mise_config_global_uninstall_preservation() {
  run_selfishell install --skip-packages --yes >/dev/null
  run_selfishell uninstall --restore --yes >/dev/null
  [[ -f "$XDG_CONFIG_HOME/mise/config.toml" ]] || fail "config.toml should remain after uninstall"
}

test_mise_config_global_dry_run_and_directory_error() {
  run_selfishell install --skip-packages --dry-run --yes >/dev/null
  [[ ! -e "$XDG_CONFIG_HOME/mise/config.toml" ]] || fail "dry-run created config.toml"

  mkdir -p "$XDG_CONFIG_HOME/mise/config.toml"
  local rc=0
  run_selfishell install --skip-packages --yes >/dev/null 2>&1 || rc=$?
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
      ' zsh "$ROOT_DIR/config/shared/zsh/runtime.zsh"
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
      ' zsh "$ROOT_DIR/config/shared/zsh/runtime.zsh"
  )" || fail "runtime created MISE_GLOBAL_CONFIG_FILE"
}

# A real `update` reaches packages_install: fake apt-get/dpkg pass the apt
# check without sudo or network, and pre-created direct targets read as present.
setup_fake_zinit() {
  local real_git

  real_git="$(command -v git)"
  mkdir -p "$HOME/.local/share/zinit/zinit.git"
  cat >"$HOME/.local/share/zinit/zinit.git/zinit.zsh" <<'EOF'
typeset -g selfishell_test_revision
zinit() {
  if [[ "$1" == ice ]]; then
    selfishell_test_revision="${3#ver}"
  elif [[ "$1" == light ]]; then
    plugin_dir="${XDG_DATA_HOME:-$HOME/.local/share}/zinit/plugins/${2//\//---}"
    command mkdir -p "$plugin_dir/.git"
    print -r -- "$selfishell_test_revision" >"$plugin_dir/.git/selfishell-approved-revision"
  fi
}
EOF
  cat >"$TEST_ROOT/bin/git" <<EOF
#!/usr/bin/env bash
if [[ "\${1:-}" == -C && -r "\$2/.git/selfishell-approved-revision" ]]; then
  if [[ "\${3:-}" == rev-parse && "\${4:-}" == HEAD ]]; then
    cat "\$2/.git/selfishell-approved-revision"
    exit 0
  elif [[ "\${3:-}" == status ]]; then
    exit 0
  fi
fi
exec "$real_git" "\$@"
EOF
  chmod +x "$TEST_ROOT/bin/git"
}

setup_fake_packages() {
  mkdir -p "$TEST_ROOT/bin"
  printf '#!/usr/bin/env bash\nexit 0\n' >"$TEST_ROOT/bin/apt-get"
  chmod +x "$TEST_ROOT/bin/apt-get"
  printf '#!/usr/bin/env bash\nexit 0\n' >"$TEST_ROOT/bin/dpkg"
  chmod +x "$TEST_ROOT/bin/dpkg"
  printf '#!/usr/bin/env bash\nprintf "install ok installed\\n"\n' >"$TEST_ROOT/bin/dpkg-query"
  chmod +x "$TEST_ROOT/bin/dpkg-query"

  mkdir -p "$HOME/.local/bin"
  printf '#!/usr/bin/env bash\nexit 0\n' >"$HOME/.local/bin/starship"
  chmod +x "$HOME/.local/bin/starship"
  setup_fake_zinit
  setup_fake_editor_tools
}

setup_fake_editor_tools() {
  local type repository revision source plugin_dir tool
  mkdir -p "$HOME/.local/bin" "$TEST_ROOT/bin"
  for tool in mise nvim; do
    printf '#!/usr/bin/env bash\nexit 0\n' >"$HOME/.local/bin/$tool"
    chmod +x "$HOME/.local/bin/$tool"
    ln -s "$HOME/.local/bin/$tool" "$TEST_ROOT/bin/$tool"
  done
  # These lifecycle tests exercise managed files; model already-synced editors
  # without downloading tools or plugin repositories.
  while read -r type repository revision _ _ source _; do
    [[ "$type" == nvim-plugin ]] || continue
    if [[ "$repository" == folke/lazy.nvim ]]; then
      plugin_dir="$HOME/.local/share/selfishell/nvim/lazy/lazy.nvim"
    else
      source="${source##*/}"
      plugin_dir="$HOME/.local/share/nvim/lazy/${source%.git}"
    fi
    mkdir -p "$plugin_dir/.git"
    printf '%s\n' "$revision" >"$plugin_dir/.git/selfishell-approved-revision"
  done <"$ROOT_DIR/dependencies.conf"
}

setup_fake_macos_packages() {
  mkdir -p "$TEST_ROOT/bin"
  cat >"$TEST_ROOT/bin/brew" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == list && "${2:-}" == --formula ]]; then
  printf '%s\n' git starship vim
elif [[ "${1:-}" == list && "${2:-}" == --cask ]]; then
  printf '%s\n' font-meslo-lg-nerd-font font-noto-sans-cjk-kr
fi
EOF
  chmod +x "$TEST_ROOT/bin/brew"

  setup_fake_zinit
  setup_fake_editor_tools
}

# Use a private checkout to simulate release changes without modifying the test source.
build_release_copy() {
  local release_root="$1"

  mkdir -p "$release_root"
  cp -R "$ROOT_DIR/bin" "$ROOT_DIR/lib" "$ROOT_DIR/packages.conf" "$ROOT_DIR/config" "$release_root/"
  printf '0.0.0-test.1\n' >"$release_root/VERSION"
  cp "$ROOT_DIR/dependencies.conf" "$release_root/dependencies.conf"
}

test_reinstall_preserves_tool_caches_until_generator_configuration_changes() {
  local release_root="$TEST_ROOT/release"
  local cache_dir="$XDG_CACHE_HOME/selfishell"
  local scenario tool

  build_release_copy "$release_root"
  bash "$release_root/bin/selfishell" install --skip-packages --yes >/dev/null
  mkdir -p "$cache_dir"
  for tool in zoxide fzf starship; do
    printf '# cached %s init\n' "$tool" >"$cache_dir/$tool-init.zsh"
  done

  for scenario in unchanged unrelated dry-run changed; do
    case "$scenario" in
      unrelated) printf '\nset noshowmode\n' >>"$release_root/config/shared/vimrc" ;;
      dry-run) printf '\n# updated generator\n' >>"$release_root/config/shared/zsh/interactive.zsh" ;;
    esac
    if [[ "$scenario" == dry-run ]]; then
      bash "$release_root/bin/selfishell" install --skip-packages --yes --dry-run >/dev/null
    else
      bash "$release_root/bin/selfishell" install --skip-packages --yes >/dev/null
    fi
    for tool in zoxide fzf starship; do
      if [[ "$scenario" == changed ]]; then
        [[ ! -e "$cache_dir/$tool-init.zsh" ]] || fail "$tool cache survived a generator change"
      else
        [[ -f "$cache_dir/$tool-init.zsh" ]] || fail "$scenario install removed $tool cache"
        [[ "$(<"$cache_dir/$tool-init.zsh")" == "# cached $tool init" ]] || fail "$scenario install rewrote $tool cache"
      fi
    done
  done
}

test_managed_file_interactive_overwrite_yes() {
  run_selfishell install --skip-packages --yes >/dev/null

  local target_file="$XDG_CONFIG_HOME/selfishell/vim/vimrc"
  printf 'user_modified_data\n' >"$target_file"

  # First "y" answers the install confirmation, read from FD 0 before the
  # resource loop remaps it; the second answers the conflict prompt, read from
  # FD 3, the copy of stdin taken before that remap.
  local stdout
  stdout="$(printf 'y\ny\n' | SELFISHELL_TEST_TTY=1 run_selfishell install --skip-packages)"

  cmp -s "$ROOT_DIR/config/shared/vimrc" "$target_file" ||
    fail "Modified managed file was not overwritten with the default"

  local conflict_backup
  conflict_backup="$(find "$XDG_STATE_HOME/selfishell/backups" -name 'vimrc.backup.*' 2>/dev/null | head -1)"
  [[ -n "$conflict_backup" ]] || fail "No conflict backup was created for the overwritten file"
  assert_file_content 'user_modified_data' "$conflict_backup"

  grep -Fq "Updated managed file: $target_file" <<<"$stdout" ||
    fail "Overwriting a previously active managed file should report it as updated, not installed"
  ! grep -Fq "Installed managed file: $target_file" <<<"$stdout" ||
    fail "Overwriting a previously active managed file must not be reported as a fresh install"
}

test_managed_file_interactive_skip_preserves_state_and_continues() {
  run_selfishell install --skip-packages --yes >/dev/null

  local completion_target="$XDG_CONFIG_HOME/selfishell/zsh/completion.zsh"
  local completion_state="$XDG_STATE_HOME/selfishell/resources/zsh-completion.state"
  local saved_state="$TEST_ROOT/zsh-completion.state.before"

  printf 'user_modified_completion\n' >"$completion_target"
  cp "$completion_state" "$saved_state"

  local rc=0
  printf 'y\nn\n' | SELFISHELL_TEST_TTY=1 run_selfishell install --skip-packages >/dev/null || rc=$?

  ((rc == 0)) || fail "Install failed after skipping a modified managed file (exit code $rc)"
  assert_file_content 'user_modified_completion' "$completion_target"
  cmp -s "$saved_state" "$completion_state" || fail "Skipping a conflict must not change its resource state"
  cmp -s "$ROOT_DIR/config/shared/vimrc" "$XDG_CONFIG_HOME/selfishell/vim/vimrc" ||
    fail "Later managed resources were not installed after a skip"
  [[ ! -d "$XDG_STATE_HOME/selfishell/backups" ]] ||
    fail "Skipping a modified file must not create a conflict backup"
}

test_managed_file_yes_flag_preserves_modification() {
  run_selfishell install --skip-packages --yes >/dev/null

  local target_file="$XDG_CONFIG_HOME/selfishell/vim/vimrc"
  local state_file="$XDG_STATE_HOME/selfishell/resources/vimrc.state"
  local saved_state="$TEST_ROOT/vimrc.state.before"

  printf 'user_modified_data\n' >"$target_file"
  cp "$state_file" "$saved_state"

  local rc=0
  run_selfishell install --skip-packages --yes >/dev/null 2>"$TEST_ROOT/stderr" || rc=$?

  ((rc != 0)) || fail "--yes must not silently overwrite a modified managed file"
  assert_file_content 'user_modified_data' "$target_file"
  cmp -s "$saved_state" "$state_file" || fail "A refused --yes conflict must not change the resource state"
  grep -Fq 'Managed file was modified; preserving it' "$TEST_ROOT/stderr" ||
    fail "--yes conflict did not report a preserving error"
  [[ ! -d "$XDG_STATE_HOME/selfishell/backups" ]] ||
    fail "--yes conflict must not create a conflict backup"
}

test_managed_link_conflict_still_aborts() {
  run_selfishell install --skip-packages --yes >/dev/null

  local link_path="$XDG_CONFIG_HOME/starship.toml"
  local state_file="$XDG_STATE_HOME/selfishell/resources/user-starship.state"
  local saved_state="$TEST_ROOT/user-starship.state.before"

  assert_symlink_to "$XDG_CONFIG_HOME/selfishell/starship.toml" "$link_path"
  cp "$state_file" "$saved_state"
  rm "$link_path"
  printf 'replaced_by_user\n' >"$link_path"
  # Listed before the link: preflight must stop before recreating it.
  rm "$XDG_CONFIG_HOME/selfishell/zsh/runtime.zsh"

  local rc=0
  run_selfishell install --skip-packages --yes >/dev/null 2>"$TEST_ROOT/stderr" || rc=$?

  ((rc != 0)) || fail "A replaced managed link must still abort installation"
  assert_file_content 'replaced_by_user' "$link_path"
  [[ ! -e "$XDG_CONFIG_HOME/selfishell/zsh/runtime.zsh" ]] ||
    fail "A link conflict still applied an earlier managed file"
  cmp -s "$saved_state" "$state_file" || fail "A replaced managed link must not change its resource state"
  grep -Fq 'Managed link was replaced; preserving it' "$TEST_ROOT/stderr" ||
    fail "Replaced link did not report a preserving error"
}

test_managed_link_creation_failure_does_not_report_success_and_is_retryable() {
  run_selfishell install --skip-packages --yes >/dev/null

  local link_path="$XDG_CONFIG_HOME/starship.toml"
  local state_file="$XDG_STATE_HOME/selfishell/resources/user-starship.state"
  local rc=0

  assert_symlink_to "$XDG_CONFIG_HOME/selfishell/starship.toml" "$link_path"
  rm "$link_path"
  chmod 0555 "$XDG_CONFIG_HOME"

  run_selfishell install --skip-packages --yes >"$TEST_ROOT/stdout" 2>"$TEST_ROOT/stderr" || rc=$?
  chmod 0755 "$XDG_CONFIG_HOME"

  ((rc != 0)) || fail "A symlink creation failure must not be reported as success"
  [[ ! -e "$link_path" ]] || fail "A failed link creation must not leave a partial target"
  ! grep -Fq 'Linked:' "$TEST_ROOT/stdout" || fail "A failed link creation printed a success message"
  [[ "$(sed -n '3p' "$state_file")" == pending ]] ||
    fail "A failed link creation must not be recorded as active"

  run_selfishell install --skip-packages --yes >/dev/null
  assert_symlink_to "$XDG_CONFIG_HOME/selfishell/starship.toml" "$link_path"
  [[ "$(sed -n '3p' "$state_file")" == active ]] ||
    fail "Retrying after a fixed permission error did not recover"
}

test_atomic_copy_step_failures_preserve_target_and_clean_up() {
  local source="$TEST_ROOT/atomic-copy-source"
  local target="$HOME/atomic-copy-target"
  local helper="$TEST_ROOT/atomic-copy-helper.bash"
  local scenario
  local status

  printf 'new content\n' >"$source"

  cat >"$helper" <<'EOF'
#!/usr/bin/env bash
source "$1/lib/common.sh"
source "$1/lib/paths.sh"
selfishell_initialize_paths
source "$1/lib/managed.sh"
case "$2" in
  cp) cp() { return 1; } ;;
  chmod) chmod() { return 1; } ;;
  mv) mv() { return 1; } ;;
esac
managed_atomic_copy "$3" "$4"
EOF

  for scenario in cp chmod mv; do
    printf 'original content\n' >"$target"

    set +e
    bash "$helper" "$ROOT_DIR" "$scenario" "$source" "$target" >/dev/null 2>"$TEST_ROOT/stderr"
    status=$?
    set -e

    [[ "$status" -ne 0 ]] || fail "A forced $scenario failure in managed_atomic_copy should propagate"
    assert_file_content 'original content' "$target"
  done

  [[ "$(find "$HOME" -maxdepth 1 -name 'atomic-copy-target.tmp.*' | wc -l)" -eq 0 ]] ||
    fail "A forced managed_atomic_copy failure left a temporary file behind"
}

test_write_state_step_failures_preserve_existing_state() {
  local target="$HOME/.zshrc"
  local state_file="$XDG_STATE_HOME/selfishell/resources/user-zshrc.state"
  local helper="$TEST_ROOT/write-state-helper.bash"
  local before_checksum
  local scenario
  local status

  printf 'original zshrc\n' >"$target"
  run_selfishell install --skip-packages --yes >/dev/null
  before_checksum="$(sed -n '7p' "$state_file")"

  cat >"$helper" <<'EOF'
#!/usr/bin/env bash
source "$1/lib/common.sh"
source "$1/lib/paths.sh"
selfishell_initialize_paths
source "$1/lib/managed.sh"
case "$2" in
  mktemp) mktemp() { return 1; } ;;
  mv) mv() { return 1; } ;;
esac
managed_write_state user-zshrc block active "$3" - - forged-checksum
EOF

  for scenario in mktemp mv; do
    set +e
    bash "$helper" "$ROOT_DIR" "$scenario" "$target" >/dev/null 2>"$TEST_ROOT/stderr"
    status=$?
    set -e

    [[ "$status" -ne 0 ]] || fail "A forced $scenario failure in managed_write_state should propagate"
    [[ "$(sed -n '7p' "$state_file")" == "$before_checksum" ]] ||
      fail "A forced $scenario failure changed the existing state checksum"
  done

  [[ "$(find "$XDG_STATE_HOME/selfishell/resources" -maxdepth 1 -name 'user-zshrc.state.tmp.*' | wc -l)" -eq 0 ]] ||
    fail "A forced managed_write_state failure left a temporary file behind"
}

test_pending_block_state_refresh_failure_does_not_report_unchanged() {
  local fake_bin="$TEST_ROOT/bin"
  local status=0

  run_selfishell install --skip-packages --yes >/dev/null
  local state_file="$SELFISHELL_RESOURCE_STATE_DIR/user-zshrc.state"
  sed '3s/active/pending/' "$state_file" >"$TEST_ROOT/pending.state"
  mv "$TEST_ROOT/pending.state" "$state_file"

  mkdir -p "$fake_bin"
  cat >"$fake_bin/mv" <<'EOF'
#!/usr/bin/env bash
for argument in "$@"; do
  case "$argument" in
    */resources/user-zshrc.state.tmp.*) exit 1 ;;
  esac
done
exec /bin/mv "$@"
EOF
  chmod +x "$fake_bin/mv"

  set +e
  PATH="$fake_bin:/usr/bin:/bin" run_selfishell install --skip-packages --yes >"$TEST_ROOT/stdout" 2>"$TEST_ROOT/stderr"
  status=$?
  set -e

  ((status != 0)) || fail "A forced state-refresh failure for a pending block should propagate"
}

test_managed_file_overwrite_conflict_atomic_copy_failure_preserves_backup_and_state() {
  run_selfishell install --skip-packages --yes >/dev/null

  local target_file="$XDG_CONFIG_HOME/selfishell/vim/vimrc"
  local state_file="$XDG_STATE_HOME/selfishell/resources/vimrc.state"
  local saved_state="$TEST_ROOT/vimrc.state.before"
  # Keep the failing command off the persistent test PATH so retry uses the real one.
  local fake_bin="$TEST_ROOT/fakebin"
  local status=0

  printf 'user_modified_vimrc\n' >"$target_file"
  cp "$state_file" "$saved_state"

  mkdir -p "$fake_bin"
  cat >"$fake_bin/cp" <<'EOF'
#!/usr/bin/env bash
for argument in "$@"; do
  case "$argument" in
    */vim/vimrc.tmp.*) exit 1 ;;
  esac
done
exec /bin/cp "$@"
EOF
  chmod +x "$fake_bin/cp"

  set +e
  printf 'y\ny\n' | PATH="$fake_bin:/usr/bin:/bin" SELFISHELL_TEST_TTY=1 run_selfishell install --skip-packages >/dev/null 2>"$TEST_ROOT/stderr"
  status=$?
  set -e

  [[ "$status" -ne 0 ]] || fail "A forced atomic-copy failure during an overwrite conflict should propagate"
  assert_file_content 'user_modified_vimrc' "$target_file"
  cmp -s "$saved_state" "$state_file" || fail "A failed overwrite must not change the existing resource state"

  local conflict_backup
  conflict_backup="$(find "$XDG_STATE_HOME/selfishell/backups" -name 'vimrc.backup.*' 2>/dev/null | head -1)"
  [[ -n "$conflict_backup" ]] || fail "No conflict backup was created before the failed overwrite"
  assert_file_content 'user_modified_vimrc' "$conflict_backup"

  ! grep -Fq 'Installed managed file' "$TEST_ROOT/stderr" ||
    fail "A failed overwrite must not report success"

  printf 'y\ny\n' | SELFISHELL_TEST_TTY=1 run_selfishell install --skip-packages >/dev/null
  cmp -s "$ROOT_DIR/config/shared/vimrc" "$target_file" ||
    fail "Retrying after removing the forced failure did not recover"
}

test_managed_link_ln_failure_restores_preexisting_regular_file() {
  local link_path="$XDG_CONFIG_HOME/starship.toml"
  local state_file="$XDG_STATE_HOME/selfishell/resources/user-starship.state"
  # Keep the failing command off the persistent test PATH so retry uses the real one.
  local fake_bin="$TEST_ROOT/fakebin"
  local status=0

  mkdir -p "$(dirname "$link_path")"
  printf 'preexisting user starship config\n' >"$link_path"

  mkdir -p "$fake_bin"
  cat >"$fake_bin/ln" <<'EOF'
#!/usr/bin/env bash
for argument in "$@"; do
  case "$argument" in
    */.config/starship.toml) exit 1 ;;
  esac
done
exec /bin/ln "$@"
EOF
  chmod +x "$fake_bin/ln"

  set +e
  PATH="$fake_bin:/usr/bin:/bin" run_selfishell install --skip-packages --yes >"$TEST_ROOT/stdout" 2>"$TEST_ROOT/stderr"
  status=$?
  set -e

  ((status != 0)) || fail "A symlink creation failure must not be reported as success"
  [[ -f "$link_path" && ! -L "$link_path" ]] ||
    fail "The original Starship file was not restored after a failed link creation"
  assert_file_content 'preexisting user starship config' "$link_path"
  [[ ! -e "$state_file" ]] ||
    fail "A restored failed link install should not leave pending state behind"
  ! grep -Fq "Linked: $link_path" "$TEST_ROOT/stdout" ||
    fail "A failed link creation printed a success message"

  run_selfishell install --skip-packages --yes >/dev/null
  assert_symlink_to "$XDG_CONFIG_HOME/selfishell/starship.toml" "$link_path"
}

test_ghostty_preflight_stops_before_other_resources_install() {
  export SELFISHELL_TEST_SYSTEM_NAME=Darwin
  local target="$XDG_CONFIG_HOME/ghostty/config.ghostty"
  local dotfiles_source="$TEST_ROOT/dotfiles/config.ghostty"
  local status

  mkdir -p "$(dirname "$dotfiles_source")" "$(dirname "$target")"
  printf 'font-size = 14\n' >"$dotfiles_source"
  ln -s "$dotfiles_source" "$target"

  set +e
  run_selfishell install --skip-packages --yes >"$TEST_ROOT/stdout" 2>"$TEST_ROOT/stderr"
  status=$?
  set -e

  [[ "$status" -eq 1 ]] || fail "Ghostty preflight should stop installation"
  assert_symlink_to "$dotfiles_source" "$target"
  [[ ! -e "$XDG_CONFIG_HOME/selfishell" ]] ||
    fail "Ghostty preflight ran after other managed resources were already installed"
  [[ ! -e "$XDG_STATE_HOME/selfishell/resources" ]] ||
    fail "Ghostty preflight ran after other managed resource state was already created"
  ! grep -Fq 'Skipping package installation' "$TEST_ROOT/stdout" ||
    fail "Ghostty preflight did not run before the package-installation stage"
}

test_block_install_failure_cleans_up_temporary_files() {
  local target="$HOME/.zshrc"
  local state_file="$XDG_STATE_HOME/selfishell/resources/user-zshrc.state"
  local status=0
  local tmp_count

  printf 'original zshrc\n' >"$target"

  set +e
  bash -c '
    source "$1/lib/common.sh"
    source "$1/lib/paths.sh"
    selfishell_initialize_paths
    source "$1/lib/managed.sh"
    managed_block_content() { return 1; }
    managed_install_block user-zshrc "$2" 0
  ' _ "$ROOT_DIR" "$target" >/dev/null 2>"$TEST_ROOT/stderr"
  status=$?
  set -e

  [[ "$status" -ne 0 ]] || fail "A forced block-content failure should propagate as an error"
  tmp_count="$(find "$HOME" -maxdepth 1 -name '.zshrc.tmp.*' | wc -l)"
  [[ "$tmp_count" -eq 0 ]] || fail "A failed block install left a temporary file behind"
  assert_file_content 'original zshrc' "$target"
  [[ "$(sed -n '3p' "$state_file")" == pending ]] ||
    fail "A failed block install must not be recorded as active"
}

test_block_splice_preserves_surrounding_bytes_and_permissions() {
  local target="$HOME/block-target"
  local prefix="$TEST_ROOT/prefix" suffix="$TEST_ROOT/suffix"
  local expected="$TEST_ROOT/expected"

  source "$ROOT_DIR/lib/common.sh"
  source "$ROOT_DIR/lib/managed.sh"
  # Cross copy-buffer boundaries with multibyte text, NULs, CRLF, and no final newline.
  awk 'BEGIN { for (i = 0; i < 8192; i++) printf "personal config\r\n" }' >"$prefix"
  printf '앞\000뒤\n' >>"$prefix"
  cp "$prefix" "$suffix"
  printf 'no final newline' >>"$suffix"
  {
    cat "$prefix"
    printf 'old block\n'
    cat "$suffix"
  } >"$target"
  chmod 640 "$target"
  MANAGED_BLOCK_START="$(wc -c <"$prefix")"
  MANAGED_BLOCK_LENGTH=10

  managed_splice_block "$target" user-vimrc
  {
    cat "$prefix"
    managed_block_content user-vimrc
    cat "$suffix"
  } >"$expected"
  cmp -s "$expected" "$target" || fail "Block replacement changed surrounding bytes"
  MANAGED_BLOCK_LENGTH="$(managed_block_content user-vimrc | wc -c)"
  managed_splice_block "$target"
  cat "$prefix" "$suffix" >"$expected"
  cmp -s "$expected" "$target" || fail "Block removal changed surrounding bytes"
  [[ "$(find "$target" -prune -perm 640 -print)" == "$target" ]] || fail "Block splicing changed file permissions"
}

test_block_remove_failure_cleans_up_temporary_files() {
  local target="$HOME/.zshrc"
  local state_file="$XDG_STATE_HOME/selfishell/resources/user-zshrc.state"
  local before_checksum reader status tmp_count

  printf 'original zshrc\n' >"$target"
  run_selfishell install --skip-packages --yes >/dev/null
  {
    printf 'personal prefix\n'
    cat "$target"
  } >"$TEST_ROOT/before-zshrc"
  cp "$TEST_ROOT/before-zshrc" "$target"
  before_checksum="$(sed -n '7p' "$state_file")"

  for reader in dd head tail; do
    status=0
    bash -c '
      source "$1/lib/common.sh"
      source "$1/lib/paths.sh"
      selfishell_initialize_paths
      source "$1/lib/managed.sh"
      case "$3" in
        dd) dd() { return 1; } ;;
        head) head() { return 1; } ;;
        tail) tail() { return 1; } ;;
      esac
      managed_read_state user-zshrc
      managed_remove_block user-zshrc "$2"
    ' _ "$ROOT_DIR" "$target" "$reader" >/dev/null 2>"$TEST_ROOT/stderr" || status=$?

    [[ "$status" -ne 0 ]] || fail "A forced $reader failure during block removal should propagate as an error"
    tmp_count="$(find "$HOME" -maxdepth 1 -name '.zshrc.tmp.*' | wc -l)"
    [[ "$tmp_count" -eq 0 ]] || fail "A failed block removal left a temporary file behind"
    cmp -s "$TEST_ROOT/before-zshrc" "$target" || fail "A failed block removal changed user bytes"
    [[ "$(sed -n '7p' "$state_file")" == "$before_checksum" ]] ||
      fail "A failed block removal must not change resource state"
  done
}

test_block_install_chmod_failure_leaves_no_target_or_state() {
  local target="$HOME/.zshrc"
  local state_file="$XDG_STATE_HOME/selfishell/resources/user-zshrc.state"
  local helper="$TEST_ROOT/block-chmod-helper.bash"
  local status

  cat >"$helper" <<'EOF'
#!/usr/bin/env bash
source "$1/lib/common.sh"
source "$1/lib/paths.sh"
selfishell_initialize_paths
source "$1/lib/managed.sh"
chmod() { return 1; }
managed_install_block user-zshrc "$2" 0
EOF

  set +e
  bash "$helper" "$ROOT_DIR" "$target" >/dev/null 2>"$TEST_ROOT/stderr"
  status=$?
  set -e

  [[ "$status" -ne 0 ]] || fail "A forced chmod failure should propagate as an error"
  [[ ! -e "$target" ]] || fail "A failed chmod must not leave a partial block target"
  [[ "$(sed -n '3p' "$state_file")" == pending ]] ||
    fail "A failed chmod must not be recorded as active"
  [[ "$(find "$HOME" -maxdepth 1 -name '.zshrc.tmp.*' | wc -l)" -eq 0 ]] ||
    fail "A failed chmod left a temporary file behind"

  run_selfishell install --skip-packages --yes >/dev/null
  grep -Fqx '# >>> Selfishell initialize >>>' "$target" ||
    fail "Retrying after removing the forced chmod failure did not recover"
}

test_block_install_truncation_failure_preserves_target_and_state() {
  local target="$HOME/.zshrc"
  local state_file="$XDG_STATE_HOME/selfishell/resources/user-zshrc.state"
  local before_content
  local status=0

  printf 'original zshrc\n' >"$target"
  before_content="$(<"$target")"
  chmod 0444 "$target"

  set +e
  run_selfishell install --skip-packages --yes >"$TEST_ROOT/stdout" 2>"$TEST_ROOT/stderr"
  status=$?
  set -e
  chmod 0644 "$target"

  ((status != 0)) || fail "A forced truncation failure should propagate as an error"
  [[ "$(<"$target")" == "$before_content" ]] ||
    fail "A failed truncation must not change the existing target bytes"
  [[ "$(sed -n '3p' "$state_file")" == pending ]] ||
    fail "A failed truncation must not be recorded as active"
  [[ "$(find "$HOME" -maxdepth 1 -name '.zshrc.tmp.*' | wc -l)" -eq 0 ]] ||
    fail "A failed truncation left a temporary file behind"
  ! grep -Fq 'Added Selfishell block' "$TEST_ROOT/stdout" ||
    fail "A failed truncation printed a success message"

  run_selfishell install --skip-packages --yes >/dev/null
  grep -Fqx '# >>> Selfishell initialize >>>' "$target" ||
    fail "Retrying after removing the forced permission failure did not recover"
}

test_block_remove_truncation_failure_preserves_block_and_state() {
  local target="$HOME/.zshrc"
  local state_file="$XDG_STATE_HOME/selfishell/resources/user-zshrc.state"
  local before_content
  local status=0

  run_selfishell install --skip-packages --yes >/dev/null
  before_content="$(<"$target")"
  chmod 0444 "$target"

  set +e
  run_selfishell uninstall --yes >"$TEST_ROOT/stdout" 2>"$TEST_ROOT/stderr"
  status=$?
  set -e
  chmod 0644 "$target"

  ((status != 0)) || fail "A forced removal-truncation failure should propagate as an error"
  [[ "$(<"$target")" == "$before_content" ]] ||
    fail "A failed block removal must not change the existing target bytes"
  grep -Fqx '# >>> Selfishell initialize >>>' "$target" ||
    fail "A failed block removal must leave the managed block in place"
  [[ -e "$state_file" ]] || fail "A failed block removal must not delete the resource state"
  [[ "$(find "$HOME" -maxdepth 1 -name '.zshrc.tmp.*' | wc -l)" -eq 0 ]] ||
    fail "A failed block removal left a temporary file behind"
  ! grep -Fq 'Selfishell configuration uninstalled' "$TEST_ROOT/stdout" ||
    fail "A failed block removal printed a success message"

  run_selfishell uninstall --yes >/dev/null
  [[ ! -e "$state_file" ]] ||
    fail "Retrying after removing the forced permission failure did not clear resource state"
  ! grep -Fqx '# >>> Selfishell initialize >>>' "$target" 2>/dev/null ||
    fail "Retrying after removing the forced permission failure did not remove the managed block"
}

test_install_final_state_write_failure_does_not_report_success() {
  local rc=0
  local before_configured
  local before_ghostty
  local tmp_count

  run_selfishell install --skip-packages --yes >/dev/null
  before_configured="$(<"$XDG_STATE_HOME/selfishell/configured")"
  before_ghostty="$(<"$XDG_STATE_HOME/selfishell/ghostty")"

  chmod 0555 "$XDG_STATE_HOME/selfishell"
  run_selfishell install --skip-packages --yes >"$TEST_ROOT/stdout" 2>"$TEST_ROOT/stderr" || rc=$?
  chmod 0755 "$XDG_STATE_HOME/selfishell"

  ((rc != 0)) || fail "A final state write failure must not be reported as success"
  ! grep -Fq 'Selfishell configuration installed.' "$TEST_ROOT/stdout" ||
    fail "A failed install printed the success message"
  tmp_count="$(find "$XDG_STATE_HOME/selfishell" -maxdepth 1 \( -name 'configured.tmp.*' -o -name 'ghostty.tmp.*' \) | wc -l)"
  [[ "$tmp_count" -eq 0 ]] || fail "A failed final state write left a temporary file behind"
  [[ "$(<"$XDG_STATE_HOME/selfishell/configured")" == "$before_configured" ]] ||
    fail "A failed final state write must not corrupt the existing configuration marker"
  [[ "$(<"$XDG_STATE_HOME/selfishell/ghostty")" == "$before_ghostty" ]] ||
    fail "A failed final state write must not corrupt the existing ghostty state"
}

test_managed_file_dry_run_conflict_changes_nothing() {
  run_selfishell install --skip-packages --yes >/dev/null

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

  run_selfishell install --skip-packages --yes >/dev/null

  local state_file="$XDG_STATE_HOME/selfishell/resources/vimrc.state"
  local original_backup
  original_backup="$(sed -n '6p' "$state_file")"
  [[ "$original_backup" != "-" ]] || fail "Installation backup was not recorded for a pre-existing file"
  assert_file_content 'original-before-install' "$original_backup"

  printf 'user-modification-after-install\n' >"$target_file"
  printf 'y\ny\n' | SELFISHELL_TEST_TTY=1 run_selfishell install --skip-packages >/dev/null

  [[ "$(sed -n '6p' "$state_file")" == "$original_backup" ]] ||
    fail "Overwriting a conflict must keep the original installation backup"
  cmp -s "$ROOT_DIR/config/shared/vimrc" "$target_file" || fail "Overwrite did not install the default managed file"

  local conflict_backup
  conflict_backup="$(find "$XDG_STATE_HOME/selfishell/backups" -name 'vimrc.backup.*' 2>/dev/null | head -1)"
  [[ -n "$conflict_backup" ]] || fail "No conflict backup was created for the overwrite"
  assert_file_content 'user-modification-after-install' "$conflict_backup"

  run_selfishell uninstall --restore --yes >/dev/null
  assert_file_content 'original-before-install' "$target_file"
}

run_discovered_tests_parallel \
  "${SELFISHELL_TEST_JOBS:-8}" \
  setup_managed_home \
  teardown_managed_home
