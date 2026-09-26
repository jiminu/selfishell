#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
ITERATIONS="${SELFISHELL_BENCHMARK_ITERATIONS:-30}"
ENFORCE_BUDGETS="${SELFISHELL_BENCHMARK_ENFORCE:-0}"
PROFILE_MODE="${SELFISHELL_BENCHMARK_PROFILE:-base}"
RESULTS_FILE="${SELFISHELL_BENCHMARK_RESULTS_FILE:-}"
ZPROF_FILE="${SELFISHELL_BENCHMARK_ZPROF_FILE:-}"
MEASURE_PROMPT=0
MEASURE_DIAGNOSTICS=0

usage() {
  cat <<'EOF'
Usage: scripts/benchmark.sh [--mode base|full] [--prompt] [--diagnostics]

  base  Selfishell's own startup cost, independent of external integrations
        (mise/starship/zinit/fzf/zoxide are excluded). This is the default.

  full  Installs the pinned mise, starship, fzf, zoxide, and zinit (with its pinned
        plugins) into an isolated HOME before measuring, so the
        interactive-cached metric reflects a real full-environment
        startup. Starship, fzf, and zoxide are installed through mise.
        This script does not install Apt/Homebrew packages.

  --prompt       Measure first and command-to-prompt latency using a PTY
                 in an empty directory and this repository (requires python3).
  --diagnostics  Measure status and doctor after isolated configuration setup.
                 Full mode includes the caller's PATH tools and package managers.

SELFISHELL_BENCHMARK_PROFILE=base|full is equivalent to --mode.
EOF
}

while (("$#" > 0)); do
  case "$1" in
    --mode)
      shift
      if (($# == 0)); then
        printf '%s\n' '--mode requires base or full' >&2
        usage >&2
        exit 2
      fi
      PROFILE_MODE="$1"
      ;;
    --prompt) MEASURE_PROMPT=1 ;;
    --diagnostics) MEASURE_DIAGNOSTICS=1 ;;
    --help | -h)
      usage
      exit 0
      ;;
    *)
      printf 'Unknown option: %s\n' "$1" >&2
      usage >&2
      exit 2
      ;;
  esac
  shift
done

case "$PROFILE_MODE" in
  base | full) ;;
  *)
    printf -- '--mode/SELFISHELL_BENCHMARK_PROFILE must be "base" or "full" (got: %s)\n' "$PROFILE_MODE" >&2
    exit 2
    ;;
esac

if [[ "$MEASURE_PROMPT" == 1 ]] && ! command -v python3 >/dev/null 2>&1; then
  printf '%s\n' '--prompt requires python3 (standard library only).' >&2
  exit 1
fi

# Validate arguments before creating any temporary files.
TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/selfishell-benchmark.XXXXXX")"
TEST_HOME="$TEST_ROOT/home"
TEST_DATA_HOME="$TEST_HOME/.local/share"
export MISE_DATA_DIR="$TEST_DATA_HOME/mise"
export MISE_CACHE_DIR="$TEST_HOME/.cache/mise"
export MISE_STATE_DIR="$TEST_HOME/.local/state/mise"
trap 'rm -rf "$TEST_ROOT"' EXIT
mkdir -p "$TEST_HOME/.cache/selfishell" "$TEST_HOME/.config/mise" \
  "$TEST_HOME/.config/selfishell/zsh" "$TEST_HOME/.local/bin" "$TEST_DATA_HOME"
: >"$TEST_HOME/.config/mise/config.toml"

case "$(uname -s)" in
  Darwin) PLATFORM_CONFIG="$ROOT_DIR/config/macos/zshrc" ;;
  *) PLATFORM_CONFIG="$ROOT_DIR/config/ubuntu/zshrc" ;;
esac

if [[ "$PROFILE_MODE" == base && "$(uname -s)" == Darwin ]]; then
  printf '#!/bin/sh\nexit 0\n' >"$TEST_HOME/.local/bin/brew"
  chmod +x "$TEST_HOME/.local/bin/brew"
fi

