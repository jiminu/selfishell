#!/usr/bin/env bash

set -euo pipefail

TEST_ROOT=""

setup_test_home() {
  TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/selfishell-test.XXXXXX")"
  export HOME="$TEST_ROOT/home"
  mkdir -p "$HOME"
}

teardown_test_home() {
  if [[ -n "$TEST_ROOT" && -d "$TEST_ROOT" ]]; then
    rm -rf "$TEST_ROOT"
  fi
}

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 99
}

skip() {
  printf 'SKIP: %s\n' "$*"
  exit 77
}

run_test_isolated() {
  local test_name="$1"
  local setup_name="${2:-}"
  local teardown_name="${3:-}"
  local completion_marker="${4:-}"

  set -Eeuo pipefail
  if [[ -n "$teardown_name" ]]; then
    # shellcheck disable=SC2064 # Resolve the selected hook before the test runs.
    trap "$teardown_name" EXIT
  fi
  if [[ -n "$setup_name" ]]; then
    "$setup_name"
  fi

  "$test_name"

  if [[ -n "$teardown_name" ]]; then
    trap - EXIT
    "$teardown_name"
  fi
  [[ -z "$completion_marker" ]] || : >"$completion_marker"
  printf 'PASS: %s\n' "$test_name"
}

# Exit 0 alone is not a pass: a test process that ends early (an `exit 0`,
# or Bash 3.2 replacing a backgrounded function with `command <tool>`)
# never reaches the completion marker.
runner_status_succeeded() {
  local test_name="$1"
  local status="$2"
  local completion_marker="${3:-}"

  case "$status" in
    0)
      [[ -z "$completion_marker" || -e "$completion_marker" ]] && return 0
      printf 'FAIL: %s (exited before completing)\n' "$test_name" >&2
      return 1
      ;;
    77) return 0 ;;
    99) return 1 ;;
    *)
      printf 'FAIL: %s (exit code %d)\n' "$test_name" "$status" >&2
      return 1
      ;;
  esac
}

run_discovered_tests() (
  local setup_name="${1:-}"
  local teardown_name="${2:-}"
  local test_name pid status marker_root

  marker_root="$(mktemp -d "${TMPDIR:-/tmp}/selfishell-test-markers.XXXXXX")"
  trap 'rm -rf "$marker_root"' EXIT HUP INT TERM

  while IFS= read -r test_name; do
    # The subshell keeps Bash 3.2 from exec-replacing the backgrounded test.
    (run_test_isolated "$test_name" "$setup_name" "$teardown_name" "$marker_root/$test_name") &
    pid=$!
    set +e
    wait "$pid"
    status=$?
    set -e

    runner_status_succeeded "$test_name" "$status" "$marker_root/$test_name" || return 1
  done < <(declare -F | awk '{print $3}' | grep '^test_' | sort)
)

run_discovered_tests_parallel() (
  local test_jobs="$1"
  local setup_name="${2:-}"
  local teardown_name="${3:-}"
  local test_index=0 batch_index failures=0 status
  local test_name log_root
  local test_list=()
  local batch_logs=()
  local batch_pids=()
  local batch_tests=()
  local batch_markers=()

  case "$test_jobs" in
    '' | *[!0-9]* | 0)
      printf 'Test jobs must be a positive integer.\n' >&2
      return 2
      ;;
  esac

  while IFS= read -r test_name; do
    test_list+=("$test_name")
  done < <(declare -F | awk '{print $3}' | grep '^test_' | sort)

  log_root="$(mktemp -d "${TMPDIR:-/tmp}/selfishell-parallel-test.XXXXXX")"
  trap 'rm -rf "$log_root"' EXIT HUP INT TERM
  printf 'Total tests found: %d (jobs: %d)\n' "${#test_list[@]}" "$test_jobs"

  while ((test_index < ${#test_list[@]})); do
    batch_logs=()
    batch_pids=()
    batch_tests=()
    batch_markers=()

    for ((batch_index = 0; batch_index < test_jobs && test_index < ${#test_list[@]}; batch_index++)); do
      test_name="${test_list[$test_index]}"
      batch_logs+=("$log_root/$test_index.log")
      batch_tests+=("$test_name")
      batch_markers+=("$log_root/$test_index.done")
      (run_test_isolated "$test_name" "$setup_name" "$teardown_name" "$log_root/$test_index.done") \
        >"$log_root/$test_index.log" 2>&1 &
      batch_pids+=("$!")
      test_index=$((test_index + 1))
    done

    for batch_index in "${!batch_pids[@]}"; do
      set +e
      wait "${batch_pids[$batch_index]}"
      status=$?
      set -e
      cat "${batch_logs[$batch_index]}"
      if ! runner_status_succeeded "${batch_tests[$batch_index]}" "$status" "${batch_markers[$batch_index]}"; then
        failures=$((failures + 1))
      fi
    done
  done

  ((failures == 0)) || return 1
)

assert_file_content() {
  local expected="$1"
  local file="$2"
  local actual

  [[ -f "$file" ]] || fail "Expected file to exist: $file"
  actual="$(<"$file")"
  [[ "$actual" == "$expected" ]] ||
    fail "Expected '$file' to contain '$expected', got '$actual'"
}

assert_symlink_to() {
  local expected="$1"
  local link="$2"

  [[ -L "$link" ]] || fail "Expected symbolic link: $link"
  [[ "$(readlink "$link")" == "$expected" ]] ||
    fail "Expected '$link' to point to '$expected'"
}

fixture_sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}
