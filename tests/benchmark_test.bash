#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/tests/test_helper.bash"

# Argument parsing and --mode base only. --mode full provisions real tools over
# the network, so it is run manually rather than made a suite dependency.

assert_benchmark_early_exit() {
  local expected_status="$1" expected_message="$2"
  local status=0 output leftover
  shift 2

  setup_test_home
  mkdir -p "$TEST_ROOT/tmp"
  output="$(TMPDIR="$TEST_ROOT/tmp" "$@" 2>&1)" || status=$?

  [[ "$status" -eq "$expected_status" ]] || fail "Expected exit $expected_status, got $status: $output"
  [[ "$output" == *"$expected_message"* ]] || fail "Missing message '$expected_message': $output"
  leftover="$(find "$TEST_ROOT/tmp" -mindepth 1)"
  [[ -z "$leftover" ]] || fail "An early-exit path left temporary files behind: $leftover"
  teardown_test_home
}

test_benchmark_rejects_unknown_mode() {
  assert_benchmark_early_exit 2 'must be "base" or "full"' \
    bash "$ROOT_DIR/scripts/benchmark.sh" --mode bogus
}

test_benchmark_rejects_missing_mode_value() {
  assert_benchmark_early_exit 2 '--mode requires base or full' \
    bash "$ROOT_DIR/scripts/benchmark.sh" --mode
}

test_benchmark_rejects_unknown_option() {
  assert_benchmark_early_exit 2 'Unknown option: --bogus-flag' \
    bash "$ROOT_DIR/scripts/benchmark.sh" --bogus-flag
}

test_benchmark_help_documents_both_modes() {
  assert_benchmark_early_exit 0 '[--mode base|full]' \
    bash "$ROOT_DIR/scripts/benchmark.sh" --help
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

test_benchmark_rejects_missing_retained_zsh_module() {
  local checkout output status=0

  setup_test_home
  checkout="$TEST_ROOT/checkout"
  mkdir -p "$checkout/scripts"
  cp "$ROOT_DIR/scripts/benchmark.sh" "$checkout/scripts/"
  cp -R "$ROOT_DIR/config" "$checkout/config"
  mv "$checkout/config/shared/zsh/aliases.zsh" "$TEST_ROOT/aliases.zsh"

  output="$(SELFISHELL_BENCHMARK_ITERATIONS=1 \
    bash "$checkout/scripts/benchmark.sh" --mode base 2>&1)" || status=$?

  ((status != 0)) || fail "Benchmark accepted a fixture missing aliases.zsh: $output"
  [[ "$output" == *'Missing benchmark Zsh module: aliases.zsh'* ]] ||
    fail "Benchmark did not identify the missing Zsh module: $output"
  teardown_test_home
}

test_benchmark_base_mode_ignores_ambient_integrations() {
  local fake_bin output profile_file real_home tool

  real_home="$HOME"
  setup_test_home
  fake_bin="$TEST_ROOT/bin"
  profile_file="$TEST_ROOT/base.zprof"
  mkdir -p "$fake_bin"
  for tool in starship fzf zoxide; do
    cat >"$fake_bin/$tool" <<'EOF'
#!/bin/sh
printf ':\n'
EOF
    chmod +x "$fake_bin/$tool"
  done

  output="$(PATH="$fake_bin:$PATH" SELFISHELL_BENCHMARK_ITERATIONS=1 \
    SELFISHELL_BENCHMARK_ZPROF_FILE="$profile_file" \
    bash "$ROOT_DIR/scripts/benchmark.sh" --mode base)"

  [[ "$output" == *'starship=absent fzf=absent zoxide=absent zinit=absent'* ]] ||
    fail "Base mode inherited optional integrations from PATH: $output"
  ! grep -qi 'starship' "$profile_file" || fail "Base profiler executed ambient Starship"
  ! grep -Fq "$real_home/.config/mise" "$profile_file" ||
    fail "Benchmark profiler read the developer mise configuration"
  teardown_test_home
}

test_benchmark_ignores_ambient_xdg_data_home() {
  local ambient_data sentinel

  setup_test_home
  ambient_data="$TEST_ROOT/ambient-data"
  sentinel="$TEST_ROOT/ambient-zinit-loaded"
  mkdir -p "$ambient_data/zinit/zinit.git"
  cat >"$ambient_data/zinit/zinit.git/zinit.zsh" <<EOF
print -r -- loaded >"$sentinel"
EOF

  XDG_DATA_HOME="$ambient_data" SELFISHELL_BENCHMARK_ITERATIONS=1 \
    bash "$ROOT_DIR/scripts/benchmark.sh" --mode base >/dev/null

  [[ ! -e "$sentinel" ]] || fail "Benchmark sourced Zinit from the caller's XDG data directory"
  teardown_test_home
}

test_benchmark_profile_env_var_is_equivalent_to_mode_flag() {
  assert_benchmark_early_exit 2 'must be "base" or "full"' \
    env SELFISHELL_BENCHMARK_PROFILE=bogus bash "$ROOT_DIR/scripts/benchmark.sh"
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

run_discovered_tests '' teardown_test_home
