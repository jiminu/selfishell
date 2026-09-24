#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/tests/test_helper.bash"

test_git_completion_initializes_without_zinit() {
  setup_test_home
  local output

  output="$(
    XDG_CACHE_HOME="$HOME/.cache" \
      ZDOTDIR="" \
      PATH="/usr/bin:/bin" \
      /bin/zsh -f -c '
        source "$1"
        (( $+functions[_git] ))
        [[ -s "$HOME/.zcompdump" ]]
      ' zsh "$ROOT_DIR/config/shared/zsh/common.zsh" 2>&1
  )"

  [[ -z "$output" ]] || fail "Missing zinit should not emit stderr noise"

  teardown_test_home
}

test_shell_startup_does_not_ask_zinit_to_fetch_missing_plugins() {
  local output
  local zinit_home
  local zinit_log

  setup_test_home
  zinit_home="$HOME/.local/share/zinit/zinit.git"
  zinit_log="$TEST_ROOT/zinit-calls"
  mkdir -p "$zinit_home"
  cat >"$zinit_home/zinit.zsh" <<'EOF'
typeset -gA ZINIT
ZINIT[PLUGINS_DIR]="${XDG_DATA_HOME:-$HOME/.local/share}/zinit/plugins"
zinit() {
  print -r -- "$*" >>"$SELFISHELL_TEST_ZINIT_LOG"
}
EOF

  output="$(
    XDG_CACHE_HOME="$HOME/.cache" \
      XDG_DATA_HOME="$HOME/.local/share" \
      SELFISHELL_TEST_ZINIT_LOG="$zinit_log" \
      ZDOTDIR="" \
      PATH="/usr/bin:/bin" \
      /bin/zsh -f -c '
        source "$1"
      ' zsh "$ROOT_DIR/config/shared/zsh/common.zsh" 2>&1
  )"

  [[ -z "$output" ]] || fail "Missing plugins emitted startup noise: $output"
  if [[ -r "$zinit_log" ]] && grep -q '^light ' "$zinit_log"; then
    fail "Shell startup asked Zinit to fetch a missing plugin"
  fi
  teardown_test_home
}

test_shell_startup_loads_preprovisioned_zinit_plugins() {
  local fake_bin
  local output
  local plugins_dir
  local zinit_home
  local zinit_log

  setup_test_home
  fake_bin="$TEST_ROOT/bin"
  plugins_dir="$HOME/.local/share/zinit/plugins"
  zinit_home="$HOME/.local/share/zinit/zinit.git"
  zinit_log="$TEST_ROOT/zinit-calls"
  mkdir -p "$fake_bin" "$zinit_home" \
    "$plugins_dir/zsh-users---zsh-completions/.git" \
    "$plugins_dir/Aloxaf---fzf-tab/.git" \
    "$plugins_dir/zsh-users---zsh-autosuggestions/.git" \
    "$plugins_dir/zdharma-continuum---fast-syntax-highlighting/.git"
  cat >"$fake_bin/fzf" <<'EOF'
#!/bin/sh
printf ':\n'
EOF
  chmod +x "$fake_bin/fzf"
  cat >"$zinit_home/zinit.zsh" <<'EOF'
typeset -gA ZINIT
ZINIT[PLUGINS_DIR]="${XDG_DATA_HOME:-$HOME/.local/share}/zinit/plugins"
zinit() {
  print -r -- "$*" >>"$SELFISHELL_TEST_ZINIT_LOG"
}
EOF

  output="$(
    XDG_CACHE_HOME="$HOME/.cache" \
      XDG_DATA_HOME="$HOME/.local/share" \
      SELFISHELL_TEST_ZINIT_LOG="$zinit_log" \
      ZDOTDIR="" \
      PATH="$fake_bin:/usr/bin:/bin" \
      /bin/zsh -f -c '
        source "$1"
      ' zsh "$ROOT_DIR/config/shared/zsh/common.zsh" 2>&1
  )"

  [[ -z "$output" ]] || fail "Provisioned plugins emitted startup noise: $output"
  [[ "$(grep -c '^light ' "$zinit_log")" -eq 4 ]] ||
    fail "Shell startup did not load all four provisioned plugins"
  teardown_test_home
}

# Skip incomplete checkouts at startup; install/update repairs them separately.
test_shell_startup_skips_an_incomplete_zinit_plugin_checkout() {
  local output
  local plugins_dir
  local zinit_home
  local zinit_log

  setup_test_home
  plugins_dir="$HOME/.local/share/zinit/plugins"
  zinit_home="$HOME/.local/share/zinit/zinit.git"
  zinit_log="$TEST_ROOT/zinit-calls"
  mkdir -p "$zinit_home" \
    "$plugins_dir/zsh-users---zsh-completions" \
    "$plugins_dir/zsh-users---zsh-autosuggestions" \
    "$plugins_dir/zdharma-continuum---fast-syntax-highlighting"
  cat >"$zinit_home/zinit.zsh" <<'EOF'
typeset -gA ZINIT
ZINIT[PLUGINS_DIR]="${XDG_DATA_HOME:-$HOME/.local/share}/zinit/plugins"
zinit() {
  print -r -- "$*" >>"$SELFISHELL_TEST_ZINIT_LOG"
}
EOF

  output="$(
    XDG_CACHE_HOME="$HOME/.cache" \
      XDG_DATA_HOME="$HOME/.local/share" \
      SELFISHELL_TEST_ZINIT_LOG="$zinit_log" \
      ZDOTDIR="" \
      PATH="/usr/bin:/bin" \
      /bin/zsh -f -c '
        source "$1"
      ' zsh "$ROOT_DIR/config/shared/zsh/common.zsh" 2>&1
  )"

  [[ -z "$output" ]] || fail "Incomplete plugin checkouts emitted startup noise: $output"
  if [[ -r "$zinit_log" ]] && grep -q '^light ' "$zinit_log"; then
    fail "Shell startup loaded a plugin directory without a repository"
  fi
  teardown_test_home
}

setup_foreign_completion_audit_marker() {
  rm -rf "$HOME/.zcompdump.audit"
  case "$1" in
    file) printf 'personal data\n' >"$HOME/.zcompdump.audit" ;;
    symlink | empty-symlink)
      printf 'personal data\n' >"$TEST_ROOT/marker-target"
      [[ "$1" != empty-symlink ]] || : >"$TEST_ROOT/marker-target"
      ln -s "$TEST_ROOT/marker-target" "$HOME/.zcompdump.audit"
      ;;
    dangling) ln -s "$TEST_ROOT/missing-target" "$HOME/.zcompdump.audit" ;;
    directory)
      mkdir "$HOME/.zcompdump.audit"
      printf 'personal data\n' >"$HOME/.zcompdump.audit/personal"
      ;;
  esac
  touch -t 202001010000 "$TEST_ROOT/marker-reference"
  [[ "$1" == dangling ]] || touch -r "$TEST_ROOT/marker-reference" "$HOME/.zcompdump.audit"
}

assert_foreign_completion_audit_marker_preserved() {
  case "$1" in
    file) assert_file_content 'personal data' "$HOME/.zcompdump.audit" ;;
    symlink | empty-symlink)
      assert_symlink_to "$TEST_ROOT/marker-target" "$HOME/.zcompdump.audit"
      if [[ "$1" == symlink ]]; then
        assert_file_content 'personal data' "$TEST_ROOT/marker-target"
      else
        assert_file_content '' "$TEST_ROOT/marker-target"
      fi
      ;;
    dangling)
      assert_symlink_to "$TEST_ROOT/missing-target" "$HOME/.zcompdump.audit"
      [[ ! -e "$TEST_ROOT/missing-target" ]] || fail "Startup created a dangling marker target"
      ;;
    directory) assert_file_content 'personal data' "$HOME/.zcompdump.audit/personal" ;;
  esac
  [[ "$1" == dangling || ! "$HOME/.zcompdump.audit" -nt "$TEST_ROOT/marker-reference" ]] ||
    fail "Startup changed the timestamp of a foreign $1 audit marker"
}

