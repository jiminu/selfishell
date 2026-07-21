#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/tests/test_helper.bash"

test_minimal_profile_initializes_git_completion_without_zinit() {
  setup_test_home
  local output

  output="$(
    XDG_CACHE_HOME="$HOME/.cache" \
      ZDOTDIR="" \
      PATH="/usr/bin:/bin" \
      /bin/zsh -f -c '
        load_nvm() { :; }
        source "$1"
        (( $+functions[_git] ))
        [[ -s "$HOME/.zcompdump" ]]
      ' zsh "$ROOT_DIR/common/common.zsh" 2>&1
  )"

  [[ -z "$output" ]] || fail "Missing zinit should not emit stderr noise"

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
    HOMEBREW_PREFIX='' \
    PATH="$fake_bin:/usr/bin:/bin" \
    /bin/zsh -f -c '
      source "$1"
      [[ "$path[1]" == "$HOME/.local/bin" ]]
      [[ "$path[2]" == "$HOME/.rd/bin" ]]
    ' zsh "$ROOT_DIR/mac/.zshrc"

  teardown_test_home
}

test_wsl_defers_windows_path_during_initialization() {
  local output
  local test_home

  setup_test_home
  test_home="$HOME"
  mkdir -p "$HOME/.config/selfishell/zsh"
  # The expression must be evaluated by the generated Zsh fixture.
  # shellcheck disable=SC2016
  printf '%s\n' \
    '[[ ${path[(I)/mnt/[a-zA-Z]/*]} -eq 0 ]] || return 1' \
    'SELFISHELL_TEST_INITIALIZED=1' >"$HOME/.config/selfishell/zsh/common.zsh"

  output="$(
    HOME="$test_home" XDG_CONFIG_HOME="$test_home/.config" \
      WSL_DISTRO_NAME=Ubuntu-24.04 PATH="/usr/bin:/mnt/c/Windows:/bin" \
      /bin/zsh -f -c '
        source "$1"
        [[ "$SELFISHELL_TEST_INITIALIZED" == 1 ]]
        print -r -- "${(j.:.)path}"
      ' zsh "$ROOT_DIR/ubuntu/.zshrc"
  )"

  [[ "$output" == "$HOME/.local/bin:$HOME/.rd/bin:/usr/bin:/bin:/mnt/c/Windows" ]] ||
    fail "WSL PATH was not restored after initialization: $output"
  teardown_test_home
}

test_mise_uses_selfishell_config_only_for_developer_profile() {
  local fake_bin developer_config minimal_config

  setup_test_home
  fake_bin="$TEST_ROOT/bin"
  mkdir -p "$fake_bin" "$HOME/.local/state/selfishell"
  cat >"$fake_bin/mise" <<'EOF'
#!/bin/sh
if [ "$1" = activate ]; then
  printf 'export SELFISHELL_TEST_MISE_ACTIVATED=1\n'
fi
EOF
  chmod +x "$fake_bin/mise"
  printf 'developer\n' >"$HOME/.local/state/selfishell/profile"

  developer_config="$(
    PATH="$fake_bin:/usr/bin:/bin" \
      XDG_CONFIG_HOME="$HOME/.config" \
      ZDOTDIR="" \
      MISE_GLOBAL_CONFIG_FILE="" \
      /bin/zsh -f -c '
      _selfishell_command_path() { command -v "$1"; }
      source "$1"
      [[ "$SELFISHELL_TEST_MISE_ACTIVATED" == 1 ]]
      print -r -- "$MISE_GLOBAL_CONFIG_FILE"
    ' zsh "$ROOT_DIR/common/runtime.zsh"
  )"

  printf 'minimal\n' >"$HOME/.local/state/selfishell/profile"
  minimal_config="$(
    PATH="$fake_bin:/usr/bin:/bin" \
      XDG_CONFIG_HOME="$HOME/.config" \
      ZDOTDIR="" \
      MISE_GLOBAL_CONFIG_FILE="$HOME/personal-mise.toml" \
      /bin/zsh -f -c '
        _selfishell_command_path() { command -v "$1"; }
        source "$1"
        print -r -- "$MISE_GLOBAL_CONFIG_FILE"
      ' zsh "$ROOT_DIR/common/runtime.zsh"
  )"

  [[ -z "$developer_config" ]] ||
    fail "Developer profile set MISE_GLOBAL_CONFIG_FILE"
  [[ "$minimal_config" == "$HOME/personal-mise.toml" ]] ||
    fail "Minimal profile replaced the user's mise config"
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
      ZDOTDIR="" \
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
    '  printf "selfishell 0.2.0\\n"' \
    'fi' >"$fake_bin/selfishell"
  chmod +x "$fake_bin/selfishell"
  printf '1.1.0\n' >"$cache_dir/available-version"
  printf '%s\n' "$now" >"$cache_dir/update-checked-at"

  output="$(
    XDG_CACHE_HOME="$HOME/.cache" \
      ZDOTDIR="" \
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

  [[ "$output" == '[Selfishell] 1.1.0 is available. Run: selfishell update' ]] ||
    fail "Default update notice did not use cached version metadata"
  teardown_test_home
}

