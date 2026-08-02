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

test_benchmark_rejects_missing_mode_value() {
  local status=0
  local output

  output="$(bash "$ROOT_DIR/scripts/benchmark.sh" --mode 2>&1)" || status=$?

  [[ "$status" -eq 2 ]] || fail "A --mode with no following value should exit 2 (got $status)"
  [[ "$output" == *'--mode requires base or full'* ]] || fail "A missing --mode value did not explain usage: $output"
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

test_benchmark_base_mode_ignores_ambient_integrations() {
  local fake_bin output tool

  setup_test_home
  fake_bin="$TEST_ROOT/bin"
  mkdir -p "$fake_bin"
  for tool in starship fzf zoxide; do
    cat >"$fake_bin/$tool" <<'EOF'
#!/bin/sh
printf ':\n'
EOF
    chmod +x "$fake_bin/$tool"
  done

  output="$(PATH="$fake_bin:$PATH" SELFISHELL_BENCHMARK_ITERATIONS=1 \
    bash "$ROOT_DIR/scripts/benchmark.sh" --mode base)"

  [[ "$output" == *'starship=absent fzf=absent zoxide=absent zinit=absent'* ]] ||
    fail "Base mode inherited optional integrations from PATH: $output"
  teardown_test_home
}

test_benchmark_profile_env_var_is_equivalent_to_mode_flag() {
  local output

  output="$(SELFISHELL_BENCHMARK_PROFILE=bogus bash "$ROOT_DIR/scripts/benchmark.sh" 2>&1)" || true

  [[ "$output" == *'must be "base" or "full"'* ]] ||
    fail "SELFISHELL_BENCHMARK_PROFILE was not honored as a --mode equivalent: $output"
}

test_benchmark_writes_opt_in_zprof_report() {
  local profile_file

  setup_test_home
  profile_file="$TEST_ROOT/startup.zprof"

  SELFISHELL_BENCHMARK_ITERATIONS=1 \
    SELFISHELL_BENCHMARK_ZPROF_FILE="$profile_file" \
    bash "$ROOT_DIR/scripts/benchmark.sh" --mode base >/dev/null

  [[ -s "$profile_file" ]] || fail "Benchmark did not write the requested zprof report"
  grep -Fq 'num  calls' "$profile_file" || fail "Benchmark output is not a zprof report"
  teardown_test_home
}

# Argument parsing and mode validation must happen before benchmark.sh
# creates its temp directory, so every early-exit path above (missing
# --mode value, unknown --mode, unknown option, an invalid env-var mode)
# leaves nothing behind. TMPDIR is scoped to an isolated sandbox so this
# check can't be confused by unrelated temp entries on the real system.
test_benchmark_early_exit_paths_leave_no_temp_directory() {
  local leftover

  setup_test_home
  trap teardown_test_home EXIT

  TMPDIR="$TEST_ROOT" bash "$ROOT_DIR/scripts/benchmark.sh" --mode >/dev/null 2>&1 || true
  TMPDIR="$TEST_ROOT" bash "$ROOT_DIR/scripts/benchmark.sh" --mode bogus >/dev/null 2>&1 || true
  TMPDIR="$TEST_ROOT" bash "$ROOT_DIR/scripts/benchmark.sh" --bogus-flag >/dev/null 2>&1 || true
  TMPDIR="$TEST_ROOT" bash "$ROOT_DIR/scripts/benchmark.sh" --help >/dev/null 2>&1 || true
  TMPDIR="$TEST_ROOT" SELFISHELL_BENCHMARK_PROFILE=bogus bash "$ROOT_DIR/scripts/benchmark.sh" >/dev/null 2>&1 || true

  leftover="$(find "$TEST_ROOT" -maxdepth 1 -name 'selfishell-benchmark.*')"
  [[ -z "$leftover" ]] || fail "An early-exit path left a benchmark temp directory behind: $leftover"

  teardown_test_home
}

run_discovered_tests '' teardown_test_home