ln -s "$ROOT_DIR/config/shared/zsh/common.zsh" "$TEST_HOME/.config/selfishell/zsh/common.zsh"
ln -s "$ROOT_DIR/config/shared/zsh/runtime.zsh" "$TEST_HOME/.config/selfishell/zsh/runtime.zsh"
ln -s "$ROOT_DIR/config/shared/zsh/history.zsh" "$TEST_HOME/.config/selfishell/zsh/history.zsh"
ln -s "$ROOT_DIR/config/shared/zsh/completion.zsh" "$TEST_HOME/.config/selfishell/zsh/completion.zsh"
ln -s "$ROOT_DIR/config/shared/zsh/interactive.zsh" "$TEST_HOME/.config/selfishell/zsh/interactive.zsh"
ln -s "$ROOT_DIR/config/shared/zsh/update-notice.zsh" "$TEST_HOME/.config/selfishell/zsh/update-notice.zsh"
ln -s "$ROOT_DIR/config/shared/zsh/aliases.zsh" "$TEST_HOME/.config/selfishell/zsh/aliases.zsh"
ln -s "$PLATFORM_CONFIG" "$TEST_HOME/.zshrc"
date +%s >"$TEST_HOME/.cache/selfishell/update-checked-at"

[[ -r "$TEST_HOME/.config/selfishell/zsh/aliases.zsh" ]] || {
  printf 'Missing benchmark Zsh module: aliases.zsh\n' >&2
  exit 1
}

# Installs the pinned shell integrations into $TEST_HOME so "full" mode
# measures a real full-environment startup, not the runner's PATH, reusing
# the production installers.
install_full_integrations() (
  export HOME="$TEST_HOME" XDG_CONFIG_HOME="$TEST_HOME/.config"
  export XDG_DATA_HOME="$TEST_DATA_HOME" XDG_STATE_HOME="$TEST_HOME/.local/state"
  export XDG_CACHE_HOME="$TEST_HOME/.cache" MISE_CEILING_PATHS="$ROOT_DIR"
  cd "$TEST_HOME"
  (cd "$ROOT_DIR" && go run ./cmd/selfishell-dev "$ROOT_DIR" benchmark-shell)

  # Restrict interactive startup to the three measured shell integrations.
  awk '
    BEGIN { print "[tools]" }
    /^\[/ { in_tools = ($0 == "[tools]"); next }
    in_tools && ($1 == "starship" || $1 == "fzf" || $1 == "zoxide") { print; found++ }
    END { print "\n[settings]\nnot_found_auto_install = false"; exit(found != 3) }
  ' "$ROOT_DIR/config/shared/mise.toml" >"$TEST_HOME/.config/mise/config.toml"
)

if [[ "$PROFILE_MODE" == full ]]; then
  install_full_integrations
fi

case "$PROFILE_MODE" in
  base)
    # Completion uses mv for the dump and touch for its empty audit marker.
    ln -s /bin/mv "$TEST_HOME/.local/bin/mv"
    ln -s /usr/bin/touch "$TEST_HOME/.local/bin/touch"
    COMMON_PATH="$TEST_HOME/.local/bin"
    INTERACTIVE_PATH="$ROOT_DIR/bin:$TEST_HOME/.local/bin"
    ;;
  full)
    COMMON_PATH="$TEST_HOME/.local/bin:/usr/bin:/bin"
    INTERACTIVE_PATH="$ROOT_DIR/bin:$TEST_HOME/.local/bin:$PATH"
    ;;
esac

validate_iterations() {
  case "$ITERATIONS" in
    "" | *[!0-9]* | 0)
      printf 'SELFISHELL_BENCHMARK_ITERATIONS must be a positive integer\n' >&2
      exit 2
      ;;
  esac
}

benchmark() {
  local label="$1"
  local iterations="$2"
  shift 2

  perl -MTime::HiRes=time -e '
    $label = shift @ARGV;
    $iterations = shift @ARGV;
    @samples = ();
    for (1 .. $iterations) {
      $started = time;
      system(@ARGV) == 0 or exit 1;
      push @samples, (time - $started) * 1000;
    }
    @sorted = sort { $a <=> $b } @samples;
    $sum += $_ for @samples;
    $p50 = $sorted[int($iterations * 0.50 + 0.999999) - 1];
    $p95 = $sorted[int($iterations * 0.95 + 0.999999) - 1];
    $max = $sorted[-1];
    printf "%s\t%.3f\t%.3f\t%.3f\t%.3f\n",
      $label, $sum / $iterations, $p50, $p95, $max;
  ' "$label" "$iterations" "$@"
}