# Check the presence of audit work, without depending on timings or call counts.
test_completion_audits_the_dump_once_a_day() {
  local audits_when_fresh audits_when_stale audits_after_refresh marker_type

  setup_test_home
  mkdir -p "$HOME/completion-functions" "$HOME/.local/share" "$HOME/bin"
  ln -s /bin/mv "$HOME/bin/mv"
  ln -s /bin/rm "$HOME/bin/rm"
  ln -s /usr/bin/touch "$HOME/bin/touch"
  # Copy the actual Zsh functions into a secure fixture directory: an insecure
  # host site-functions directory must not turn the clean-cache test into a
  # test of the host permissions.
  /bin/zsh -f -c '
    for name in compinit compaudit compdump compinstall _git; do
      files=(${^fpath}/$name(N))
      command cp "$files[1]" "$1/$name" || exit 1
    done
  ' zsh "$HOME/completion-functions"
  chmod 0700 "$HOME/completion-functions"

  count_startup_audits() {
    XDG_CACHE_HOME="$HOME/.cache" \
      XDG_DATA_HOME="$HOME/.local/share" \
      ZDOTDIR="$HOME" \
      PATH="$HOME/bin" \
      /bin/zsh -f -i -c '
        fpath=("$HOME/completion-functions")
        _compdir=""
        _selfishell_command_path() { return 1; }
        zmodload zsh/zprof
        source "$1"
        zprof
      ' zsh "$ROOT_DIR/config/shared/zsh/completion.zsh" 2>/dev/null | grep -c compaudit || true
  }

  count_startup_audits >/dev/null
  audits_when_fresh="$(count_startup_audits)"
  [[ "$audits_when_fresh" -eq 0 ]] ||
    fail "A fresh completion dump was audited again on startup"

  touch -t 202001010000 "$HOME/.zcompdump" "$HOME/.zcompdump.audit"
  audits_when_stale="$(count_startup_audits)"
  [[ "$audits_when_stale" -gt 0 ]] ||
    fail "A day-old completion dump was not re-audited"
  audits_after_refresh="$(count_startup_audits)"
  [[ "$audits_after_refresh" -eq 0 ]] ||
    fail "A completed daily audit was repeated on the next startup"

  # A newly installed tool's completion must not wait for the next daily audit.
  # Backdate the dump so the new file's directory is newer at 1 s resolution.
  touch -t 202001010000 "$HOME/.zcompdump" "$HOME/.zcompdump.zwc"
  printf '#compdef zzselfishell\n' >"$HOME/completion-functions/_zzselfishell"
  [[ "$(count_startup_audits)" -gt 0 ]] || fail "A new completion file did not rebuild the dump"
  grep -q '_zzselfishell' "$HOME/.zcompdump" || fail "The rebuilt dump omitted the new completion"
  [[ "$(count_startup_audits)" -eq 0 ]] || fail "An unchanged completion set was rebuilt again"

  for marker_type in file symlink empty-symlink dangling directory; do
    setup_foreign_completion_audit_marker "$marker_type"
    rm -f "$HOME/.zcompdump"
    count_startup_audits >/dev/null
    assert_foreign_completion_audit_marker_preserved "$marker_type"
    [[ "$(count_startup_audits)" -gt 0 ]] ||
      fail "A foreign $marker_type marker bypassed the completion audit"
    assert_foreign_completion_audit_marker_preserved "$marker_type"
  done
  teardown_test_home
}

test_insecure_completion_directory_does_not_block_startup() {
  local output completion_dir scenario

  for scenario in missing noninteractive removed compile-failure expired foreign-file foreign-symlink foreign-empty-symlink foreign-dangling foreign-directory; do
    setup_test_home
    completion_dir="$TEST_ROOT/insecure-completions"
    mkdir -p "$completion_dir" "$HOME/.local/share"
    chmod 0777 "$completion_dir"
    printf '#compdef selfishell-insecure-probe\n' >"$completion_dir/_selfishell_insecure_probe"
    mkdir "$TEST_ROOT/secure-completions"
    printf '#compdef selfishell-safe-probe\nprint SAFE_COMPLETION\n' >"$TEST_ROOT/secure-completions/_selfishell_safe_probe"
    printf '#compdef selfishell-safe-probe\nprint INSECURE_LOADED\n' >"$completion_dir/_selfishell_safe_probe"
    case "$scenario" in
      foreign-*) setup_foreign_completion_audit_marker "${scenario#foreign-}" ;;
      noninteractive)
        run_completion_startup_probe "$completion_dir" +i >/dev/null
        # An unchanged file count must not validate an unaudited dump.
        touch "$TEST_ROOT/secure-completions/_added_one" "$TEST_ROOT/secure-completions/_added_two"
        ;;
      removed | compile-failure)
        run_completion_startup_probe "$completion_dir" >/dev/null
        # A previous clean audit must not survive a new insecure audit.
        touch "$HOME/.zcompdump.audit"
        rm "$HOME/.zcompdump"
        ;;
      expired)
        run_completion_startup_probe "$completion_dir" >/dev/null
        touch -t 202001010000 "$HOME/.zcompdump.audit"
        ;;
    esac

    output="$(SELFISHELL_TEST_FAIL_COMPILE="$scenario" run_completion_startup_probe "$completion_dir")"
    [[ "$output" == *STARTUP_COMPLETE* ]] ||
      fail "Startup blocked ($scenario): $output"
    [[ "$output" == *SAFE_COMPLETION* && "$output" != *INSECURE_REGISTERED* &&
      "$output" != *INSECURE_LOADED* ]] ||
      fail "Startup did not restrict completion to the secure directory ($scenario): $output"
    [[ "$output" == *'insecure completion directories detected'* ]] ||
      fail "Startup did not warn about the insecure directory ($scenario): $output"
    output="$(run_completion_startup_probe "$completion_dir")"
    [[ "$output" == *STARTUP_COMPLETE* && "$output" == *SAFE_COMPLETION* &&
      "$output" != *INSECURE_REGISTERED* && "$output" != *INSECURE_LOADED* ]] ||
      fail "Cached startup can autoload from an insecure directory ($scenario): $output"
    [[ "$scenario" != foreign-* ]] || assert_foreign_completion_audit_marker_preserved "${scenario#foreign-}"
    teardown_test_home
  done
}

# Inject an insecure fpath entry to check exclusion without blocking startup.
run_completion_startup_probe() {
  local completion_dir="${1:-}"
  local shell_mode="${2:--i}"
  XDG_CACHE_HOME="$HOME/.cache" \
    XDG_DATA_HOME="$HOME/.local/share" \
    ZDOTDIR="$HOME" \
    PATH="/usr/bin:/bin" \
    SELFISHELL_TEST_COMPLETION_DIR="$completion_dir" \
    /bin/zsh -f "$shell_mode" -c '
      [[ -z "$SELFISHELL_TEST_COMPLETION_DIR" ]] || fpath=("$SELFISHELL_TEST_COMPLETION_DIR" "${SELFISHELL_TEST_COMPLETION_DIR:h}/secure-completions" $fpath)
      if [[ "$SELFISHELL_TEST_FAIL_COMPILE" == compile-failure ]]; then
        zcompile() { return 1; }
      fi
      source "$1"
      (( ! ${+_comps[selfishell-insecure-probe]} )) || print INSECURE_REGISTERED
      (( ! ${+_comps[selfishell-safe-probe]} )) || _selfishell_safe_probe
      print STARTUP_COMPLETE
    ' zsh "$ROOT_DIR/config/shared/zsh/common.zsh" 2>&1
}

