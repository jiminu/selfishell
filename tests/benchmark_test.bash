#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/tests/test_helper.bash"

# These only exercise argument parsing and --mode base (no external
# integrations, no network); --mode full is covered live by the
# shell-full-profile-benchmark CI job, not here, since it provisions real
# tools over the network and would make the regular test suite
# network-dependent.

test_benchmark_rejects_unknown_mode() {
  local status=0
  local output

  output="$(bash "$ROOT_DIR/scripts/benchmark.sh" --mode bogus 2>&1)" || status=$?

  [[ "$status" -eq 2 ]] || fail "An unknown --mode should exit 2 (got $status)"
  [[ "$output" == *'must be "base" or "full"'* ]] || fail "Unknown --mode did not explain the valid values: $output"
}

test_benchmark_rejects_unknown_option() {
  local status=0
  local output

  output="$(bash "$ROOT_DIR/scripts/benchmark.sh" --bogus-flag 2>&1)" || status=$?

  [[ "$status" -eq 2 ]] || fail "An unknown option should exit 2 (got $status)"
  [[ "$output" == *'Unknown option: --bogus-flag'* ]] || fail "Unknown option did not name the flag: $output"
}

test_benchmark_help_documents_both_modes() {
  local output

  output="$(bash "$ROOT_DIR/scripts/benchmark.sh" --help)"

  [[ "$output" == *'base'* && "$output" == *'full'* ]] ||
    fail "--help did not document both benchmark modes: $output"
}

test_benchmark_base_mode_runs_without_network() {
  local output
  local status=0

  output="$(SELFISHELL_BENCHMARK_ITERATIONS=1 bash "$ROOT_DIR/scripts/benchmark.sh" --mode base 2>&1)" || status=$?

  ((status == 0)) || fail "Base-mode benchmark should succeed without network access: $output"
  [[ "$output" == *'mode=base'* ]] || fail "Base-mode benchmark did not report its mode: $output"
  [[ "$output" == *'common-cached'* && "$output" == *'interactive-cached'* ]] ||
    fail "Base-mode benchmark did not report the expected metrics: $output"
}

test_benchmark_profile_env_var_is_equivalent_to_mode_flag() {
  local output

  output="$(SELFISHELL_BENCHMARK_PROFILE=bogus bash "$ROOT_DIR/scripts/benchmark.sh" 2>&1)" || true

  [[ "$output" == *'must be "base" or "full"'* ]] ||
    fail "SELFISHELL_BENCHMARK_PROFILE was not honored as a --mode equivalent: $output"
}

run_test() {
  local test_name="$1"

  "$test_name"
  printf 'PASS: %s\n' "$test_name"
}

main() {
  local test_name
  local failures=0

  while IFS= read -r test_name; do
    if ! run_test "$test_name"; then
      failures=$((failures + 1))
    fi
  done < <(declare -F | awk '{print $3}' | grep '^test_' | sort)

  if ((failures > 0)); then
    printf '%d test(s) failed\n' "$failures" >&2
    return 1
  fi
}

main "$@"
