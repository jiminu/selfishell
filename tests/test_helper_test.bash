#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/tests/test_helper.bash"

test_discovered_runner_stops_after_unexpected_failure() {
  local fixture output status

  setup_test_home
  fixture="$TEST_ROOT/unexpected-failure.bash"
  cat >"$fixture" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
source "$SELFISHELL_TEST_HELPER"

test_unexpected_failure() {
  false
  printf 'continued\n' >"$SELFISHELL_TEST_MARKER"
}

test_would_run_after_failure() {
  printf 'second test ran\n' >"$SELFISHELL_TEST_SECOND_MARKER"
}

run_discovered_tests
EOF

  set +e
  output="$(
    SELFISHELL_TEST_HELPER="$ROOT_DIR/tests/test_helper.bash" \
      SELFISHELL_TEST_MARKER="$TEST_ROOT/continued" \
      SELFISHELL_TEST_SECOND_MARKER="$TEST_ROOT/second" \
      bash "$fixture" 2>&1
  )"
  status=$?
  set -e

  [[ "$status" -eq 1 ]] || fail "Unexpected command failure returned $status instead of 1"
  [[ ! -e "$TEST_ROOT/continued" ]] || fail "Unexpected failure continued inside its test"
  [[ ! -e "$TEST_ROOT/second" ]] || fail "Runner continued to the next test after failure"
  [[ "$output" == *'FAIL: test_unexpected_failure (exit code 1)'* ]] ||
    fail "Unexpected command failure did not identify its test: $output"
  [[ "$output" != *'PASS: test_unexpected_failure'* ]] ||
    fail "Unexpected command failure was reported as passing"
  teardown_test_home
}

test_discovered_runner_cleans_up_after_failure() {
  local fixture output status

  setup_test_home
  fixture="$TEST_ROOT/failing-cleanup.bash"
  cat >"$fixture" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
source "$SELFISHELL_TEST_HELPER"

setup_fixture() {
  printf 'created\n' >"$SELFISHELL_TEST_SETUP_MARKER"
}

teardown_fixture() {
  printf 'cleaned\n' >"$SELFISHELL_TEST_CLEANUP_MARKER"
}

test_fails_after_setup() {
  false
}

run_discovered_tests setup_fixture teardown_fixture
EOF

  set +e
  output="$(
    SELFISHELL_TEST_HELPER="$ROOT_DIR/tests/test_helper.bash" \
      SELFISHELL_TEST_SETUP_MARKER="$TEST_ROOT/setup" \
      SELFISHELL_TEST_CLEANUP_MARKER="$TEST_ROOT/cleanup" \
      bash "$fixture" 2>&1
  )"
  status=$?
  set -e

  [[ "$status" -eq 1 ]] || fail "Failing fixture returned $status instead of 1"
  assert_file_content 'created' "$TEST_ROOT/setup"
  assert_file_content 'cleaned' "$TEST_ROOT/cleanup"
  [[ "$output" == *'FAIL: test_fails_after_setup (exit code 1)'* ]] ||
    fail "Failing fixture did not identify its test: $output"
  teardown_test_home
}

test_discovered_runner_reports_skip_without_pass() {
  local fixture output

  setup_test_home
  fixture="$TEST_ROOT/skipped.bash"
  cat >"$fixture" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
source "$SELFISHELL_TEST_HELPER"

test_skipped_fixture() {
  skip 'fixture unavailable'
}

run_discovered_tests
EOF

  output="$(SELFISHELL_TEST_HELPER="$ROOT_DIR/tests/test_helper.bash" bash "$fixture")"

  [[ "$output" == 'SKIP: fixture unavailable' ]] ||
    fail "Skipped fixture emitted unexpected output: $output"
  [[ "$output" != *'PASS:'* ]] || fail "Skipped fixture was also reported as passing"
  teardown_test_home
}