test_update_notice_stale_lock_is_reclaimed_after_ttl() {
  local fake_bin cache_dir output
  local stale_created_at

  setup_test_home
  fake_bin="$TEST_ROOT/bin"
  cache_dir="$HOME/.cache/selfishell"
  mkdir -p "$fake_bin" "$cache_dir/update-check.lock"
  # Positional parameters must expand in the generated mock, not this test.
  # shellcheck disable=SC2016
  printf '%s\n' \
    '#!/usr/bin/env bash' \
    'if [[ "${2:-}" == "--available" ]]; then' \
    '  printf "2.0.0\\n"' \
    'else' \
    '  printf "selfishell 0.2.0\\n"' \
    'fi' >"$fake_bin/selfishell"
  chmod +x "$fake_bin/selfishell"

  # Simulate a lock left behind by a refresh that was killed mid-run (e.g.
  # the terminal closed) well past the default TTL.
  stale_created_at=$(($(date +%s) - 700))
  printf '99999\n' >"$cache_dir/update-check.lock/pid"
  printf '%s\n' "$stale_created_at" >"$cache_dir/update-check.lock/created_at"

  output="$(
    PATH="$fake_bin:/usr/bin:/bin" \
      /bin/zsh -f -c '
        source "$1"
        _selfishell_update_notice_refresh "$2" 12345
        [[ -e "$2/update-check.lock" ]] && print "LOCK_LEFT" || print "LOCK_CLEARED"
        cat "$2/available-version" 2>/dev/null
      ' zsh "$ROOT_DIR/common/update-notice.zsh" "$cache_dir"
  )"

  [[ "$output" == *'LOCK_CLEARED'* ]] ||
    fail "A stale lock older than the TTL was not reclaimed and cleared: $output"
  [[ "$output" == *'2.0.0'* ]] ||
    fail "Reclaiming a stale lock did not perform the refresh: $output"
  teardown_test_home
}

test_update_notice_fresh_lock_blocks_concurrent_refresh() {
  local fake_bin cache_dir output
  local fresh_created_at

  setup_test_home
  fake_bin="$TEST_ROOT/bin"
  cache_dir="$HOME/.cache/selfishell"
  mkdir -p "$fake_bin" "$cache_dir/update-check.lock"
  printf '#!/usr/bin/env bash\nprintf "2.0.0\\n"\n' >"$fake_bin/selfishell"
  chmod +x "$fake_bin/selfishell"

  fresh_created_at="$(date +%s)"
  printf '99999\n' >"$cache_dir/update-check.lock/pid"
  printf '%s\n' "$fresh_created_at" >"$cache_dir/update-check.lock/created_at"

  output="$(
    PATH="$fake_bin:/usr/bin:/bin" \
      /bin/zsh -f -c '
        source "$1"
        _selfishell_update_notice_refresh "$2" 12345
        [[ -e "$2/update-check.lock" ]] && print "LOCK_LEFT" || print "LOCK_CLEARED"
        [[ -e "$2/available-version" ]] && print "VERSION_WRITTEN" || print "VERSION_ABSENT"
      ' zsh "$ROOT_DIR/common/update-notice.zsh" "$cache_dir"
  )"

  [[ "$output" == *'LOCK_LEFT'* ]] ||
    fail "A fresh, still-held lock was incorrectly reclaimed: $output"
  [[ "$output" == *'VERSION_ABSENT'* ]] ||
    fail "A concurrent refresh ran despite a fresh lock still being held: $output"
  teardown_test_home
}

test_update_notice_stale_empty_lock_directory_is_reclaimed() {
  local fake_bin cache_dir output

  setup_test_home
  fake_bin="$TEST_ROOT/bin"
  cache_dir="$HOME/.cache/selfishell"
  mkdir -p "$fake_bin" "$cache_dir/update-check.lock"
  printf '#!/usr/bin/env bash\nprintf "2.0.0\\n"\n' >"$fake_bin/selfishell"
  chmod +x "$fake_bin/selfishell"
  # No pid/created_at at all -- either a lock left by a Selfishell version
  # that predates lock metadata, or a writer that died between mkdir and
  # its first write -- so only the directory's own (old) mtime is left to
  # judge staleness by.
  touch -t 202001010000 "$cache_dir/update-check.lock"

  output="$(
    PATH="$fake_bin:/usr/bin:/bin" \
      /bin/zsh -f -c '
        source "$1"
        _selfishell_update_notice_refresh "$2" 12345
        [[ -e "$2/update-check.lock" ]] && print "LOCK_LEFT" || print "LOCK_CLEARED"
      ' zsh "$ROOT_DIR/common/update-notice.zsh" "$cache_dir"
  )"

  [[ "$output" == *'LOCK_CLEARED'* ]] ||
    fail "A stale, metadata-less lock directory was not reclaimed: $output"
  teardown_test_home
}

test_update_notice_fresh_empty_lock_directory_is_preserved() {
  local fake_bin cache_dir output

  setup_test_home
  fake_bin="$TEST_ROOT/bin"
  cache_dir="$HOME/.cache/selfishell"
  mkdir -p "$fake_bin" "$cache_dir/update-check.lock"
  printf '#!/usr/bin/env bash\nprintf "2.0.0\\n"\n' >"$fake_bin/selfishell"
  chmod +x "$fake_bin/selfishell"

  output="$(
    PATH="$fake_bin:/usr/bin:/bin" \
      /bin/zsh -f -c '
        source "$1"
        _selfishell_update_notice_refresh "$2" 12345
        [[ -e "$2/update-check.lock" ]] && print "LOCK_LEFT" || print "LOCK_CLEARED"
        [[ -e "$2/available-version" ]] && print "VERSION_WRITTEN" || print "VERSION_ABSENT"
      ' zsh "$ROOT_DIR/common/update-notice.zsh" "$cache_dir"
  )"

  [[ "$output" == *'LOCK_LEFT'* ]] ||
    fail "A fresh, metadata-less lock directory was incorrectly reclaimed: $output"
  [[ "$output" == *'VERSION_ABSENT'* ]] ||
    fail "A concurrent refresh ran despite a fresh metadata-less lock: $output"
  teardown_test_home
}