check_budget() {
  local label="$1"
  local actual="$2"
  local budget="$3"

  [[ -n "$budget" ]] || return 0
  if awk -v actual="$actual" -v budget="$budget" 'BEGIN { exit !(actual > budget) }'; then
    if [[ "$ENFORCE_BUDGETS" == 1 ]]; then
      printf 'Benchmark budget exceeded: %s p95 %.3fms > %.3fms\n' \
        "$label" "$actual" "$budget" >&2
      return 1
    fi
    if [[ "${GITHUB_ACTIONS:-}" == true ]]; then
      printf '::warning title=Shell performance budget::%s p95 %.3fms exceeds %.3fms\n' \
        "$label" "$actual" "$budget" >&2
    else
      printf 'WARNING: benchmark budget exceeded: %s p95 %.3fms > %.3fms\n' \
        "$label" "$actual" "$budget" >&2
    fi
  fi
}

record_result() {
  local result="$1"

  printf '%s\n' "$result"
  if [[ -n "$RESULTS_FILE" ]]; then
    printf '%s\t%s\t%s\t%s\n' "$(uname -s)" "$(uname -m)" "$PROFILE_MODE" "$result" >>"$RESULTS_FILE"
  fi
}

run_prompt_benchmark() {
  local prompt_results result

  verify_full_integrations "$ROOT_DIR"
  mkdir "$TEST_ROOT/prompt"
  cat >"$TEST_ROOT/prompt/.zshrc" <<'EOF'
source "$SELFISHELL_BENCHMARK_PLATFORM_CONFIG"
autoload -Uz add-zsh-hook
setopt promptsubst
typeset -gi _selfishell_benchmark_prompt=0
_selfishell_benchmark_precmd() { (( ++_selfishell_benchmark_prompt )); }
add-zsh-hook precmd _selfishell_benchmark_precmd
RPROMPT+='__SFS_READY_${_selfishell_benchmark_prompt}__'
EOF
  prompt_results="$(HOME="$TEST_HOME" ZDOTDIR="$TEST_ROOT/prompt" \
    SELFISHELL_BENCHMARK_PLATFORM_CONFIG="$PLATFORM_CONFIG" \
    SELFISHELL_BENCHMARK_PATH="$INTERACTIVE_PATH" \
    python3 "$ROOT_DIR/scripts/benchmark-prompt.py" "$ITERATIONS" "$ROOT_DIR")" || return
  while IFS= read -r result; do
    record_result "$result"
  done <<<"$prompt_results"
}

run_diagnostic_command() (
  local diagnostic_shell
  diagnostic_shell="$(PATH="$DIAGNOSTIC_PATH" command -v zsh)"
  cd "$DIAGNOSTIC_HOME"
  env -i HOME="$DIAGNOSTIC_HOME" PATH="$DIAGNOSTIC_PATH" SHELL="$diagnostic_shell" \
    TMPDIR="$TEST_ROOT" TERM=dumb NO_COLOR=1 \
    XDG_CONFIG_HOME="$DIAGNOSTIC_HOME/.config" XDG_DATA_HOME="$DIAGNOSTIC_HOME/.local/share" \
    XDG_STATE_HOME="$DIAGNOSTIC_HOME/.local/state" XDG_CACHE_HOME="$DIAGNOSTIC_HOME/.cache" \
    MISE_DATA_DIR="$TEST_DATA_HOME/mise" MISE_CACHE_DIR="$DIAGNOSTIC_HOME/.cache/mise" \
    MISE_STATE_DIR="$DIAGNOSTIC_HOME/.local/state/mise" MISE_OFFLINE=1 MISE_CEILING_PATHS="$ROOT_DIR" \
    HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_ANALYTICS=1 \
    /bin/bash "$ROOT_DIR/bin/selfishell" "$@"
)

run_diagnostic_sample() {
  local command="$1" expected_status="$2" status=0
  run_diagnostic_command "$command" >/dev/null 2>&1 || status=$?
  if [[ "$status" != "$expected_status" ]]; then
    printf 'Diagnostic %s exit changed from %s to %s.\n' "$command" "$expected_status" "$status" >&2
    return 1
  fi
}