test_discovered_runner_executes_every_test() {
  local fixture output

  setup_test_home
  fixture="$TEST_ROOT/every-test.bash"
  cat >"$fixture" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
source "$SELFISHELL_TEST_HELPER"

test_first_fixture() {
  printf 'first\n' >"$SELFISHELL_TEST_FIRST_MARKER"
}

test_second_fixture() {
  printf 'second\n' >"$SELFISHELL_TEST_SECOND_MARKER"
}

run_discovered_tests
EOF

  output="$(
    SELFISHELL_TEST_HELPER="$ROOT_DIR/tests/test_helper.bash" \
      SELFISHELL_TEST_FIRST_MARKER="$TEST_ROOT/first" \
      SELFISHELL_TEST_SECOND_MARKER="$TEST_ROOT/second" \
      bash "$fixture"
  )"

  assert_file_content 'first' "$TEST_ROOT/first"
  assert_file_content 'second' "$TEST_ROOT/second"
  [[ "$output" == *'PASS: test_first_fixture'* ]] || fail "First fixture test did not run"
  [[ "$output" == *'PASS: test_second_fixture'* ]] || fail "Second fixture test did not run"
  teardown_test_home
}

test_parallel_runner_finishes_batch_and_reports_each_result() {
  local fixture output status

  setup_test_home
  fixture="$TEST_ROOT/parallel.bash"
  cat >"$fixture" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
source "$SELFISHELL_TEST_HELPER"

test_failure() {
  false
}

test_passes() {
  printf 'ran\n' >"$SELFISHELL_TEST_PASS_MARKER"
}

test_skips() {
  skip 'parallel fixture unavailable'
}

run_discovered_tests_parallel 3
EOF

  set +e
  output="$(
    SELFISHELL_TEST_HELPER="$ROOT_DIR/tests/test_helper.bash" \
      SELFISHELL_TEST_PASS_MARKER="$TEST_ROOT/passed" \
      bash "$fixture" 2>&1
  )"
  status=$?
  set -e

  [[ "$status" -eq 1 ]] || fail "Parallel fixture returned $status instead of 1"
  assert_file_content 'ran' "$TEST_ROOT/passed"
  [[ "$output" == *'FAIL: test_failure (exit code 1)'* ]] ||
    fail "Parallel failure did not identify its test: $output"
  [[ "$output" == *'PASS: test_passes'* ]] || fail "Parallel passing test was not reported"
  [[ "$output" == *'SKIP: parallel fixture unavailable'* ]] ||
    fail "Parallel skipped test was not reported"
  [[ "$output" != *'PASS: test_skips'* ]] || fail "Parallel skipped test was also reported as passing"
  teardown_test_home
}

# shellcheck disable=SC2016 # Fixture variables expand in child shells.
test_suite_runner_refills_slots_and_preserves_failure_reports() {
  local fixture output status=0

  setup_test_home
  fixture="$TEST_ROOT/fixture/tests"
  mkdir -p "$fixture"
  cp "$ROOT_DIR/tests/run.bash" "$fixture/run.bash"
  cat >"$fixture/managed_install_test.bash" <<'EOF'
for ((attempt = 0; attempt < 100; attempt++)); do
  [[ ! -f "$HOME/released" ]] || break
  sleep 0.1
done
[[ -f "$HOME/released" ]] || exit 1
: >"$HOME/completed"
EOF
  printf 'exit 1\n' >"$fixture/release_bootstrap_test.bash"
  printf ': >"$HOME/released"\n' >"$fixture/lifecycle_e2e_test.bash"
  printf 'exit 0\n' >"$fixture/common_zsh_test.bash"
  printf ': >"$HOME/last-suite"\n' >"$fixture/extra_test.bash"

  output="$(SELFISHELL_SUITE_JOBS=2 bash "$fixture/run.bash" 2>&1)" || status=$?

  [[ -f "$HOME/completed" ]] || fail "An idle suite slot waited for the whole batch"
  [[ -f "$HOME/last-suite" ]] || fail "A failing suite prevented later suites from running"
  [[ "$status" == 1 && "$output" == *'1 test suite(s) failed: release_bootstrap_test.bash'* ]] ||
    fail "Suite runner lost the failing suite's name or exit status: $output"
  [[ "$(grep -c '^SUITE:' <<<"$output")" == 5 ]] || fail "Suite runner omitted a suite log: $output"

  mkdir "$TEST_ROOT/bin"
  printf '#!/bin/sh\nexit 1\n' >"$TEST_ROOT/bin/xargs"
  chmod +x "$TEST_ROOT/bin/xargs"
  status=0
  output="$(PATH="$TEST_ROOT/bin:$PATH" SELFISHELL_SUITE_JOBS=2 bash "$fixture/run.bash" 2>&1)" || status=$?
  [[ "$status" == 1 && "$output" == *'5 test suite(s) failed:'* ]] ||
    fail "Dispatcher failure was counted as an extra suite: $output"
}

run_discovered_tests '' teardown_test_home
