#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# The EXIT trap below runs after main() returns, so this cannot be one of its
# locals: cleaning up after a failed run would fail on an unbound variable and
# leave the log directory behind.
log_root=""

run_suite() {
  local suite="$1"
  local started_at finished_at
  local status=0

  started_at="$(date +%s)"
  bash "$ROOT_DIR/tests/$suite" || status=$?
  finished_at="$(date +%s)"
  printf 'SUITE: %s (%ss)\n' "$suite" "$((finished_at - started_at))"
  return "$status"
}

main() {
  local suite_jobs="${SELFISHELL_SUITE_JOBS:-4}"
  local failures=0 result=0
  local suite
  local suite_path
  local failed_suites=()
  local suites=(
    managed_install_test.bash
    release_bootstrap_test.bash
    lifecycle_e2e_test.bash
    common_zsh_test.bash
  )

  case "$suite_jobs" in
    '' | *[!0-9]* | 0)
      printf 'SELFISHELL_SUITE_JOBS must be a positive integer.\n' >&2
      return 2
      ;;
  esac

  for suite_path in "$ROOT_DIR"/tests/*_test.bash; do
    suite="${suite_path##*/}"
    case "$suite" in
      managed_install_test.bash | release_bootstrap_test.bash | lifecycle_e2e_test.bash | common_zsh_test.bash) ;;
      *) suites+=("$suite") ;;
    esac
  done

  log_root="$(mktemp -d "${TMPDIR:-/tmp}/selfishell-suite-test.XXXXXX")"
  trap 'rm -rf "$log_root"' EXIT HUP INT TERM
  printf 'Running %d test suites (jobs: %d)\n' "${#suites[@]}" "$suite_jobs"

  export ROOT_DIR log_root
  export -f run_suite
  # shellcheck disable=SC2016 # Expand variables in each worker.
  printf '%s\0' "${suites[@]}" | xargs -0 -n 1 -P "$suite_jobs" bash -c '
    status=0
    run_suite "$1" >"$log_root/$1.log" 2>&1 || status=$?
    printf "%s\n" "$status" >"$log_root/$1.status"
  ' _ || result=1

  for suite in "${suites[@]}"; do
    [[ ! -f "$log_root/$suite.log" ]] || cat "$log_root/$suite.log"
    if [[ ! -f "$log_root/$suite.status" || "$(<"$log_root/$suite.status")" != 0 ]]; then
      failures=$((failures + 1))
      failed_suites+=("$suite")
    fi
  done

  if ((failures > 0)); then
    printf '%d test suite(s) failed: %s\n' "$failures" "${failed_suites[*]}" >&2
    return 1
  fi

  trap - EXIT HUP INT TERM
  rm -rf "$log_root"
  return "$result"
}

main "$@"