test_update_notice_stale_lock_with_only_pid_is_reclaimed() {
  local fake_bin cache_dir output

  setup_test_home
  fake_bin="$TEST_ROOT/bin"
  cache_dir="$HOME/.cache/selfishell"
  mkdir -p "$fake_bin" "$cache_dir/update-check.lock"
  printf '#!/usr/bin/env bash\nprintf "2.0.0\\n"\n' >"$fake_bin/selfishell"
  chmod +x "$fake_bin/selfishell"
  printf '99999\n' >"$cache_dir/update-check.lock/pid"
  touch -t 202001010000 "$cache_dir/update-check.lock/pid" "$cache_dir/update-check.lock"

  output="$(
    PATH="$fake_bin:/usr/bin:/bin" \
      /bin/zsh -f -c '
        source "$1"
        _selfishell_update_notice_refresh "$2" 12345
        [[ -e "$2/update-check.lock" ]] && print "LOCK_LEFT" || print "LOCK_CLEARED"
      ' zsh "$ROOT_DIR/common/update-notice.zsh" "$cache_dir"
  )"

  [[ "$output" == *'LOCK_CLEARED'* ]] ||
    fail "A stale lock with only a pid file was not reclaimed: $output"
  teardown_test_home
}

test_update_notice_corrupt_created_at_falls_back_to_directory_mtime() {
  local fake_bin cache_dir output label

  setup_test_home
  fake_bin="$TEST_ROOT/bin"
  cache_dir="$HOME/.cache/selfishell"
  mkdir -p "$fake_bin"
  printf '#!/usr/bin/env bash\nprintf "2.0.0\\n"\n' >"$fake_bin/selfishell"
  chmod +x "$fake_bin/selfishell"

  for label in stale fresh; do
    mkdir -p "$cache_dir/update-check.lock"
    printf 'not-a-timestamp\n' >"$cache_dir/update-check.lock/created_at"
    [[ "$label" == stale ]] && touch -t 202001010000 "$cache_dir/update-check.lock/created_at" "$cache_dir/update-check.lock"

    output="$(
      PATH="$fake_bin:/usr/bin:/bin" \
        /bin/zsh -f -c '
          source "$1"
          _selfishell_update_notice_refresh "$2" 12345
          [[ -e "$2/update-check.lock" ]] && print "LOCK_LEFT" || print "LOCK_CLEARED"
        ' zsh "$ROOT_DIR/common/update-notice.zsh" "$cache_dir"
    )"

    if [[ "$label" == stale ]]; then
      [[ "$output" == *'LOCK_CLEARED'* ]] ||
        fail "A corrupt created_at backed by an old directory mtime was not reclaimed: $output"
    else
      [[ "$output" == *'LOCK_LEFT'* ]] ||
        fail "A corrupt created_at backed by a fresh directory mtime was incorrectly reclaimed: $output"
    fi
    rm -rf "$cache_dir/update-check.lock" "$cache_dir/available-version" "$cache_dir/update-checked-at"
  done
  teardown_test_home
}

test_update_notice_unreadable_created_at_falls_back_to_directory_mtime() {
  local fake_bin cache_dir output

  # Permission bits don't restrict root's own reads, so this scenario can't
  # be produced when running as root (e.g. some containers).
  [[ "$(id -u)" != 0 ]] || return 0

  setup_test_home
  fake_bin="$TEST_ROOT/bin"
  cache_dir="$HOME/.cache/selfishell"
  mkdir -p "$fake_bin" "$cache_dir/update-check.lock"
  printf '#!/usr/bin/env bash\nprintf "2.0.0\\n"\n' >"$fake_bin/selfishell"
  chmod +x "$fake_bin/selfishell"
  printf '%s\n' "$(date +%s)" >"$cache_dir/update-check.lock/created_at"
  chmod 000 "$cache_dir/update-check.lock/created_at"
  touch -t 202001010000 "$cache_dir/update-check.lock"

  output="$(
    PATH="$fake_bin:/usr/bin:/bin" \
      /bin/zsh -f -c '
        source "$1"
        _selfishell_update_notice_refresh "$2" 12345
        [[ -e "$2/update-check.lock" ]] && print "LOCK_LEFT" || print "LOCK_CLEARED"
      ' zsh "$ROOT_DIR/common/update-notice.zsh" "$cache_dir"
  )"

  chmod 644 "$cache_dir/update-check.lock/created_at" 2>/dev/null || true
  [[ "$output" == *'LOCK_CLEARED'* ]] ||
    fail "An unreadable created_at backed by an old directory mtime was not reclaimed: $output"
  teardown_test_home
}

test_update_notice_future_created_at_is_preserved() {
  local fake_bin cache_dir output future_created_at

  setup_test_home
  fake_bin="$TEST_ROOT/bin"
  cache_dir="$HOME/.cache/selfishell"
  mkdir -p "$fake_bin" "$cache_dir/update-check.lock"
  printf '#!/usr/bin/env bash\nprintf "2.0.0\\n"\n' >"$fake_bin/selfishell"
  chmod +x "$fake_bin/selfishell"
  future_created_at=$(($(date +%s) + 100000))
  printf '%s\n' "$future_created_at" >"$cache_dir/update-check.lock/created_at"

  output="$(
    PATH="$fake_bin:/usr/bin:/bin" \
      /bin/zsh -f -c '
        source "$1"
        _selfishell_update_notice_refresh "$2" 12345
        [[ -e "$2/update-check.lock" ]] && print "LOCK_LEFT" || print "LOCK_CLEARED"
      ' zsh "$ROOT_DIR/common/update-notice.zsh" "$cache_dir"
  )"

  [[ "$output" == *'LOCK_LEFT'* ]] ||
    fail "A lock with a future created_at was incorrectly reclaimed: $output"
  teardown_test_home
}