run_diagnostic_benchmark() {
  local command status output result
  export DIAGNOSTIC_HOME="$TEST_ROOT/diagnostics-home"
  export DIAGNOSTIC_PATH=/usr/bin:/bin
  [[ "$PROFILE_MODE" != full ]] || DIAGNOSTIC_PATH="$TEST_HOME/.local/bin:$PATH"
  mkdir -p "$DIAGNOSTIC_HOME/.local/share"
  if [[ -d "$TEST_DATA_HOME/zinit" ]]; then
    ln -s "$TEST_DATA_HOME/zinit" "$DIAGNOSTIC_HOME/.local/share/zinit"
  fi
  run_diagnostic_command install --skip-packages --yes >"$TEST_ROOT/diagnostics-setup.log" 2>&1 || {
    cat "$TEST_ROOT/diagnostics-setup.log" >&2
    return 1
  }
  record_result '# Diagnostics: configured HOME; no system packages installed; missing tools may yield exit 1.'
  export -f run_diagnostic_command run_diagnostic_sample
  for command in status doctor; do
    status=0
    output="$(run_diagnostic_command "$command" 2>&1)" || status=$?
    case "$status" in
      0 | 1) ;;
      *)
        printf '%s\n' "$output" >&2
        return "$status"
        ;;
    esac
    record_result "# cli-$command exit=$status"
    printf '%s\n' "$output"
    # shellcheck disable=SC2016 # Run the exported function in each timed child.
    result="$(benchmark "cli-$command" "$ITERATIONS" bash -c 'run_diagnostic_sample "$@"' _ "$command" "$status")" || return
    record_result "$result"
  done
}

run_common_zsh() {
  # In full mode, $TEST_HOME/.local/bin holds the pinned integrations. Base
  # mode uses it only for the benchmark-only macOS brew barrier.
  cd "$TEST_HOME"
  HOME="$TEST_HOME" XDG_DATA_HOME="$TEST_DATA_HOME" XDG_CACHE_HOME="$TEST_HOME/.cache" \
    MISE_GLOBAL_CONFIG_FILE="$TEST_HOME/.config/mise/config.toml" MISE_SHELL='' \
    PATH="$COMMON_PATH" TERM=xterm-256color \
    /bin/zsh -f -c 'source "$1"' \
    zsh "$ROOT_DIR/config/shared/zsh/common.zsh" >/dev/null 2>&1
}

run_interactive_zsh() {
  cd "$TEST_HOME"
  HOME="$TEST_HOME" ZDOTDIR="$TEST_HOME" XDG_CONFIG_HOME="$TEST_HOME/.config" \
    XDG_DATA_HOME="$TEST_DATA_HOME" XDG_CACHE_HOME="$TEST_HOME/.cache" \
    MISE_GLOBAL_CONFIG_FILE="$TEST_HOME/.config/mise/config.toml" MISE_SHELL='' \
    PATH="$INTERACTIVE_PATH" TERM=xterm-256color \
    /bin/zsh -d -i -c exit >/dev/null 2>&1
}

verify_full_integrations() {
  local directory="${1:-$TEST_HOME}"
  [[ "$PROFILE_MODE" == full ]] || return 0
  (
    cd "$directory"
    HOME="$TEST_HOME" ZDOTDIR="$TEST_HOME" XDG_CONFIG_HOME="$TEST_HOME/.config" \
      XDG_DATA_HOME="$TEST_DATA_HOME" XDG_CACHE_HOME="$TEST_HOME/.cache" \
      MISE_GLOBAL_CONFIG_FILE="$TEST_HOME/.config/mise/config.toml" MISE_SHELL='' \
      PATH="$INTERACTIVE_PATH" TERM=xterm-256color MISE_OFFLINE=1 MISE_CEILING_PATHS="$ROOT_DIR" \
      /bin/zsh -d -i -c '
        for tool in starship fzf zoxide; do
          [[ "${commands[$tool]}" == "$MISE_DATA_DIR/installs/"* ]] || exit 1
        done
        (( $+functions[prompt_starship_precmd] && $+functions[fzf-file-widget] && $+functions[__zoxide_z] ))
      ' >/dev/null 2>&1
  ) || {
    printf 'Pinned shell integrations did not initialize in the benchmark HOME.\n' >&2
    return 1
  }
}

profile_interactive_zsh() {
  local profile_status=0

  printf 'zmodload zsh/zprof\n' >"$TEST_HOME/.zshenv"
  cd "$TEST_HOME"
  HOME="$TEST_HOME" ZDOTDIR="$TEST_HOME" XDG_CONFIG_HOME="$TEST_HOME/.config" \
    XDG_DATA_HOME="$TEST_DATA_HOME" XDG_CACHE_HOME="$TEST_HOME/.cache" \
    MISE_GLOBAL_CONFIG_FILE="$TEST_HOME/.config/mise/config.toml" MISE_SHELL='' \
    PATH="$INTERACTIVE_PATH" TERM=xterm-256color \
    /bin/zsh -d -i -c 'zprof' >"$ZPROF_FILE" 2>&1 || profile_status=$?
  rm -f "$TEST_HOME/.zshenv"
  return "$profile_status"
}