test_secure_completion_directory_does_not_add_warning() {
  local with_dir without_dir with_warned without_warned
  local completion_dir

  setup_test_home
  completion_dir="$TEST_ROOT/secure-completions"
  mkdir -p "$completion_dir" "$HOME/.local/share"
  chmod 0755 "$completion_dir"
  touch -t 202001010000 "$HOME/.zcompdump" "$HOME/.zcompdump.audit"
  with_dir="$(run_completion_startup_probe "$completion_dir")"
  [[ "$with_dir" == *STARTUP_COMPLETE* ]] ||
    fail "Shell startup did not complete with a secure completion directory: $with_dir"

  # Compare with baseline startup so host fpath permissions cannot cause a false failure.
  touch -t 202001010000 "$HOME/.zcompdump" "$HOME/.zcompdump.audit"
  without_dir="$(run_completion_startup_probe)"
  [[ "$without_dir" == *STARTUP_COMPLETE* ]] ||
    fail "Shell startup did not complete without a completion directory: $without_dir"

  with_warned=0
  [[ "$with_dir" != *'insecure completion directories detected'* ]] || with_warned=1
  without_warned=0
  [[ "$without_dir" != *'insecure completion directories detected'* ]] || without_warned=1
  [[ "$with_warned" == "$without_warned" ]] ||
    fail "Adding our own secure completion directory changed the insecure-directory warning (with=$with_dir without=$without_dir)"
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
    ' zsh "$ROOT_DIR/config/macos/zshrc"

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
      ' zsh "$ROOT_DIR/config/ubuntu/zshrc"
  )"

  [[ "$output" == "$HOME/.local/bin:$HOME/.rd/bin:/usr/bin:/bin:/mnt/c/Windows" ]] ||
    fail "WSL PATH was not restored after initialization: $output"
  teardown_test_home
}

test_non_wsl_command_lookup_uses_native_path_semantics() {
  local output
  local path_dir
  local relative_dir
  local work_dir

  setup_test_home
  work_dir="$TEST_ROOT/work"
  path_dir="$TEST_ROOT/bin"
  relative_dir="$TEST_ROOT/relative-bin"
  mkdir -p "$work_dir" "$path_dir" "$relative_dir"
  printf '#!/bin/sh\n' >"$work_dir/cwd-probe"
  printf '#!/bin/sh\n' >"$path_dir/path-probe"
  printf '#!/bin/sh\n' >"$relative_dir/relative-probe"
  chmod +x "$work_dir/cwd-probe" "$path_dir/path-probe" "$relative_dir/relative-probe"

  output="$(
    WSL_DISTRO_NAME="" \
      XDG_CACHE_HOME="$HOME/.cache" \
      ZDOTDIR="" \
      PATH="/usr/bin:/bin" \
      /bin/zsh -f -c '
        source "$1"
        cd "$2"
        path=("" ../relative-bin "$3" /usr/bin /bin)
        print -r -- "current=$(_selfishell_command_path cwd-probe)"
        print -r -- "relative=$(_selfishell_command_path relative-probe)"
        print -r -- "path=$(_selfishell_command_path path-probe)"
        if _selfishell_command_path missing-probe >/dev/null; then
          print -r -- "missing=found"
        else
          print -r -- "missing=absent"
        fi
      ' zsh "$ROOT_DIR/config/shared/zsh/common.zsh" "$work_dir" "$path_dir"
  )"

  [[ "$output" == "current=cwd-probe
relative=../relative-bin/relative-probe
path=$path_dir/path-probe
missing=absent" ]] || fail "Native command lookup did not preserve PATH semantics: $output"
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
        source "$1"
        _selfishell_current_version
      ' zsh "$ROOT_DIR/config/shared/zsh/common.zsh"
  )"

  [[ "$output" == 1.2.3 ]] || fail "Update notice did not read the installed VERSION file"
  teardown_test_home
}

test_update_notice_defers_current_version_lookup_until_available_version_exists() {
  local cache_dir current_calls fake_bin output refresh_calls

  setup_test_home
  fake_bin="$TEST_ROOT/bin"
  cache_dir="$HOME/.cache/selfishell"
  current_calls="$TEST_ROOT/current-calls"
  refresh_calls="$TEST_ROOT/refresh-calls"
  mkdir -p "$fake_bin" "$cache_dir"
  printf '#!/usr/bin/env sh\nexit 0\n' >"$fake_bin/selfishell"
  chmod +x "$fake_bin/selfishell"

  output="$(
    XDG_CACHE_HOME="$HOME/.cache" \
      ZDOTDIR="" \
      PATH="$fake_bin:/usr/bin:/bin" \
      /bin/zsh -f -c '
        cache_dir="$2"
        current_calls="$3"
        refresh_calls="$4"
        _selfishell_command_path() { command -v "$1"; }
        source "$1"
        _selfishell_current_version() {
          print -r -- called >>"$current_calls"
          print -r -- 1.0.0
        }
        _selfishell_update_notice_refresh() {
          print -r -- scheduled >>"$refresh_calls"
        }

        SELFISHELL_UPDATE_CHECK_INTERVAL=0 _selfishell_update_notice
        for attempt in {1..40}; do
          [[ -r "$refresh_calls" ]] && break
          command sleep 0.05
        done
        [[ ! -e "$current_calls" ]] || exit 1
        [[ -r "$refresh_calls" ]] || exit 1

        : >"$cache_dir/available-version"
        SELFISHELL_UPDATE_CHECK_INTERVAL=9999999999 _selfishell_update_notice
        [[ ! -e "$current_calls" ]] || exit 1

        print -r -- 1.1.0 >"$cache_dir/available-version"
        notice="$(SELFISHELL_UPDATE_CHECK_INTERVAL=9999999999 _selfishell_update_notice 2>&1)"
        [[ "$notice" == "[Selfishell] 1.1.0 is available. Run: selfishell update" ]] || exit 1
        [[ "$(wc -l <"$current_calls")" -eq 1 ]] || exit 1

        print -r -- 1.0.0 >"$cache_dir/available-version"
        SELFISHELL_UPDATE_CHECK_INTERVAL=9999999999 _selfishell_update_notice
        [[ ! -e "$cache_dir/available-version" ]] || exit 1
        [[ "$(wc -l <"$current_calls")" -eq 2 ]] || exit 1
      ' zsh "$ROOT_DIR/config/shared/zsh/update-notice.zsh" "$cache_dir" "$current_calls" "$refresh_calls"
  )"

  [[ -z "$output" ]] || fail "Update notice lazy version lookup test emitted output: $output"
  teardown_test_home
}

setup_update_notice_cli() {
  fake_bin="$TEST_ROOT/bin"
  cache_dir="$HOME/.cache/selfishell"
  mkdir -p "$fake_bin" "$cache_dir"
  cat >"$fake_bin/selfishell" <<'EOF'
#!/bin/sh
[ "$1" = version ] || exit 1
if [ "${2:-}" = --available ]; then
  printf '1.1.0\n'
else
  printf 'selfishell 0.2.0\n'
fi
EOF
  chmod +x "$fake_bin/selfishell"
}

test_update_notice_compares_semantic_versions() {
  setup_test_home
  /bin/zsh -f -c '
    source "$1"
    while read -r candidate current expected; do
      actual=0
      _selfishell_version_is_newer "$candidate" "$current" && actual=1
      [[ "$actual" == "$expected" ]] || {
        print -u2 -- "Wrong version comparison: $candidate > $current (expected $expected, got $actual)"
        exit 1
      }
    done
  ' zsh "$ROOT_DIR/config/shared/zsh/update-notice.zsh" <"$ROOT_DIR/tests/fixtures/version-precedence.txt" ||
    fail "Semantic version comparison failed"
  teardown_test_home
}