test_update_notice_lock_ttl_rejects_invalid_values() {
  local fake_bin cache_dir output ttl stale_created_at

  setup_test_home
  fake_bin="$TEST_ROOT/bin"
  cache_dir="$HOME/.cache/selfishell"
  mkdir -p "$fake_bin"
  printf '#!/usr/bin/env bash\nprintf "2.0.0\\n"\n' >"$fake_bin/selfishell"
  chmod +x "$fake_bin/selfishell"
  stale_created_at=$(($(date +%s) - 700))

  for ttl in abc -100 1.5 0 ''; do
    mkdir -p "$cache_dir/update-check.lock"
    printf '%s\n' "$stale_created_at" >"$cache_dir/update-check.lock/created_at"

    output="$(
      SELFISHELL_UPDATE_LOCK_TTL="$ttl" PATH="$fake_bin:/usr/bin:/bin" \
        /bin/zsh -f -c '
          source "$1"
          _selfishell_update_notice_refresh "$2" 12345
          [[ -e "$2/update-check.lock" ]] && print "LOCK_LEFT" || print "LOCK_CLEARED"
        ' zsh "$ROOT_DIR/common/update-notice.zsh" "$cache_dir"
    )"

    [[ "$output" == *'LOCK_CLEARED'* ]] ||
      fail "An invalid SELFISHELL_UPDATE_LOCK_TTL='$ttl' did not fall back to the default TTL: $output"
    rm -rf "$cache_dir/update-check.lock" "$cache_dir/available-version" "$cache_dir/update-checked-at"
  done
  teardown_test_home
}

test_update_notice_lock_ttl_zero_does_not_mean_instantly_stale() {
  local fake_bin cache_dir output

  setup_test_home
  fake_bin="$TEST_ROOT/bin"
  cache_dir="$HOME/.cache/selfishell"
  mkdir -p "$fake_bin" "$cache_dir/update-check.lock"
  printf '#!/usr/bin/env bash\nprintf "2.0.0\\n"\n' >"$fake_bin/selfishell"
  chmod +x "$fake_bin/selfishell"
  printf '%s\n' "$(($(date +%s) - 2))" >"$cache_dir/update-check.lock/created_at"

  output="$(
    SELFISHELL_UPDATE_LOCK_TTL=0 PATH="$fake_bin:/usr/bin:/bin" \
      /bin/zsh -f -c '
        source "$1"
        _selfishell_update_notice_refresh "$2" 12345
        [[ -e "$2/update-check.lock" ]] && print "LOCK_LEFT" || print "LOCK_CLEARED"
      ' zsh "$ROOT_DIR/common/update-notice.zsh" "$cache_dir"
  )"

  [[ "$output" == *'LOCK_LEFT'* ]] ||
    fail "SELFISHELL_UPDATE_LOCK_TTL=0 treated a 2-second-old lock as instantly stale instead of falling back to the default: $output"
  teardown_test_home
}

test_update_notice_lock_ttl_honors_valid_custom_value() {
  local fake_bin cache_dir output

  setup_test_home
  fake_bin="$TEST_ROOT/bin"
  cache_dir="$HOME/.cache/selfishell"
  mkdir -p "$fake_bin" "$cache_dir/update-check.lock"
  printf '#!/usr/bin/env bash\nprintf "2.0.0\\n"\n' >"$fake_bin/selfishell"
  chmod +x "$fake_bin/selfishell"
  printf '%s\n' "$(($(date +%s) - 5))" >"$cache_dir/update-check.lock/created_at"

  output="$(
    SELFISHELL_UPDATE_LOCK_TTL=2 PATH="$fake_bin:/usr/bin:/bin" \
      /bin/zsh -f -c '
        source "$1"
        _selfishell_update_notice_refresh "$2" 12345
        [[ -e "$2/update-check.lock" ]] && print "LOCK_LEFT" || print "LOCK_CLEARED"
      ' zsh "$ROOT_DIR/common/update-notice.zsh" "$cache_dir"
  )"

  [[ "$output" == *'LOCK_CLEARED'* ]] ||
    fail "A valid custom SELFISHELL_UPDATE_LOCK_TTL was not honored: $output"
  teardown_test_home
}

test_update_notice_refresh_removes_lock_even_when_version_lookup_fails() {
  local cache_dir output

  setup_test_home
  cache_dir="$HOME/.cache/selfishell"
  mkdir -p "$cache_dir"

  output="$(
    PATH="/usr/bin:/bin" \
      /bin/zsh -f -c '
        source "$1"
        _selfishell_update_notice_refresh "$2" 12345
        [[ -e "$2/update-check.lock" ]] && print "LOCK_LEFT" || print "LOCK_CLEARED"
      ' zsh "$ROOT_DIR/common/update-notice.zsh" "$cache_dir"
  )"

  [[ "$output" == *'LOCK_CLEARED'* ]] ||
    fail "The lock was not released after a failed version lookup: $output"
  teardown_test_home
}

test_update_lock_stale_since_preserves_lock_when_age_cannot_be_determined() {
  local output

  setup_test_home
  output="$(
    /bin/zsh -f -c '
      source "$1"
      if result="$(_selfishell_update_lock_stale_since "$2" 600 99999999999)"; then
        print "DETERMINED:$result"
      else
        print "PRESERVED"
      fi
    ' zsh "$ROOT_DIR/common/update-notice.zsh" "$TEST_ROOT/no-such-lock-dir"
  )"

  [[ "$output" == 'PRESERVED' ]] ||
    fail "A lock whose age cannot be determined at all should be left alone: $output"
  teardown_test_home
}

test_shell_tool_cache_generation_succeeds_atomically() {
  local cache_dir output

  setup_test_home
  cache_dir="$HOME/.cache/selfishell"
  mkdir -p "$cache_dir"

  output="$(
    ZDOTDIR="" PATH="/usr/bin:/bin" SELFISHELL_COMMON_DIR="$ROOT_DIR/common" \
      /bin/zsh -f -c '
        _selfishell_command_path() { command -v "$1"; }
        source "$1"
        _selfishell_generate_zsh_cache "$2/cache.zsh" echo "print ok"
        [[ -s "$2/cache.zsh" ]] && cat "$2/cache.zsh"
        command find "$2" -maxdepth 1 -name "*.tmp.*" | command wc -l | command tr -d " "
      ' zsh "$ROOT_DIR/common/interactive.zsh" "$cache_dir"
  )"

  [[ "$output" == *'print ok'* ]] || fail "A successful cache generation did not write the expected content: $output"
  [[ "$output" == *$'\n0' ]] || fail "A successful cache generation left a temporary file behind: $output"
  teardown_test_home
}

