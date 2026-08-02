#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ITERATIONS="${SELFISHELL_BENCHMARK_ITERATIONS:-30}"
ENFORCE_BUDGETS="${SELFISHELL_BENCHMARK_ENFORCE:-0}"
PROFILE_MODE="${SELFISHELL_BENCHMARK_PROFILE:-base}"
RESULTS_FILE="${SELFISHELL_BENCHMARK_RESULTS_FILE:-}"
ZPROF_FILE="${SELFISHELL_BENCHMARK_ZPROF_FILE:-}"

usage() {
  cat <<'EOF'
Usage: scripts/benchmark.sh [--mode base|full]

  base  Selfishell's own startup cost, independent of external integrations
        (mise/starship/zinit/fzf/zoxide are excluded).
        This is the default and what CI runs on every push/PR.

  full  Installs the pinned mise, starship, and zinit (with its pinned
        plugins) into an isolated HOME before measuring, so the
        interactive-cached metric reflects a real developer-profile
        startup. fzf and zoxide are measured if already on PATH (install
        them via the platform package manager before running this mode);
        this script does not invoke a package manager itself.

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

# Argument parsing and mode validation happen above, before this creates
# anything on disk, so --help/a bad --mode/an unknown option can never
# leave a benchmark temp directory behind.
TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/selfishell-benchmark.XXXXXX")"
TEST_HOME="$TEST_ROOT/home"
trap 'rm -rf "$TEST_ROOT"' EXIT
mkdir -p "$TEST_HOME/.cache/selfishell" "$TEST_HOME/.config/mise" \
  "$TEST_HOME/.config/selfishell/zsh" "$TEST_HOME/.local/bin"
: >"$TEST_HOME/.config/mise/config.toml"

case "$(uname -s)" in
  Darwin) PLATFORM_CONFIG="$ROOT_DIR/mac/.zshrc" ;;
  *) PLATFORM_CONFIG="$ROOT_DIR/ubuntu/.zshrc" ;;
esac

if [[ "$PROFILE_MODE" == base && "$(uname -s)" == Darwin ]]; then
  printf '#!/bin/sh\nexit 0\n' >"$TEST_HOME/.local/bin/brew"
  chmod +x "$TEST_HOME/.local/bin/brew"
fi

ln -s "$ROOT_DIR/common/common.zsh" "$TEST_HOME/.config/selfishell/zsh/common.zsh"
ln -s "$ROOT_DIR/common/runtime.zsh" "$TEST_HOME/.config/selfishell/zsh/runtime.zsh"
ln -s "$ROOT_DIR/common/completion.zsh" "$TEST_HOME/.config/selfishell/zsh/completion.zsh"
ln -s "$ROOT_DIR/common/interactive.zsh" "$TEST_HOME/.config/selfishell/zsh/interactive.zsh"
ln -s "$ROOT_DIR/common/update-notice.zsh" "$TEST_HOME/.config/selfishell/zsh/update-notice.zsh"
ln -s "$ROOT_DIR/common/aliases-common.zsh" "$TEST_HOME/.config/selfishell/zsh/aliases-common.zsh"
ln -s "$ROOT_DIR/common/aliases-git.zsh" "$TEST_HOME/.config/selfishell/zsh/aliases-git.zsh"
ln -s "$ROOT_DIR/common/aliases-kubectl.zsh" "$TEST_HOME/.config/selfishell/zsh/aliases-kubectl.zsh"
ln -s "$PLATFORM_CONFIG" "$TEST_HOME/.zshrc"
date +%s >"$TEST_HOME/.cache/selfishell/update-checked-at"

# Installs the pinned mise/starship/zinit into the isolated $TEST_HOME so
# "full" mode measures a real developer-profile startup, not just whatever
# happens to already be on the runner's PATH. Reuses dependency_install
# (the same code the real installer uses) rather than reimplementing
# download/checkout logic here. fzf and zoxide are intentionally left to
# the platform package manager -- installing packages is out of scope for
# this script -- so provision them separately before running --mode full.
install_full_profile_integrations() {
  local name status=0

  for name in mise starship zinit; do
    HOME="$TEST_HOME" XDG_STATE_HOME="$TEST_HOME/.local/state" XDG_CACHE_HOME="$TEST_HOME/.cache" \
      SELFISHELL_ROOT="$ROOT_DIR" \
      bash -c '
        source "$1/lib/common.sh"
        source "$1/lib/paths.sh"
        source "$1/lib/platform.sh"
        source "$1/lib/dependencies.sh"
        source "$1/lib/installers.sh"
        install_direct_package required "$2" 0
      ' _ "$ROOT_DIR" "$name" || status=1
  done

  ((status == 0)) || {
    printf 'Failed to provision one or more full-profile integrations (mise/starship/zinit)\n' >&2
    exit 1
  }
}

if [[ "$PROFILE_MODE" == full ]]; then
  install_full_profile_integrations
fi

case "$PROFILE_MODE" in
  base) INTERACTIVE_PATH="$ROOT_DIR/bin:$TEST_HOME/.local/bin:/usr/bin:/bin" ;;
  full) INTERACTIVE_PATH="$ROOT_DIR/bin:$TEST_HOME/.local/bin:$PATH" ;;
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

run_common_zsh() {
  # In full mode, $TEST_HOME/.local/bin holds the pinned integrations. Base
  # mode uses it only for the benchmark-only macOS brew barrier.
  cd "$TEST_HOME"
  HOME="$TEST_HOME" XDG_CACHE_HOME="$TEST_HOME/.cache" \
    MISE_GLOBAL_CONFIG_FILE="$TEST_HOME/.config/mise/config.toml" MISE_SHELL='' \
    PATH="$TEST_HOME/.local/bin:/usr/bin:/bin" TERM=xterm-256color \
    /bin/zsh -f -c 'source "$1"' \
    zsh "$ROOT_DIR/common/common.zsh" >/dev/null 2>&1
}

run_interactive_zsh() {
  cd "$TEST_HOME"
  HOME="$TEST_HOME" ZDOTDIR="$TEST_HOME" XDG_CONFIG_HOME="$TEST_HOME/.config" \
    XDG_CACHE_HOME="$TEST_HOME/.cache" \
    MISE_GLOBAL_CONFIG_FILE="$TEST_HOME/.config/mise/config.toml" MISE_SHELL='' \
    PATH="$INTERACTIVE_PATH" TERM=xterm-256color \
    /bin/zsh -d -i -c exit >/dev/null 2>&1
}

profile_interactive_zsh() {
  local profile_status=0

  printf 'zmodload zsh/zprof\n' >"$TEST_HOME/.zshenv"
  cd "$TEST_HOME"
  HOME="$TEST_HOME" ZDOTDIR="$TEST_HOME" XDG_CONFIG_HOME="$TEST_HOME/.config" \
    XDG_CACHE_HOME="$TEST_HOME/.cache" \
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
    if PATH="$INTERACTIVE_PATH" command -v "$integration" >/dev/null 2>&1; then
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
export ROOT_DIR TEST_HOME INTERACTIVE_PATH
record_result "$(benchmark common-first 1 bash -c 'run_common_zsh')"
common_result="$(benchmark common-cached "$ITERATIONS" bash -c 'run_common_zsh')"
record_result "$common_result"

# Warm the complete interactive configuration before measuring it.
run_interactive_zsh
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