test_update_notice_uses_cache_and_refreshes_in_background() {
  local fake_bin cache_dir output

  setup_test_home
  setup_update_notice_cli
  printf '1.1.0\n' >"$cache_dir/available-version"
  date +%s >"$cache_dir/update-checked-at"

  output="$(
    XDG_CACHE_HOME="$HOME/.cache" ZDOTDIR="" PATH="$fake_bin:/usr/bin:/bin" \
      /bin/zsh -f -c '
        _selfishell_command_path() { command -v "$1"; }
        source "$1"
        _selfishell_update_notice
        [[ -z "$(SELFISHELL_UPDATE_NOTICE=0 _selfishell_update_notice 2>&1)" ]] || exit 1
        command rm -f "$2/available-version" "$2/update-checked-at"
        _selfishell_update_notice_refresh "$2" 12345
        [[ "$(<"$2/available-version")" == 1.1.0 ]] || exit 1
        [[ "$(<"$2/update-checked-at")" == 12345 ]] || exit 1
        command rm -f "$2/available-version" "$2/update-checked-at"
        SELFISHELL_UPDATE_CHECK_INTERVAL=0 _selfishell_update_notice
        for attempt in {1..40}; do
          [[ -r "$2/available-version" && -r "$2/update-checked-at" && ! -e "$2/update-check.lock" ]] && break
          command sleep 0.05
        done
        [[ -r "$2/available-version" && "$(<"$2/available-version")" == 1.1.0 ]] || exit 1
        [[ -s "$2/update-checked-at" && ! -e "$2/update-check.lock" ]] || exit 1
      ' zsh "$ROOT_DIR/config/shared/zsh/update-notice.zsh" "$cache_dir" 2>&1 >"$TEST_ROOT/notice-stdout"
  )" || fail "Update notice cache or background refresh failed"

  # stderr, so `zsh -i -c` output captured by a script stays clean.
  [[ "$output" == *'1.1.0'* && "$output" == *'selfishell update'* ]] ||
    fail "Update notice did not offer the cached version on stderr: $output"
  [[ ! -s "$TEST_ROOT/notice-stdout" ]] || fail "Update notice wrote to stdout: $(<"$TEST_ROOT/notice-stdout")"
  teardown_test_home
}

# Metadata wins when valid; interrupted/older writers fall back to directory
# age. Each case checks both lock ownership and whether a refresh occurred.
test_update_notice_lock_recovery() {
  local fake_bin cache_dir metadata age ttl expected now

  setup_test_home
  setup_update_notice_cli
  now="$(date +%s)"
  while read -r metadata age ttl expected; do
    [[ "$metadata" != unreadable || "$(id -u)" != 0 ]] || continue
    rm -rf "$cache_dir/update-check.lock" "$cache_dir/available-version" "$cache_dir/update-checked-at"
    mkdir "$cache_dir/update-check.lock"
    case "$metadata" in
      timestamp) printf '%s\n' "$((now + age))" >"$cache_dir/update-check.lock/created_at" ;;
      pid) printf '99999\n' >"$cache_dir/update-check.lock/pid" ;;
      corrupt) printf 'not-a-timestamp\n' >"$cache_dir/update-check.lock/created_at" ;;
      zero) printf '0\n' >"$cache_dir/update-check.lock/created_at" ;;
      unreadable)
        printf '%s\n' "$now" >"$cache_dir/update-check.lock/created_at"
        chmod 000 "$cache_dir/update-check.lock/created_at"
        ;;
    esac
    if [[ "$metadata" != timestamp && "$age" == stale ]]; then
      touch -t 202001010000 "$cache_dir/update-check.lock"
    fi
    [[ "$ttl" != empty ]] || ttl=''

    SELFISHELL_UPDATE_LOCK_TTL="$ttl" PATH="$fake_bin:/usr/bin:/bin" \
      /bin/zsh -f -c '
        source "$1"
        _selfishell_update_notice_refresh "$2" 12345 || :
        if [[ "$3" == refreshed ]]; then
          [[ ! -e "$2/update-check.lock" ]] || exit 1
          [[ -r "$2/available-version" && "$(<"$2/available-version")" == 1.1.0 ]] || exit 1
          [[ -r "$2/update-checked-at" && "$(<"$2/update-checked-at")" == 12345 ]] || exit 1
        else
          [[ -d "$2/update-check.lock" && ! -e "$2/available-version" && ! -e "$2/update-checked-at" ]] || exit 1
        fi
      ' zsh "$ROOT_DIR/config/shared/zsh/update-notice.zsh" "$cache_dir" "$expected" ||
      fail "Lock recovery: metadata=$metadata age=$age ttl=$ttl expected=$expected"
  done <<'LOCKS'
timestamp -700 600 refreshed
timestamp 0 600 held
absent stale 600 refreshed
absent fresh 600 held
pid stale 600 refreshed
corrupt stale 600 refreshed
corrupt fresh 600 held
zero stale 600 refreshed
zero fresh 600 held
unreadable stale 600 refreshed
timestamp 100000 600 held
timestamp -700 abc refreshed
timestamp -700 -100 refreshed
timestamp -700 1.5 refreshed
timestamp -700 0 refreshed
timestamp -700 empty refreshed
timestamp -2 0 held
timestamp -5 2 refreshed
LOCKS
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
      ' zsh "$ROOT_DIR/config/shared/zsh/update-notice.zsh" "$cache_dir"
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
    ' zsh "$ROOT_DIR/config/shared/zsh/update-notice.zsh" "$TEST_ROOT/no-such-lock-dir"
  )"

  [[ "$output" == 'PRESERVED' ]] ||
    fail "A lock whose age cannot be determined at all should be left alone: $output"
  teardown_test_home
}

test_update_notice_refresh_cleans_up_temp_files_on_write_failure() {
  local cache_dir output

  setup_test_home
  cache_dir="$HOME/.cache/selfishell"
  mkdir -p "$cache_dir"
  chmod 555 "$cache_dir"

  output="$(
    PATH="/usr/bin:/bin" \
      /bin/zsh -f -c '
        source "$1"
        _selfishell_update_notice_refresh "$2" 12345
        command find "$2" -maxdepth 1 -name "*.tmp.*" 2>/dev/null | command wc -l | command tr -d " "
      ' zsh "$ROOT_DIR/config/shared/zsh/update-notice.zsh" "$cache_dir" 2>/dev/null
  )"
  chmod 755 "$cache_dir"

  [[ "$(id -u)" == 0 ]] || [[ "$output" == 0 ]] ||
    fail "A write failure left a temporary available-version/checked-at file behind: $output"
  teardown_test_home
}

test_update_notice_refresh_cleans_up_temp_files_on_mv_failure() {
  local fake_bin cache_dir output

  setup_test_home
  fake_bin="$TEST_ROOT/bin"
  cache_dir="$HOME/.cache/selfishell"
  mkdir -p "$fake_bin" "$cache_dir"
  printf '#!/usr/bin/env bash\nprintf "2.0.0\\n"\n' >"$fake_bin/selfishell"
  chmod +x "$fake_bin/selfishell"
  cat >"$fake_bin/mv" <<'EOF'
#!/usr/bin/env bash
exit 1
EOF
  chmod +x "$fake_bin/mv"

  output="$(
    PATH="$fake_bin:/usr/bin:/bin" \
      /bin/zsh -f -c '
        source "$1"
        _selfishell_update_notice_refresh "$2" 12345
        command find "$2" -maxdepth 1 -name "*.tmp.*" | command wc -l | command tr -d " "
      ' zsh "$ROOT_DIR/config/shared/zsh/update-notice.zsh" "$cache_dir"
  )"

  [[ "$output" == 0 ]] ||
    fail "A failed final mv left a temporary available-version/checked-at file behind: $output"
  teardown_test_home
}

