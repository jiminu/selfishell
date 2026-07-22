#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

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
  local suite_index=0
  local suite_jobs="${SELFISHELL_SUITE_JOBS:-4}"
  local batch_index
  local failures=0
  local log_root
  local suite
  local suite_path
  local suites=(
    managed_install_test.bash
    release_bootstrap_test.bash
    lifecycle_e2e_test.bash
    common_zsh_test.bash
  )
  local batch_logs=()
  local batch_pids=()

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

  while ((suite_index < ${#suites[@]})); do
    batch_logs=()
    batch_pids=()

    for ((batch_index = 0; batch_index < suite_jobs && suite_index < ${#suites[@]}; batch_index++)); do
      suite="${suites[$suite_index]}"
      batch_logs+=("$log_root/$suite_index.log")
      run_suite "$suite" >"$log_root/$suite_index.log" 2>&1 &
      batch_pids+=("$!")
      suite_index=$((suite_index + 1))
    done

    for batch_index in "${!batch_pids[@]}"; do
      if ! wait "${batch_pids[$batch_index]}"; then
        failures=$((failures + 1))
      fi
      cat "${batch_logs[$batch_index]}"
    done
  done

  if ((failures > 0)); then
    printf '%d test suite(s) failed.\n' "$failures" >&2
    return 1
  fi

  trap - EXIT HUP INT TERM
  rm -rf "$log_root"
}

main "$@"
