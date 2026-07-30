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

run_discovered_tests '' teardown_test_home