test_shell_tool_cache_generation_failures_preserve_existing_cache() {
  local cache_dir output label

  setup_test_home
  cache_dir="$HOME/.cache/selfishell"
  mkdir -p "$cache_dir"

  for label in nonzero-exit empty-output invalid-syntax; do
    output="$(
      ZDOTDIR="" PATH="/usr/bin:/bin" SELFISHELL_COMMON_DIR="$ROOT_DIR/common" \
        /bin/zsh -f -c '
          _selfishell_command_path() { command -v "$1"; }
          source "$1"
          print -r -- "# preexisting cache" >| "$2/cache.zsh"

          case "$3" in
            nonzero-exit) fake_tool() { print "partial"; return 1 } ;;
            empty-output) fake_tool() { :; } ;;
            invalid-syntax) fake_tool() { print "if [[ not valid zsh"; } ;;
          esac

          _selfishell_generate_zsh_cache "$2/cache.zsh" fake_tool
          cat "$2/cache.zsh"
          command find "$2" -maxdepth 1 -name "*.tmp.*" | command wc -l | command tr -d " "
        ' zsh "$ROOT_DIR/common/interactive.zsh" "$cache_dir" "$label"
    )"

    [[ "$output" == *'# preexisting cache'* ]] ||
      fail "A $label failure corrupted the existing cache: $output"
    [[ "$output" == *$'\n0' ]] ||
      fail "A $label failure left a temporary file behind: $output"
  done
  teardown_test_home
}

test_shell_tool_cache_regenerates_when_binary_is_newer() {
  local fake_bin cache_dir output

  setup_test_home
  fake_bin="$TEST_ROOT/bin"
  cache_dir="$HOME/.cache/selfishell"
  mkdir -p "$fake_bin" "$cache_dir"
  cat >"$fake_bin/zoxide" <<'EOF'
#!/usr/bin/env bash
printf 'echo regenerated\n'
EOF
  chmod +x "$fake_bin/zoxide"
  printf '# stale cache\n' >"$cache_dir/zoxide-init.zsh"
  # -t, not -d: -d is a GNU extension BSD/macOS touch doesn't support.
  touch -t 202001010000 "$cache_dir/zoxide-init.zsh"
  touch "$fake_bin/zoxide"

  output="$(
    ZDOTDIR="" PATH="$fake_bin:/usr/bin:/bin" SELFISHELL_COMMON_DIR="$ROOT_DIR/common" \
      XDG_CONFIG_HOME="$HOME/.config" XDG_CACHE_HOME="$HOME/.cache" \
      /bin/zsh -f -c '_selfishell_command_path() { command -v "$1"; }; source "$1"' \
      zsh "$ROOT_DIR/common/interactive.zsh" 2>/dev/null
    cat "$cache_dir/zoxide-init.zsh"
  )"

  [[ "$output" == *'regenerated'* ]] ||
    fail "Cache was not regenerated when the tool binary is newer than the cache: $output"
  teardown_test_home
}

test_shell_tool_cache_does_not_regenerate_when_cache_is_newer_than_binary() {
  local fake_bin cache_dir output

  setup_test_home
  fake_bin="$TEST_ROOT/bin"
  cache_dir="$HOME/.cache/selfishell"
  mkdir -p "$fake_bin" "$cache_dir"
  cat >"$fake_bin/zoxide" <<'EOF'
#!/usr/bin/env bash
printf 'echo regenerated\n'
EOF
  chmod +x "$fake_bin/zoxide"
  touch -t 202001010000 "$fake_bin/zoxide"
  printf '# already current\n' >"$cache_dir/zoxide-init.zsh"

  output="$(
    ZDOTDIR="" PATH="$fake_bin:/usr/bin:/bin" SELFISHELL_COMMON_DIR="$ROOT_DIR/common" \
      XDG_CONFIG_HOME="$HOME/.config" XDG_CACHE_HOME="$HOME/.cache" \
      /bin/zsh -f -c '_selfishell_command_path() { command -v "$1"; }; source "$1"' \
      zsh "$ROOT_DIR/common/interactive.zsh" 2>/dev/null
    cat "$cache_dir/zoxide-init.zsh"
  )"

  [[ "$output" == *'# already current'* ]] ||
    fail "Cache was regenerated even though it is newer than the tool binary: $output"
  [[ "$output" != *'regenerated'* ]] ||
    fail "The tool was invoked even though its cache is already current: $output"
  teardown_test_home
}

test_shell_tool_cache_write_failure_preserves_existing_cache() {
  local cache_dir output

  setup_test_home
  cache_dir="$HOME/.cache/selfishell"
  mkdir -p "$cache_dir"
  printf '# preexisting cache\n' >"$cache_dir/cache.zsh"
  chmod 555 "$cache_dir"

  output="$(
    ZDOTDIR="" PATH="/usr/bin:/bin" SELFISHELL_COMMON_DIR="$ROOT_DIR/common" \
      /bin/zsh -f -c '
        _selfishell_command_path() { command -v "$1"; }
        source "$1"
        _selfishell_generate_zsh_cache "$2/cache.zsh" echo "print ok"
        cat "$2/cache.zsh"
        command find "$2" -maxdepth 1 -name "*.tmp.*" 2>/dev/null | command wc -l | command tr -d " "
      ' zsh "$ROOT_DIR/common/interactive.zsh" "$cache_dir" 2>/dev/null
  )"
  chmod 755 "$cache_dir"

  [[ "$(id -u)" == 0 ]] || {
    [[ "$output" == *'# preexisting cache'* ]] ||
      fail "A write failure corrupted the existing cache: $output"
    [[ "$output" == *$'\n0' ]] ||
      fail "A write failure left a temporary file behind: $output"
  }
  teardown_test_home
}