test_shell_tool_cache_generation_succeeds_atomically() {
  local cache_dir output

  setup_test_home
  cache_dir="$HOME/.cache/selfishell"
  mkdir -p "$cache_dir"

  output="$(
    ZDOTDIR="" PATH="/usr/bin:/bin" SELFISHELL_COMMON_DIR="$ROOT_DIR/config/shared/zsh" \
      /bin/zsh -f -c '
        _selfishell_command_path() { command -v "$1"; }
        source "$1"
        _selfishell_generate_zsh_cache "$2/cache.zsh" echo "print ok"
        [[ -s "$2/cache.zsh" ]] && cat "$2/cache.zsh"
        command find "$2" -maxdepth 1 -name "*.tmp.*" | command wc -l | command tr -d " "
      ' zsh "$ROOT_DIR/config/shared/zsh/interactive.zsh" "$cache_dir"
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
      ZDOTDIR="" PATH="/usr/bin:/bin" SELFISHELL_COMMON_DIR="$ROOT_DIR/config/shared/zsh" \
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
        ' zsh "$ROOT_DIR/config/shared/zsh/interactive.zsh" "$cache_dir" "$label"
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
    ZDOTDIR="" PATH="$fake_bin:/usr/bin:/bin" SELFISHELL_COMMON_DIR="$ROOT_DIR/config/shared/zsh" \
      XDG_CONFIG_HOME="$HOME/.config" XDG_CACHE_HOME="$HOME/.cache" \
      /bin/zsh -f -c '_selfishell_command_path() { command -v "$1"; }; source "$1"' \
      zsh "$ROOT_DIR/config/shared/zsh/interactive.zsh" 2>/dev/null
    cat "$cache_dir/zoxide-init.zsh"
  )"

  [[ "$output" == *'regenerated'* ]] ||
    fail "Cache was not regenerated when the tool binary is newer than the cache: $output"
  teardown_test_home
}

# Re-sourcing ~/.zshrc must not re-run starship's init: its keymap wrapper
# then calls itself until zsh's nesting limit.
test_starship_init_runs_once_per_shell() {
  local fake_bin output

  setup_test_home
  fake_bin="$TEST_ROOT/bin"
  mkdir -p "$fake_bin"
  cat >"$fake_bin/starship" <<'EOF'
#!/usr/bin/env bash
printf 'prompt_starship_precmd() { :; }\n(( ++starship_inits ))\n'
EOF
  chmod +x "$fake_bin/starship"

  output="$(
    ZDOTDIR="" PATH="$fake_bin:/usr/bin:/bin" SELFISHELL_COMMON_DIR="$ROOT_DIR/config/shared/zsh" \
      XDG_CONFIG_HOME="$HOME/.config" XDG_CACHE_HOME="$HOME/.cache" \
      /bin/zsh -f -c '_selfishell_command_path() { command -v "$1"; }; typeset -gi starship_inits=0
        source "$1"; source "$1"; print -r -- "inits=$starship_inits"' \
      zsh "$ROOT_DIR/config/shared/zsh/interactive.zsh" 2>/dev/null
  )"

  [[ "$output" == *'inits=1'* ]] || fail "Starship init ran again when the configuration was sourced twice: $output"
  teardown_test_home
}

# Suggestions bind once, after syntax highlighting wraps the widgets; binding
# before every prompt cost 6-7 ms, and before the first command none showed.
test_autosuggestions_bind_once_after_syntax_highlighting() {
  local output

  setup_test_home
  output="$(
    ZDOTDIR="" PATH="/usr/bin:/bin" SELFISHELL_COMMON_DIR="$ROOT_DIR/config/shared/zsh" \
      XDG_CONFIG_HOME="$HOME/.config" XDG_CACHE_HOME="$HOME/.cache" \
      /bin/zsh -f -c '
        _selfishell_command_path() { command -v "$1"; }
        _selfishell_zinit_plugin_ready() { return 0; }
        zinit() { [[ "$1" == ice ]] && print -r -- "ice: ${(j: :)@[2,-1]}"; [[ "$1" == light ]] && print -r -- "light: $2"; }
        source "$1"
        print -r -- "manual=${+ZSH_AUTOSUGGEST_MANUAL_REBIND}"
      ' zsh "$ROOT_DIR/config/shared/zsh/interactive.zsh" 2>/dev/null
  )"

  [[ "$output" == *'manual=1'* ]] || fail "Autosuggestions still rebind widgets before every prompt: $output"
  [[ "$output" == *'ver4672ad5dd9ad68a7effc1476d65afb7c584ce2b3 atload'*'|| _zsh_autosuggest_bind_widgets'* ]] ||
    fail "Syntax highlighting does not bind suggestions after loading: $output"
  [[ "$output" == *$'light: zsh-users/zsh-autosuggestions\n'*'light: zdharma-continuum/fast-syntax-highlighting'* ]] ||
    fail "Syntax highlighting no longer loads after autosuggestions: $output"
  teardown_test_home
}

# Timestamps alone miss package rollbacks and replacements that preserve mtime.
# Exercise the real cache reader/writer and count generator executions, not time.
test_shell_tool_cache_reuses_unchanged_tools_and_refreshes_replaced_tools() {
  local fake_bin tool replacement output expected_args

  for tool in fzf zoxide starship; do
    for replacement in preserved-mtime older-mtime symlink; do
      setup_test_home
      fake_bin="$TEST_ROOT/bin"
      mkdir -p "$fake_bin"
      expected_args='init zsh'
      [[ "$tool" != fzf ]] || expected_args='--zsh'
      cat >"$fake_bin/$tool" <<'EOF'
#!/bin/sh
[ "$*" = "$SELFISHELL_TEST_INIT_ARGS" ] || exit 1
printf 'called\n' >>"$HOME/generations"
printf 'print old\n'
EOF
      chmod +x "$fake_bin/$tool"
      touch -t 202101010000 "$fake_bin/$tool"

      run_tool_cache_startup() {
        ZDOTDIR="" PATH="$fake_bin:/usr/bin:/bin" SELFISHELL_COMMON_DIR="$ROOT_DIR/config/shared/zsh" \
          XDG_CONFIG_HOME="$HOME/.config" XDG_CACHE_HOME="$HOME/.cache" \
          SELFISHELL_TEST_INIT_ARGS="$expected_args" \
          /bin/zsh -f -c '_selfishell_command_path() { command -v "$1"; }; source "$1"' \
          zsh "$ROOT_DIR/config/shared/zsh/interactive.zsh"
      }

      output="$(run_tool_cache_startup)"
      [[ "$output" == old ]] || fail "$tool did not source its generated initialization: $output"
      output="$(run_tool_cache_startup)"
      [[ "$output" == old && "$(wc -l <"$HOME/generations")" -eq 1 ]] ||
        fail "$tool regenerated unchanged initialization ($replacement): $output"

      sed 's/print old/print new/' "$fake_bin/$tool" >"$fake_bin/replacement"
      chmod +x "$fake_bin/replacement"
      touch -r "$fake_bin/$tool" "$fake_bin/replacement"
      if [[ "$replacement" == older-mtime ]]; then
        touch -t 202001010000 "$fake_bin/replacement"
      fi
      if [[ "$replacement" == symlink ]]; then
        rm "$fake_bin/$tool"
        ln -s "$fake_bin/replacement" "$fake_bin/$tool"
      else
        mv "$fake_bin/replacement" "$fake_bin/$tool"
      fi

      output="$(run_tool_cache_startup)"
      [[ "$output" == new && "$(wc -l <"$HOME/generations")" -eq 2 ]] ||
        fail "$tool did not regenerate after $replacement replacement: $output"
      output="$(run_tool_cache_startup)"
      [[ "$output" == new && "$(wc -l <"$HOME/generations")" -eq 2 ]] ||
        fail "$tool regenerated unchanged replacement: $output"
      teardown_test_home
    done
  done
}

