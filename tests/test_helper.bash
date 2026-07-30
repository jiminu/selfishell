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
  printf 'PASS: %s\n' "$test_name"
}

runner_status_succeeded() {
  local test_name="$1"
  local status="$2"

  case "$status" in
    0 | 77) return 0 ;;
    99) return 1 ;;
    *)
      printf 'FAIL: %s (exit code %d)\n' "$test_name" "$status" >&2
      return 1
      ;;
  esac
}

run_discovered_tests() {
  local setup_name="${1:-}"
  local teardown_name="${2:-}"
  local test_name pid status

  while IFS= read -r test_name; do
    run_test_isolated "$test_name" "$setup_name" "$teardown_name" &
    pid=$!
    set +e
    wait "$pid"
    status=$?
    set -e

    runner_status_succeeded "$test_name" "$status" || return 1
  done < <(declare -F | awk '{print $3}' | grep '^test_' | sort)
}

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