test_shell_tool_cache_final_mv_failure_cleans_up_and_preserves_existing_cache() {
  local fake_bin cache_dir output

  setup_test_home
  fake_bin="$TEST_ROOT/bin"
  cache_dir="$HOME/.cache/selfishell"
  mkdir -p "$fake_bin" "$cache_dir"
  printf '# preexisting cache\n' >"$cache_dir/cache.zsh"
  cat >"$fake_bin/mv" <<'EOF'
#!/usr/bin/env bash
exit 1
EOF
  chmod +x "$fake_bin/mv"

  output="$(
    ZDOTDIR="" PATH="$fake_bin:/usr/bin:/bin" SELFISHELL_COMMON_DIR="$ROOT_DIR/common" \
      /bin/zsh -f -c '
        _selfishell_command_path() { command -v "$1"; }
        source "$1"
        _selfishell_generate_zsh_cache "$2/cache.zsh" echo "print ok"
        cat "$2/cache.zsh"
        command find "$2" -maxdepth 1 -name "*.tmp.*" | command wc -l | command tr -d " "
      ' zsh "$ROOT_DIR/common/interactive.zsh" "$cache_dir"
  )"

  [[ "$output" == *'# preexisting cache'* ]] ||
    fail "A failed final mv corrupted the existing cache: $output"
  [[ "$output" == *$'\n0' ]] ||
    fail "A failed final mv left a temporary file behind: $output"
  teardown_test_home
}

test_fzf_cache_generation_succeeds_atomically() {
  local fake_bin cache_dir output

  setup_test_home
  fake_bin="$TEST_ROOT/bin"
  cache_dir="$HOME/.cache/selfishell"
  mkdir -p "$fake_bin" "$cache_dir"
  cat >"$fake_bin/fzf" <<'EOF'
#!/usr/bin/env bash
printf 'bindkey -M emacs "^R" fzf-history-widget\n'
EOF
  chmod +x "$fake_bin/fzf"

  output="$(
    ZDOTDIR="" PATH="$fake_bin:/usr/bin:/bin" SELFISHELL_COMMON_DIR="$ROOT_DIR/common" \
      /bin/zsh -f -c '
        _selfishell_command_path() { command -v "$1"; }
        source "$1"
        _selfishell_generate_fzf_cache "$2/cache.zsh"
        [[ -s "$2/cache.zsh" ]] && cat "$2/cache.zsh"
        command find "$2" -maxdepth 1 -name "*.tmp.*" | command wc -l | command tr -d " "
      ' zsh "$ROOT_DIR/common/interactive.zsh" "$cache_dir"
  )"

  [[ "$output" == *'fzf-history-widget'* ]] ||
    fail "A successful fzf cache generation did not write the expected content: $output"
  [[ "$output" == *$'\n0' ]] ||
    fail "A successful fzf cache generation left a temporary file behind: $output"
  teardown_test_home
}

test_fzf_cache_generation_rejects_invalid_zsh_syntax() {
  local fake_bin cache_dir output

  setup_test_home
  fake_bin="$TEST_ROOT/bin"
  cache_dir="$HOME/.cache/selfishell"
  mkdir -p "$fake_bin" "$cache_dir"
  printf '# preexisting fzf cache\n' >"$cache_dir/cache.zsh"
  cat >"$fake_bin/fzf" <<'EOF'
#!/usr/bin/env bash
printf 'if [[ not valid zsh\n'
EOF
  chmod +x "$fake_bin/fzf"

  output="$(
    ZDOTDIR="" PATH="$fake_bin:/usr/bin:/bin" SELFISHELL_COMMON_DIR="$ROOT_DIR/common" \
      /bin/zsh -f -c '
        _selfishell_command_path() { command -v "$1"; }
        source "$1"
        _selfishell_generate_fzf_cache "$2/cache.zsh"
        cat "$2/cache.zsh"
        command find "$2" -maxdepth 1 -name "*.tmp.*" | command wc -l | command tr -d " "
      ' zsh "$ROOT_DIR/common/interactive.zsh" "$cache_dir"
  )"

  [[ "$output" == *'# preexisting fzf cache'* ]] ||
    fail "Invalid zsh syntax from fzf corrupted the existing cache: $output"
  [[ "$output" == *$'\n0' ]] ||
    fail "Invalid zsh syntax from fzf left a temporary file behind: $output"
  teardown_test_home
}

test_fzf_cache_final_mv_failure_cleans_up_and_preserves_existing_cache() {
  local fake_bin cache_dir output

  setup_test_home
  fake_bin="$TEST_ROOT/bin"
  cache_dir="$HOME/.cache/selfishell"
  mkdir -p "$fake_bin" "$cache_dir"
  printf '# preexisting fzf cache\n' >"$cache_dir/cache.zsh"
  cat >"$fake_bin/fzf" <<'EOF'
#!/usr/bin/env bash
printf 'bindkey -M emacs "^R" fzf-history-widget\n'
EOF
  chmod +x "$fake_bin/fzf"
  cat >"$fake_bin/mv" <<'EOF'
#!/usr/bin/env bash
exit 1
EOF
  chmod +x "$fake_bin/mv"

  output="$(
    ZDOTDIR="" PATH="$fake_bin:/usr/bin:/bin" SELFISHELL_COMMON_DIR="$ROOT_DIR/common" \
      /bin/zsh -f -c '
        _selfishell_command_path() { command -v "$1"; }
        source "$1"
        _selfishell_generate_fzf_cache "$2/cache.zsh"
        cat "$2/cache.zsh"
        command find "$2" -maxdepth 1 -name "*.tmp.*" | command wc -l | command tr -d " "
      ' zsh "$ROOT_DIR/common/interactive.zsh" "$cache_dir"
  )"

  [[ "$output" == *'# preexisting fzf cache'* ]] ||
    fail "A failed final mv corrupted the existing fzf cache: $output"
  [[ "$output" == *$'\n0' ]] ||
    fail "A failed final mv left a temporary fzf cache file behind: $output"
  teardown_test_home
}

