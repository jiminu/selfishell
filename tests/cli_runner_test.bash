#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/tests/test_helper.bash"
source "$ROOT_DIR/tests/cli_runner.bash"

test_default_runner_uses_source_cli() {
  local output
  output="$(SELFISHELL_TEST_CLI='' run_selfishell version)"
  [[ "$output" == 'selfishell development' ]] || fail "Runner did not use the source CLI: $output"
}

test_invalid_override_does_not_fall_back() {
  local override status
  mkdir -p "$TEST_ROOT/a directory"
  printf '#!/bin/sh\nexit 0\n' >"$TEST_ROOT/not executable"

  for override in relative "$TEST_ROOT/a directory" "$TEST_ROOT/missing" "$TEST_ROOT/not executable"; do
    status=0
    SELFISHELL_TEST_CLI="$override" run_selfishell version >"$TEST_ROOT/stdout" 2>"$TEST_ROOT/stderr" || status=$?
    [[ "$status" -ne 0 ]] || fail "Runner accepted invalid override: $override"
    [[ ! -s "$TEST_ROOT/stdout" ]] || fail "Runner ran a fallback for: $override"
    grep -Fq 'SELFISHELL_TEST_CLI' "$TEST_ROOT/stderr" || fail "Runner did not explain invalid override: $override"
  done
}

test_runner_executes_override_with_its_shebang_and_preserves_io() {
  local candidate="$TEST_ROOT/other shell"
  local status
  cat >"$candidate" <<'EOF'
#!/bin/sh
if [ -n "${BASH_VERSION+set}" ]; then exit 88; fi
printf 'arg=%s\n' "$1"
IFS= read -r input
printf 'input=%s\n' "$input"
printf 'error=%s\n' "$2" >&2
exit 23
EOF
  chmod +x "$candidate"

  status=0
  printf 'stdin with spaces\n' | SELFISHELL_TEST_CLI="$candidate" run_selfishell 'arg with spaces' 'stderr with spaces' >"$TEST_ROOT/stdout" 2>"$TEST_ROOT/stderr" || status=$?
  [[ "$status" -eq 23 ]] || fail "Runner did not preserve exit status: $status"
  assert_file_content $'arg=arg with spaces\ninput=stdin with spaces' "$TEST_ROOT/stdout"
  assert_file_content 'error=stderr with spaces' "$TEST_ROOT/stderr"
}

run_discovered_tests setup_test_home teardown_test_home