test_shell_tool_cache_write_failure_preserves_existing_cache() {
  local cache_dir output

  setup_test_home
  cache_dir="$HOME/.cache/selfishell"
  mkdir -p "$cache_dir"
  printf '# preexisting cache\n' >"$cache_dir/cache.zsh"
  chmod 555 "$cache_dir"

  output="$(
    ZDOTDIR="" PATH="/usr/bin:/bin" SELFISHELL_COMMON_DIR="$ROOT_DIR/config/shared/zsh" \
      /bin/zsh -f -c '
        _selfishell_command_path() { command -v "$1"; }
        source "$1"
        _selfishell_generate_zsh_cache "$2/cache.zsh" echo "print ok"
        cat "$2/cache.zsh"
        command find "$2" -maxdepth 1 -name "*.tmp.*" 2>/dev/null | command wc -l | command tr -d " "
      ' zsh "$ROOT_DIR/config/shared/zsh/interactive.zsh" "$cache_dir" 2>/dev/null
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
    ZDOTDIR="" PATH="$fake_bin:/usr/bin:/bin" SELFISHELL_COMMON_DIR="$ROOT_DIR/config/shared/zsh" \
      /bin/zsh -f -c '
        _selfishell_command_path() { command -v "$1"; }
        source "$1"
        _selfishell_generate_zsh_cache "$2/cache.zsh" echo "print ok"
        cat "$2/cache.zsh"
        command find "$2" -maxdepth 1 -name "*.tmp.*" | command wc -l | command tr -d " "
      ' zsh "$ROOT_DIR/config/shared/zsh/interactive.zsh" "$cache_dir"
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
    ZDOTDIR="" PATH="$fake_bin:/usr/bin:/bin" SELFISHELL_COMMON_DIR="$ROOT_DIR/config/shared/zsh" \
      /bin/zsh -f -c '
        _selfishell_command_path() { command -v "$1"; }
        source "$1"
        _selfishell_generate_fzf_cache "$2/cache.zsh"
        [[ -s "$2/cache.zsh" ]] && cat "$2/cache.zsh"
        command find "$2" -maxdepth 1 -name "*.tmp.*" | command wc -l | command tr -d " "
      ' zsh "$ROOT_DIR/config/shared/zsh/interactive.zsh" "$cache_dir"
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
    ZDOTDIR="" PATH="$fake_bin:/usr/bin:/bin" SELFISHELL_COMMON_DIR="$ROOT_DIR/config/shared/zsh" \
      /bin/zsh -f -c '
        _selfishell_command_path() { command -v "$1"; }
        source "$1"
        _selfishell_generate_fzf_cache "$2/cache.zsh"
        cat "$2/cache.zsh"
        command find "$2" -maxdepth 1 -name "*.tmp.*" | command wc -l | command tr -d " "
      ' zsh "$ROOT_DIR/config/shared/zsh/interactive.zsh" "$cache_dir"
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
    ZDOTDIR="" PATH="$fake_bin:/usr/bin:/bin" SELFISHELL_COMMON_DIR="$ROOT_DIR/config/shared/zsh" \
      /bin/zsh -f -c '
        _selfishell_command_path() { command -v "$1"; }
        source "$1"
        _selfishell_generate_fzf_cache "$2/cache.zsh"
        cat "$2/cache.zsh"
        command find "$2" -maxdepth 1 -name "*.tmp.*" | command wc -l | command tr -d " "
      ' zsh "$ROOT_DIR/config/shared/zsh/interactive.zsh" "$cache_dir"
  )"

  [[ "$output" == *'# preexisting fzf cache'* ]] ||
    fail "A failed final mv corrupted the existing fzf cache: $output"
  [[ "$output" == *$'\n0' ]] ||
    fail "A failed final mv left a temporary fzf cache file behind: $output"
  teardown_test_home
}

# The cp-from-system-docs fallback only fires when fzf is unreachable, so PATH
# is rebuilt from individually symlinked tools: /usr/bin and /bin are the same
# merged directory on most Linux systems and can't hide an installed fzf.
# Skipped where the fallback path doesn't exist at all.
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
    ZDOTDIR="" SELFISHELL_COMMON_DIR="$ROOT_DIR/config/shared/zsh" PATH="$restricted_bin" \
      /bin/zsh -f -c '
        source "$1"
        _selfishell_generate_fzf_cache "$2/fallback-cache.zsh"
        [[ -s "$2/fallback-cache.zsh" ]] && print FALLBACK_WRITTEN
        command find "$2" -maxdepth 1 -name "*.tmp.*" | command wc -l | command tr -d " "
      ' zsh "$ROOT_DIR/config/shared/zsh/interactive.zsh" "$cache_dir" 2>/dev/null
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
    ZDOTDIR="" SELFISHELL_COMMON_DIR="$ROOT_DIR/config/shared/zsh" PATH="$restricted_bin" \
      /bin/zsh -f -c '
        source "$1"
        _selfishell_generate_fzf_cache "$2/fallback-cache.zsh"
        cat "$2/fallback-cache.zsh"
        command find "$2" -maxdepth 1 -name "*.tmp.*" | command wc -l | command tr -d " "
      ' zsh "$ROOT_DIR/config/shared/zsh/interactive.zsh" "$cache_dir" 2>/dev/null
  )"

  [[ "$output" == *'# preexisting fallback cache'* ]] ||
    fail "A failed fallback copy corrupted the existing cache: $output"
  [[ "$output" == *$'\n0' ]] ||
    fail "A failed fallback copy left a temporary file behind: $output"
  teardown_test_home
}

test_interactive_shell_omits_cloud_and_git_aliases() {
  local command_name fake_bin

  setup_test_home
  fake_bin="$TEST_ROOT/bin"
  mkdir -p "$fake_bin"
  for command_name in eza nvim; do
    cat >"$fake_bin/$command_name" <<'EOF'
#!/usr/bin/env sh
exit 0
EOF
    chmod +x "$fake_bin/$command_name"
  done

  XDG_CACHE_HOME="$HOME/.cache" XDG_DATA_HOME="$HOME/.local/share" \
    ZDOTDIR="" SELFISHELL_COMMON_DIR="$ROOT_DIR/config/shared/zsh" PATH="$fake_bin:/usr/bin:/bin" \
    /bin/zsh -f -c '
      _selfishell_command_path() { command -v "$1"; }
      source "$1"
      [[ "${aliases[ls]}" == "eza --group-directories-first" ]] || exit 1
      [[ "${aliases[vim]}" == nvim ]] || exit 1
      (( ! ${+aliases[tf]} )) || exit 1
      (( ! ${+aliases[k]} )) || exit 1
      (( ! ${+aliases[kg]} )) || exit 1
      (( ! ${+aliases[kd]} )) || exit 1
      (( ! ${+aliases[g]} )) || exit 1
    ' zsh "$ROOT_DIR/config/shared/zsh/interactive.zsh" ||
    fail "Interactive shell still defines a cloud or Git alias"
}

test_kubectl_completion_registers_only_canonical_command() {
  local fake_bin

  setup_test_home
  fake_bin="$TEST_ROOT/bin"
  mkdir -p "$fake_bin"
  cat >"$fake_bin/kubectl" <<'EOF'
#!/usr/bin/env sh

[ "$1" = completion ] && [ "$2" = zsh ] || exit 1
printf '%s\n' '_kubectl() { return 0; }'
EOF
  chmod +x "$fake_bin/kubectl"

  XDG_CACHE_HOME="$HOME/.cache" XDG_DATA_HOME="$HOME/.local/share" \
    ZDOTDIR="$HOME" PATH="$fake_bin:/usr/bin:/bin" \
    /bin/zsh -f -c '
      _selfishell_command_path() { command -v "$1"; }
      source "$1"
      [[ "${_comps[kubectl]}" == _selfishell_kubectl_completion ]] || exit 1
      (( ! ${+_comps[k]} )) || exit 1
      _selfishell_kubectl_completion || exit 1
      [[ "${_comps[kubectl]}" == _kubectl ]] || exit 1
      (( ! ${+_comps[k]} )) || exit 1
    ' zsh "$ROOT_DIR/config/shared/zsh/completion.zsh" ||
    fail "Kubectl completion did not keep k unmapped"
}