# _selfishell_generate_fzf_cache's fallback (cp from the system fzf docs)
# only triggers when fzf itself is unreachable, so PATH is rebuilt from
# individually-symlinked tools rather than the usual /usr/bin:/bin --
# those are the same merged directory on most Linux systems and can't be
# used to make an installed fzf disappear. Skipped outright wherever that
# hardcoded fallback path doesn't exist (e.g. CI's shell job, which never
# installs fzf), since there's nothing this test can exercise there.
test_fzf_cache_copy_fallback_success_and_failure() {
  local restricted_bin cache_dir output tool

  [[ -r /usr/share/doc/fzf/examples/key-bindings.zsh ]] || return 0

  setup_test_home
  cache_dir="$HOME/.cache/selfishell"
  restricted_bin="$TEST_ROOT/restricted-bin"
  mkdir -p "$cache_dir" "$restricted_bin"
  for tool in mkdir rm mv zsh cat find wc tr; do
    ln -sf "$(command -v "$tool")" "$restricted_bin/$tool"
  done
  ln -sf "$(command -v cp)" "$restricted_bin/cp"

  output="$(
    ZDOTDIR="" SELFISHELL_COMMON_DIR="$ROOT_DIR/common" PATH="$restricted_bin" \
      /bin/zsh -f -c '
        source "$1"
        _selfishell_generate_fzf_cache "$2/fallback-cache.zsh"
        [[ -s "$2/fallback-cache.zsh" ]] && print FALLBACK_WRITTEN
        command find "$2" -maxdepth 1 -name "*.tmp.*" | command wc -l | command tr -d " "
      ' zsh "$ROOT_DIR/common/interactive.zsh" "$cache_dir" 2>/dev/null
  )"

  [[ "$output" == *'FALLBACK_WRITTEN'* ]] ||
    fail "The system key-bindings fallback was not copied when fzf was unreachable: $output"
  [[ "$output" == *$'\n0' ]] ||
    fail "The fallback copy left a temporary file behind: $output"

  rm -f "$cache_dir/fallback-cache.zsh" "$restricted_bin/cp"
  cat >"$restricted_bin/cp" <<'EOF'
#!/usr/bin/env bash
exit 1
EOF
  chmod +x "$restricted_bin/cp"
  printf '# preexisting fallback cache\n' >"$cache_dir/fallback-cache.zsh"

  output="$(
    ZDOTDIR="" SELFISHELL_COMMON_DIR="$ROOT_DIR/common" PATH="$restricted_bin" \
      /bin/zsh -f -c '
        source "$1"
        _selfishell_generate_fzf_cache "$2/fallback-cache.zsh"
        cat "$2/fallback-cache.zsh"
        command find "$2" -maxdepth 1 -name "*.tmp.*" | command wc -l | command tr -d " "
      ' zsh "$ROOT_DIR/common/interactive.zsh" "$cache_dir" 2>/dev/null
  )"

  [[ "$output" == *'# preexisting fallback cache'* ]] ||
    fail "A failed fallback copy corrupted the existing cache: $output"
  [[ "$output" == *$'\n0' ]] ||
    fail "A failed fallback copy left a temporary file behind: $output"
  teardown_test_home
}

test_neovim_plugin_specs_delay_noncritical_plugins() {
  grep -Fqx '    ft = languages.lsp_filetypes,' "$ROOT_DIR/common/nvim/lua/plugins/lsp.lua" ||
    fail "LSP plugin was not limited to supported filetypes"
  grep -Fqx '    event = { "BufReadPre", "BufNewFile" },' "$ROOT_DIR/common/nvim/lua/plugins/editor.lua" ||
    fail "Rainbow delimiters was not loaded before the initial FileType event"
  grep -Fqx '    event = "VeryLazy",' "$ROOT_DIR/common/nvim/lua/plugins/editor.lua" ||
    fail "Which-key was not deferred to VeryLazy"
}

test_editor_aliases_stay_with_neovim() {
  local fake_bin output

  setup_test_home
  fake_bin="$TEST_ROOT/bin"
  mkdir -p "$fake_bin"
  cat >"$fake_bin/nvim" <<'EOF'
#!/usr/bin/env sh
exit 0
EOF
  chmod +x "$fake_bin/nvim"

  output="$(
    PATH="$fake_bin:/usr/bin:/bin" \
      ZDOTDIR="" \
      /bin/zsh -f -c '
        _selfishell_command_path() { command -v "$1"; }
        source "$1"
        alias vim
      ' zsh "$ROOT_DIR/common/aliases-editor.zsh"
  )"

  [[ "$output" == *'vim=nvim'* ]] || fail "vim was not redirected to Neovim"
  teardown_test_home
}

test_minimal_profile_keeps_system_vim() {
  local output

  setup_test_home
  output="$(
    PATH="$TEST_ROOT/empty-bin" \
      ZDOTDIR="" \
      /bin/zsh -f -c '
        _selfishell_command_path() { command -v "$1"; }
        source "$1"
        alias vim 2>/dev/null || true
      ' zsh "$ROOT_DIR/common/aliases-editor.zsh"
  )"

  [[ "$output" != *'nvim'* ]] || fail "Vim alias should not be forced without Neovim"
  teardown_test_home
}

