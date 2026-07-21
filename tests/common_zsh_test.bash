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
        command find "$2" -maxdepth 1 -name "*.tmp.*" | command wc -l
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
          command find "$2" -maxdepth 1 -name "*.tmp.*" | command wc -l
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
  touch -d '2020-01-01' "$cache_dir/zoxide-init.zsh"
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
test_shell_tool_cache_generation_succeeds_atomically
printf 'PASS: test_shell_tool_cache_generation_succeeds_atomically\n'
test_shell_tool_cache_generation_failures_preserve_existing_cache
printf 'PASS: test_shell_tool_cache_generation_failures_preserve_existing_cache\n'
test_shell_tool_cache_regenerates_when_binary_is_newer
printf 'PASS: test_shell_tool_cache_regenerates_when_binary_is_newer\n'
test_neovim_plugin_specs_delay_noncritical_plugins
printf 'PASS: test_neovim_plugin_specs_delay_noncritical_plugins\n'
test_editor_aliases_stay_with_neovim
printf 'PASS: test_editor_aliases_stay_with_neovim\n'
test_minimal_profile_keeps_system_vim
printf 'PASS: test_minimal_profile_keeps_system_vim\n'
