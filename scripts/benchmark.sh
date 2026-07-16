#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ITERATIONS="${SELFISHELL_BENCHMARK_ITERATIONS:-30}"
TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/selfishell-benchmark.XXXXXX")"
TEST_HOME="$TEST_ROOT/home"

trap 'rm -rf "$TEST_ROOT"' EXIT
mkdir -p "$TEST_HOME/.cache"

benchmark() {
  local label="$1"
  local iterations="$2"
  shift 2

  perl -MTime::HiRes=time -e '
    $label = shift @ARGV;
    $iterations = shift @ARGV;
    $started = time;
    for (1 .. $iterations) {
      system(@ARGV) == 0 or exit 1;
    }
    printf "%s\t%.3f\n", $label, (time - $started) * 1000 / $iterations;
  ' "$label" "$iterations" "$@"
}

assert_budget() {
  local label="$1"
  local actual="$2"
  local budget="$3"

  [[ -n "$budget" ]] || return 0
  awk -v label="$label" -v actual="$actual" -v budget="$budget" '
    BEGIN {
      if (actual > budget) {
        printf "Benchmark budget exceeded: %s %.3fms > %.3fms\n", label, actual, budget > "/dev/stderr"
        exit 1
      }
    }
  '
}

run_common_zsh() {
  HOME="$TEST_HOME" XDG_CACHE_HOME="$TEST_HOME/.cache" PATH=/usr/bin:/bin \
    /bin/zsh -f -c 'load_nvm() { :; }; source "$1"' \
    zsh "$ROOT_DIR/common/common.zsh" >/dev/null 2>&1
}

printf 'Selfishell benchmark (%s iterations, milliseconds per run)\n' "$ITERATIONS"
benchmark baseline-zsh "$ITERATIONS" /bin/zsh -f -c ':'

# Export the helper for the timed child Bash process. The first run creates the
# completion dump, and the following measurement represents cached startup.
export -f run_common_zsh
export ROOT_DIR TEST_HOME
benchmark common-first 1 bash -c 'run_common_zsh'
common_result="$(benchmark common-cached "$ITERATIONS" bash -c 'run_common_zsh')"
printf '%s\n' "$common_result"

# The positional parameter is intentionally expanded by the child Bash.
# shellcheck disable=SC2016
version_result="$(benchmark cli-version "$ITERATIONS" bash -c '"$1" version >/dev/null' bash "$ROOT_DIR/bin/selfishell")"
# shellcheck disable=SC2016
help_result="$(benchmark cli-help "$ITERATIONS" bash -c '"$1" help >/dev/null' bash "$ROOT_DIR/bin/selfishell")"
printf '%s\n%s\n' "$version_result" "$help_result"

assert_budget common-cached "${common_result#*$'\t'}" "${SELFISHELL_BENCHMARK_COMMON_MAX_MS:-}"
assert_budget cli-version "${version_result#*$'\t'}" "${SELFISHELL_BENCHMARK_VERSION_MAX_MS:-}"
assert_budget cli-help "${help_result#*$'\t'}" "${SELFISHELL_BENCHMARK_HELP_MAX_MS:-}"
