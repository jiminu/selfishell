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

test_update_notice_reads_installed_version_file() {
  local fake_root output

  setup_test_home
  fake_root="$TEST_ROOT/releases/1.2.3"
  mkdir -p "$fake_root/bin"
  printf '1.2.3\n' >"$fake_root/VERSION"
  printf '#!/usr/bin/env bash\nprintf "selfishell 9.9.9\\n"\n' >"$fake_root/bin/selfishell"
  chmod +x "$fake_root/bin/selfishell"

  output="$(
    PATH="$fake_root/bin:/usr/bin:/bin" \
      /bin/zsh -f -c '
        load_nvm() { :; }
        source "$1"
        _selfishell_current_version
      ' zsh "$ROOT_DIR/common/common.zsh"
  )"

  [[ "$output" == 1.2.3 ]] || fail "Update notice did not read the installed VERSION file"
  teardown_test_home
}

test_update_notice_uses_cache_and_refreshes_in_background_format() {
  local fake_bin cache_dir output now

  setup_test_home
  fake_bin="$TEST_ROOT/bin"
  cache_dir="$HOME/.cache/selfishell"
  now="$(date +%s)"
  mkdir -p "$fake_bin" "$cache_dir"
  # Positional parameters must expand in the generated mock, not this test.
  # shellcheck disable=SC2016
  printf '%s\n' \
    '#!/usr/bin/env bash' \
    'if [[ "${2:-}" == "--available" ]]; then' \
    '  printf "1.1.0\\n"' \
    'else' \
    '  printf "selfishell 1.0.0\\n"' \
    'fi' >"$fake_bin/selfishell"
  chmod +x "$fake_bin/selfishell"
  printf '1.1.0\n' >"$cache_dir/available-version"
  printf '%s\n' "$now" >"$cache_dir/update-checked-at"

  output="$(
    XDG_CACHE_HOME="$HOME/.cache" \
      PATH="$fake_bin:/usr/bin:/bin" \
      /bin/zsh -f -c '
        load_nvm() { :; }
        source "$1"
        ! _selfishell_version_is_newer 0.1.0-beta.9 0.1.0-beta.12
        _selfishell_version_is_newer 0.1.0-beta.13 0.1.0-beta.12
        _selfishell_version_is_newer 0.1.0 0.1.0-beta.12
        ! _selfishell_version_is_newer 0.1.0-beta.12 0.1.0
        _selfishell_update_notice
        SELFISHELL_UPDATE_NOTICE=0 _selfishell_update_notice
        command rm -f "$2/available-version" "$2/update-checked-at"
        _selfishell_update_notice_refresh "$2" 12345
        [[ "$(<"$2/available-version")" == 1.1.0 ]]
        [[ "$(<"$2/update-checked-at")" == 12345 ]]
        command rm -f "$2/available-version" "$2/update-checked-at"
        SELFISHELL_UPDATE_CHECK_INTERVAL=0 _selfishell_update_notice
        for attempt in {1..40}; do
          [[ -r "$2/available-version" ]] && break
          command sleep 0.05
        done
        [[ "$(<"$2/available-version")" == 1.1.0 ]]
      ' zsh "$ROOT_DIR/common/common.zsh" "$cache_dir"
  )"

  [[ "$output" == '[Selfishell] 1.1.0 is available. Run: sfs update' ]] ||
    fail "Default update notice did not use cached version metadata"
  teardown_test_home
}

test_minimal_profile_initializes_git_completion_without_zinit
printf 'PASS: test_minimal_profile_initializes_git_completion_without_zinit\n'
test_macos_managed_zsh_adds_default_cli_prefix_to_path
printf 'PASS: test_macos_managed_zsh_adds_default_cli_prefix_to_path\n'
test_update_notice_reads_installed_version_file
printf 'PASS: test_update_notice_reads_installed_version_file\n'
test_update_notice_uses_cache_and_refreshes_in_background_format
printf 'PASS: test_update_notice_uses_cache_and_refreshes_in_background_format\n'