test_editor_aliases_and_environment_prefer_neovim() {
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
        unset EDITOR VISUAL
        source "$1"
        alias vim
        /bin/sh -c '\''[ "$EDITOR" = nvim ] && [ "$VISUAL" = nvim ]'\'' || exit 1

        EDITOR="code --wait"
        unset VISUAL
        source "$1"
        /bin/sh -c '\''[ "$EDITOR" = "code --wait" ] && [ "$VISUAL" = "code --wait" ]'\'' || exit 1

        unset EDITOR
        VISUAL="emacsclient -c"
        source "$1"
        /bin/sh -c '\''[ "$EDITOR" = nvim ] && [ "$VISUAL" = "emacsclient -c" ]'\'' || exit 1

        EDITOR=nano
        source "$1"
        /bin/sh -c '\''[ "$EDITOR" = nano ] && [ "$VISUAL" = "emacsclient -c" ]'\'' || exit 1
      ' zsh "$ROOT_DIR/config/shared/zsh/aliases.zsh"
  )" || fail "Editor defaults were not exported or replaced the users choice"

  [[ "$output" == *'vim=nvim'* ]] || fail "vim was not redirected to Neovim"
  teardown_test_home
}

test_missing_neovim_keeps_system_vim() {
  local output

  setup_test_home
  output="$(
    PATH="$TEST_ROOT/empty-bin" \
      ZDOTDIR="" \
      /bin/zsh -f -c '
        _selfishell_command_path() { command -v "$1"; }
        unset EDITOR VISUAL
        source "$1"
        alias vim 2>/dev/null || true
        [[ -z ${EDITOR+x} && -z ${VISUAL+x} ]] || exit 1
      ' zsh "$ROOT_DIR/config/shared/zsh/aliases.zsh"
  )" || fail "Editor defaults should not be forced without Neovim"

  [[ "$output" != *'nvim'* ]] || fail "Vim alias should not be forced without Neovim"
  teardown_test_home
}

test_git_staged_diff_alias_is_available() {
  local output

  setup_test_home
  output="$(
    PATH="/usr/bin:/bin" \
      ZDOTDIR="" \
      /bin/zsh -f -c '
        _selfishell_command_path() { command -v "$1"; }
        source "$1"
        alias gds
      ' zsh "$ROOT_DIR/config/shared/zsh/aliases.zsh"
  )"

  [[ "$output" == "gds='git diff --staged'" ]] ||
    fail "gds does not show staged Git changes: $output"
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
zsh-users/zsh-completions config/shared/zsh/completion.zsh
Aloxaf/fzf-tab config/shared/zsh/interactive.zsh
zsh-users/zsh-autosuggestions config/shared/zsh/interactive.zsh
zdharma-continuum/fast-syntax-highlighting config/shared/zsh/interactive.zsh
PLUGINS
}

# Stub Zinit and fzf; callers choose whether the plugin checkout exists.
setup_fzf_tab_stubs() {
  local fake_bin="$TEST_ROOT/bin"
  local zinit_home="$HOME/.local/share/zinit/zinit.git"

  mkdir -p "$fake_bin" "$zinit_home"
  cat >"$fake_bin/fzf" <<'EOF'
#!/bin/sh
printf ':\n'
EOF
  chmod +x "$fake_bin/fzf"
  cat >"$zinit_home/zinit.zsh" <<'EOF'
typeset -gA ZINIT
ZINIT[PLUGINS_DIR]="${XDG_DATA_HOME:-$HOME/.local/share}/zinit/plugins"
zinit() { : }
EOF
}

# Writes each configured preview command to $TEST_ROOT/previews/<name>, so the
# tests can run them the way fzf does instead of matching their source text.
dump_fzf_tab_previews() {
  mkdir -p "$TEST_ROOT/previews"
  XDG_CACHE_HOME="$HOME/.cache" XDG_DATA_HOME="$HOME/.local/share" ZDOTDIR="" \
    PATH="$TEST_ROOT/bin:/usr/bin:/bin" \
    /bin/zsh -f -c '
      source "$1"
      for name context in \
        directory ":fzf-tab:complete:cd:x" \
        file ":fzf-tab:complete:vim:x" \
        branch ":fzf-tab:complete:git-switch:x" \
        diff ":fzf-tab:complete:git-add:x" \
        stash ":fzf-tab:complete:git-stash-show:x" \
        process ":fzf-tab:complete:kill:argument-rest"; do
        zstyle -s "$context" fzf-preview command_string || continue
        print -r -- "$command_string" >"$2/$name"
      done
    ' zsh "$ROOT_DIR/config/shared/zsh/common.zsh" "$TEST_ROOT/previews" 2>/dev/null
}

# Runs preview $1 as fzf does: from directory $2, with $word $3, $realpath $4
# and PATH $5. Stderr goes to $TEST_ROOT/preview-errors; the exit status is
# dropped because only a preview's output streams reach the user.
run_fzf_tab_preview() {
  (
    cd "$2" || exit 1
    PATH="${5:-$PATH}" word="$3" realpath="$4" \
      /bin/zsh -f -c "$(cat "$TEST_ROOT/previews/$1")" 2>"$TEST_ROOT/preview-errors"
  ) || true
}

test_fzf_tab_previews_wait_for_the_plugin() {
  local output

  setup_test_home
  setup_fzf_tab_stubs

  output="$(
    XDG_CACHE_HOME="$HOME/.cache" XDG_DATA_HOME="$HOME/.local/share" ZDOTDIR="" \
      PATH="$TEST_ROOT/bin:/usr/bin:/bin" \
      /bin/zsh -f -c '
        source "$1"
        # zstyle -L reports 1 when nothing matches, which is the passing case.
        zstyle -L ":fzf-tab:complete:*" fzf-preview || true
      ' zsh "$ROOT_DIR/config/shared/zsh/common.zsh" 2>&1
  )"

  [[ -z "$output" ]] ||
    fail "Previews were configured even though fzf-tab is not installed: $output"
  teardown_test_home
}

test_fzf_tab_previews_cover_the_commands_they_advertise() {
  local name previews

  setup_test_home
  setup_fzf_tab_stubs
  previews="$TEST_ROOT/previews"
  mkdir -p "$HOME/.local/share/zinit/plugins/Aloxaf---fzf-tab/.git"

  dump_fzf_tab_previews

  for name in directory file branch diff stash process; do
    [[ -s "$previews/$name" ]] ||
      fail "No fzf-tab preview is configured for $name completion"
  done
  teardown_test_home
}

# The previews are zstyle values, so `zsh -n` never parses them. Run each for
# real with eza, bat and batcat absent, outside a Git repository, against a
# path containing a space.
test_fzf_tab_previews_fall_back_without_leaking_errors() {
  local errors output previews restricted_bin sandbox tool

  setup_test_home
  setup_fzf_tab_stubs
  previews="$TEST_ROOT/previews"
  restricted_bin="$TEST_ROOT/restricted-bin"
  sandbox="$TEST_ROOT/sandbox/a dir"
  mkdir -p "$restricted_bin" "$sandbox" \
    "$HOME/.local/share/zinit/plugins/Aloxaf---fzf-tab/.git"
  printf 'one\ntwo\n' >"$sandbox/a file.txt"
  for tool in git ps head ls; do
    ln -sf "$(command -v "$tool")" "$restricted_bin/$tool"
  done

  dump_fzf_tab_previews

  assert_fzf_tab_preview_is_quiet() {
    run_fzf_tab_preview "$1" "$sandbox" "$2" "$3" "$restricted_bin" >/dev/null
    errors="$(cat "$TEST_ROOT/preview-errors")"
    [[ -z "$errors" ]] ||
      fail "The $1 preview wrote to stderr: $errors"
  }

  # $sandbox is not a repository, so the Git previews run against one here.
  assert_fzf_tab_preview_is_quiet directory '' "$sandbox"
  assert_fzf_tab_preview_is_quiet file '' "$sandbox/a file.txt"
  assert_fzf_tab_preview_is_quiet directory '' "$sandbox/gone"
  assert_fzf_tab_preview_is_quiet branch main ''
  assert_fzf_tab_preview_is_quiet diff 'a file.txt' ''
  assert_fzf_tab_preview_is_quiet stash 'stash@{0}' ''
  assert_fzf_tab_preview_is_quiet process 0 ''

  output="$(run_fzf_tab_preview directory "$sandbox" '' "$sandbox" "$restricted_bin")"
  [[ "$output" == *'a file.txt'* ]] ||
    fail "The directory preview showed nothing without eza: $output"
  output="$(run_fzf_tab_preview file "$sandbox" '' "$sandbox/a file.txt" "$restricted_bin")"
  [[ "$output" == *'two'* ]] ||
    fail "The file preview showed nothing without bat: $output"
  output="$(run_fzf_tab_preview branch "$sandbox" main '' "$restricted_bin")"
  [[ -z "$output" ]] ||
    fail "The branch preview produced output outside a repository: $output"

  unset -f assert_fzf_tab_preview_is_quiet
  teardown_test_home
}