describe_integrations() {
  local integration status
  local summary="Interactive integrations:"

  for integration in starship fzf zoxide; do
    if [[ "$PROFILE_MODE" == full ]] &&
      HOME="$TEST_HOME" XDG_CONFIG_HOME="$TEST_HOME/.config" \
        MISE_GLOBAL_CONFIG_FILE="$TEST_HOME/.config/mise/config.toml" \
        "$TEST_HOME/.local/bin/mise" -C "$TEST_HOME" which "$integration" >/dev/null 2>&1; then
      status=enabled
    elif PATH="$INTERACTIVE_PATH" command -v "$integration" >/dev/null 2>&1; then
      status=enabled
    else
      status=absent
    fi
    summary="$summary $integration=$status"
  done
  if [[ -s "$TEST_HOME/.local/share/zinit/zinit.git/zinit.zsh" ]]; then
    status=enabled
  else
    status=absent
  fi
  summary="$summary zinit=$status"

  printf '%s\n' "$summary"
  if [[ -n "$RESULTS_FILE" ]]; then
    printf '%s\t%s\t%s\t# %s\n' "$(uname -s)" "$(uname -m)" "$PROFILE_MODE" "$summary" >>"$RESULTS_FILE"
  fi
}

validate_iterations
printf 'Selfishell benchmark (mode=%s, %s iterations, milliseconds per run)\n' "$PROFILE_MODE" "$ITERATIONS"
printf 'metric\tmean\tp50\tp95\tmax\n'
describe_integrations

baseline_result="$(benchmark baseline-zsh "$ITERATIONS" /bin/zsh -f -c ':')"
record_result "$baseline_result"

# The first run creates the completion dump. Following measurements represent
# the cached common configuration used during ordinary startup.
export -f run_common_zsh run_interactive_zsh
export ROOT_DIR TEST_ROOT TEST_HOME TEST_DATA_HOME COMMON_PATH INTERACTIVE_PATH
record_result "$(benchmark common-first 1 bash -c 'run_common_zsh')"
common_result="$(benchmark common-cached "$ITERATIONS" bash -c 'run_common_zsh')"
record_result "$common_result"

# Warm the complete interactive configuration before measuring it.
run_interactive_zsh
verify_full_integrations
interactive_result="$(benchmark interactive-cached "$ITERATIONS" bash -c 'run_interactive_zsh')"
record_result "$interactive_result"

# The positional parameter is intentionally expanded by the child Bash.
# shellcheck disable=SC2016
version_result="$(benchmark cli-version "$ITERATIONS" bash -c '"$1" version >/dev/null' bash "$ROOT_DIR/bin/selfishell")"
# shellcheck disable=SC2016
help_result="$(benchmark cli-help "$ITERATIONS" bash -c '"$1" help >/dev/null' bash "$ROOT_DIR/bin/selfishell")"
record_result "$version_result"
record_result "$help_result"

check_budget common-cached "$(printf '%s\n' "$common_result" | awk -F '\t' '{ print $4 }')" \
  "${SELFISHELL_BENCHMARK_COMMON_P95_MAX_MS:-}"
check_budget interactive-cached "$(printf '%s\n' "$interactive_result" | awk -F '\t' '{ print $4 }')" \
  "${SELFISHELL_BENCHMARK_INTERACTIVE_P95_MAX_MS:-}"
check_budget cli-version "$(printf '%s\n' "$version_result" | awk -F '\t' '{ print $4 }')" \
  "${SELFISHELL_BENCHMARK_VERSION_P95_MAX_MS:-}"
check_budget cli-help "$(printf '%s\n' "$help_result" | awk -F '\t' '{ print $4 }')" \
  "${SELFISHELL_BENCHMARK_HELP_P95_MAX_MS:-}"

if [[ -n "$ZPROF_FILE" ]]; then
  profile_interactive_zsh
fi

if [[ "$MEASURE_PROMPT" == 1 ]]; then
  run_prompt_benchmark
fi
if [[ "$MEASURE_DIAGNOSTICS" == 1 ]]; then
  run_diagnostic_benchmark
fi