test_zsh_plugin_pins_match_dependencies_conf() {
  local repository target_file expected_commit

  while read -r repository target_file; do
    expected_commit="$(awk -v repository="$repository" '$1 == "zsh-plugin" && $2 == repository { print $3 }' "$ROOT_DIR/dependencies.conf")"
    [[ "$expected_commit" =~ ^[0-9a-f]{40}$ ]] ||
      fail "No approved zsh-plugin commit recorded for $repository"
    grep -Fq "ver'$expected_commit'" "$ROOT_DIR/$target_file" ||
      fail "$target_file does not pin $repository to the dependencies.conf commit ($expected_commit)"
  done <<'PLUGINS'
zsh-users/zsh-completions common/completion.zsh
Aloxaf/fzf-tab common/interactive.zsh
zsh-users/zsh-autosuggestions common/interactive.zsh
zdharma-continuum/fast-syntax-highlighting common/interactive.zsh
PLUGINS
}

test_minimal_profile_initializes_git_completion_without_zinit
printf 'PASS: test_minimal_profile_initializes_git_completion_without_zinit\n'
test_macos_managed_zsh_adds_default_cli_prefix_to_path
printf 'PASS: test_macos_managed_zsh_adds_default_cli_prefix_to_path\n'
test_wsl_defers_windows_path_during_initialization
printf 'PASS: test_wsl_defers_windows_path_during_initialization\n'
test_mise_uses_selfishell_config_only_for_developer_profile
printf 'PASS: test_mise_uses_selfishell_config_only_for_developer_profile\n'
test_update_notice_reads_installed_version_file
printf 'PASS: test_update_notice_reads_installed_version_file\n'
test_update_notice_uses_cache_and_refreshes_in_background_format
printf 'PASS: test_update_notice_uses_cache_and_refreshes_in_background_format\n'
test_zsh_plugin_pins_match_dependencies_conf
printf 'PASS: test_zsh_plugin_pins_match_dependencies_conf\n'
test_update_notice_stale_lock_is_reclaimed_after_ttl
printf 'PASS: test_update_notice_stale_lock_is_reclaimed_after_ttl\n'
test_update_notice_fresh_lock_blocks_concurrent_refresh
printf 'PASS: test_update_notice_fresh_lock_blocks_concurrent_refresh\n'
test_update_notice_stale_empty_lock_directory_is_reclaimed
printf 'PASS: test_update_notice_stale_empty_lock_directory_is_reclaimed\n'
test_update_notice_fresh_empty_lock_directory_is_preserved
printf 'PASS: test_update_notice_fresh_empty_lock_directory_is_preserved\n'
test_update_notice_stale_lock_with_only_pid_is_reclaimed
printf 'PASS: test_update_notice_stale_lock_with_only_pid_is_reclaimed\n'
test_update_notice_corrupt_created_at_falls_back_to_directory_mtime
printf 'PASS: test_update_notice_corrupt_created_at_falls_back_to_directory_mtime\n'
test_update_notice_unreadable_created_at_falls_back_to_directory_mtime
printf 'PASS: test_update_notice_unreadable_created_at_falls_back_to_directory_mtime\n'
test_update_notice_future_created_at_is_preserved
printf 'PASS: test_update_notice_future_created_at_is_preserved\n'
test_update_notice_lock_ttl_rejects_invalid_values
printf 'PASS: test_update_notice_lock_ttl_rejects_invalid_values\n'
test_update_notice_lock_ttl_zero_does_not_mean_instantly_stale
printf 'PASS: test_update_notice_lock_ttl_zero_does_not_mean_instantly_stale\n'
test_update_notice_lock_ttl_honors_valid_custom_value
printf 'PASS: test_update_notice_lock_ttl_honors_valid_custom_value\n'
test_update_notice_refresh_removes_lock_even_when_version_lookup_fails
printf 'PASS: test_update_notice_refresh_removes_lock_even_when_version_lookup_fails\n'
test_update_lock_stale_since_preserves_lock_when_age_cannot_be_determined
printf 'PASS: test_update_lock_stale_since_preserves_lock_when_age_cannot_be_determined\n'
test_shell_tool_cache_generation_succeeds_atomically
printf 'PASS: test_shell_tool_cache_generation_succeeds_atomically\n'
test_shell_tool_cache_generation_failures_preserve_existing_cache
printf 'PASS: test_shell_tool_cache_generation_failures_preserve_existing_cache\n'
test_shell_tool_cache_regenerates_when_binary_is_newer
printf 'PASS: test_shell_tool_cache_regenerates_when_binary_is_newer\n'
test_shell_tool_cache_does_not_regenerate_when_cache_is_newer_than_binary
printf 'PASS: test_shell_tool_cache_does_not_regenerate_when_cache_is_newer_than_binary\n'
test_shell_tool_cache_write_failure_preserves_existing_cache
printf 'PASS: test_shell_tool_cache_write_failure_preserves_existing_cache\n'
test_shell_tool_cache_final_mv_failure_cleans_up_and_preserves_existing_cache
printf 'PASS: test_shell_tool_cache_final_mv_failure_cleans_up_and_preserves_existing_cache\n'
test_fzf_cache_generation_succeeds_atomically
printf 'PASS: test_fzf_cache_generation_succeeds_atomically\n'
test_fzf_cache_generation_rejects_invalid_zsh_syntax
printf 'PASS: test_fzf_cache_generation_rejects_invalid_zsh_syntax\n'
test_fzf_cache_final_mv_failure_cleans_up_and_preserves_existing_cache
printf 'PASS: test_fzf_cache_final_mv_failure_cleans_up_and_preserves_existing_cache\n'
test_fzf_cache_copy_fallback_success_and_failure
printf 'PASS: test_fzf_cache_copy_fallback_success_and_failure\n'
test_neovim_plugin_specs_delay_noncritical_plugins
printf 'PASS: test_neovim_plugin_specs_delay_noncritical_plugins\n'
test_editor_aliases_stay_with_neovim
printf 'PASS: test_editor_aliases_stay_with_neovim\n'
test_minimal_profile_keeps_system_vim
printf 'PASS: test_minimal_profile_keeps_system_vim\n'