# Quiet is not correct: these previews must produce the right history and hunk.
# The commands they wrap act on the working tree by default, so a preview built
# on `git diff HEAD` would also show staged work none of them touch.
test_fzf_tab_git_previews_read_the_repository() {
  local output repository

  setup_test_home
  setup_fzf_tab_stubs
  repository="$TEST_ROOT/a repository"
  mkdir -p "$repository" "$HOME/.local/share/zinit/plugins/Aloxaf---fzf-tab/.git"
  dump_fzf_tab_previews

  # Isolate Git identity and configuration, including commit signing.
  export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null
  export GIT_AUTHOR_NAME=selfishell GIT_AUTHOR_EMAIL=selfishell@example.invalid
  export GIT_COMMITTER_NAME=selfishell GIT_COMMITTER_EMAIL=selfishell@example.invalid

  git -C "$repository" init -q -b main
  printf 'base\n' >"$repository/staged only.txt"
  printf 'base\n' >"$repository/worktree only.txt"
  git -C "$repository" add -A
  git -C "$repository" commit -q -m 'record the first revision'
  git -C "$repository" branch feature/login
  # A remote-tracking ref with no local branch, which is how a branch someone
  # else pushed reaches the candidate list: zsh offers it stripped of its
  # remote, and `git log release-2` cannot resolve that.
  git -C "$repository" update-ref refs/remotes/origin/release-2 HEAD
  git -C "$repository" commit -q --allow-empty -m 'drop the unused flag'

  output="$(run_fzf_tab_preview branch "$repository" feature/login '')"
  [[ "$output" == *'record the first revision'* ]] ||
    fail "The branch preview did not show the branch's commits: $output"
  [[ "$output" != *'drop the unused flag'* ]] ||
    fail "The branch preview showed commits the branch does not contain: $output"
  output="$(run_fzf_tab_preview branch "$repository" release-2 '')"
  [[ "$output" == *'record the first revision'* ]] ||
    fail "The branch preview did not resolve a remote branch by its bare name: $output"

  # One file changed in the index alone, one in the working tree alone. In a
  # single file the staged line would sit inside the unstaged hunk as context
  # and look present either way, defeating the assertion below.
  printf 'base\nstaged-change\n' >"$repository/staged only.txt"
  git -C "$repository" add 'staged only.txt'
  printf 'base\nworktree-change\n' >"$repository/worktree only.txt"

  output="$(run_fzf_tab_preview diff "$repository" 'worktree only.txt' '')"
  [[ "$output" == *'worktree-change'* ]] ||
    fail "The diff preview did not show the working tree change: $output"
  output="$(run_fzf_tab_preview diff "$repository" 'staged only.txt' '')"
  [[ "$output" != *'staged-change'* ]] ||
    fail "The diff preview showed work that is already staged: $output"

  git -C "$repository" stash push -q -m 'set the parser aside'
  output="$(run_fzf_tab_preview stash "$repository" 'stash@{0}' '')"
  [[ "$output" == *'worktree-change'* ]] ||
    fail "The stash preview did not show what the stash holds: $output"
  teardown_test_home
}

# fzf-tab clears FZF_DEFAULT_OPTS, so it needs the terminal palette flag separately.
test_fzf_is_pointed_at_the_terminal_palette() {
  local output

  setup_test_home
  setup_fzf_tab_stubs
  mkdir -p "$HOME/.local/share/zinit/plugins/Aloxaf---fzf-tab/.git"

  output="$(
    XDG_CACHE_HOME="$HOME/.cache" XDG_DATA_HOME="$HOME/.local/share" ZDOTDIR="" \
      PATH="$TEST_ROOT/bin:/usr/bin:/bin" \
      /bin/zsh -f -c '
        source "$1"
        print -r -- "opts=$FZF_DEFAULT_OPTS"
        print -r -- "type=${(t)FZF_DEFAULT_OPTS}"
        local -a flags
        zstyle -a ":fzf-tab:complete:cd:x" fzf-flags flags && print -r -- "cd=${(j: :)flags}"
        # zstyle answers with one pattern rather than a union, so a context that
        # sets fzf-flags of its own has to carry the palette itself.
        zstyle -a ":fzf-tab:complete:kill:argument-rest" fzf-flags flags &&
          print -r -- "kill=${(j: :)flags}"
      ' zsh "$ROOT_DIR/config/shared/zsh/common.zsh" 2>/dev/null
  )"

  [[ "$output" == *'opts=--color=16'* ]] ||
    fail "fzf was not pointed at the terminal's own colors: $output"
  [[ "$output" == *'type='*export* ]] ||
    fail "FZF_DEFAULT_OPTS was not exported, so fzf will not see it: $output"
  [[ "$(grep '^cd=' <<<"$output")" == *--color=16* ]] ||
    fail "fzf-tab was not given the supported terminal palette option: $output"
  [[ "$output" == *'kill='*--color=16* ]] ||
    fail "A context with its own fzf-flags lost the palette: $output"
  teardown_test_home
}

# FZF_DEFAULT_OPTS belongs to the user: it must keep working for the standalone
# widgets and must not reach fzf-tab, which several flags break -- --with-nth
# defeats the NUL encoding it uses for candidates.
test_fzf_tab_is_not_handed_the_users_fzf_options() {
  local output

  setup_test_home
  setup_fzf_tab_stubs
  mkdir -p "$HOME/.local/share/zinit/plugins/Aloxaf---fzf-tab/.git"

  output="$(
    XDG_CACHE_HOME="$HOME/.cache" XDG_DATA_HOME="$HOME/.local/share" ZDOTDIR="" \
      PATH="$TEST_ROOT/bin:/usr/bin:/bin" \
      FZF_DEFAULT_OPTS='--with-nth=2.. --bind=ctrl-a:select-all' \
      /bin/zsh -f -c '
        source "$1"
        print -r -- "opts=$FZF_DEFAULT_OPTS"
        local -a flags
        zstyle -a ":fzf-tab:complete:cd:x" fzf-flags flags && print -r -- "cd=${(j: :)flags}"
        # zstyle -s blanks the variable when nothing matches, so the sentinel
        # has to come from its exit status rather than from an initial value.
        local follows
        zstyle -s ":fzf-tab:x" use-fzf-default-opts follows || follows=unset
        print -r -- "follows=$follows"
      ' zsh "$ROOT_DIR/config/shared/zsh/common.zsh" 2>/dev/null
  )"

  [[ "$output" == *'opts=--with-nth=2.. --bind=ctrl-a:select-all'* ]] ||
    fail "The user's own FZF_DEFAULT_OPTS was overwritten: $output"
  [[ "$(grep '^cd=' <<<"$output")" != *--with-nth* &&
  "$(grep '^cd=' <<<"$output")" != *--bind* ]] ||
    fail "fzf-tab was handed the user's standalone options: $output"
  [[ "$output" == *'follows=unset'* ]] ||
    fail "fzf-tab was told to read FZF_DEFAULT_OPTS: $output"
  teardown_test_home
}

run_discovered_tests '' teardown_test_home
