#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

source "$ROOT_DIR/tests/test_helper.bash"

test_help_and_local_version_skip_full_cli_loading() {
  local help_trace
  local version_trace

  help_trace="$(bash -x "$ROOT_DIR/bin/selfishell" help 2>&1 >/dev/null)"
  version_trace="$(bash -x "$ROOT_DIR/bin/selfishell" version 2>&1 >/dev/null)"

  [[ "$help_trace" != *'/lib/paths.sh'* ]] || fail "Help eagerly loaded the full CLI"
  [[ "$version_trace" != *'/lib/paths.sh'* ]] || fail "Local version eagerly loaded the full CLI"
}

run_discovered_tests '' teardown_test_home
