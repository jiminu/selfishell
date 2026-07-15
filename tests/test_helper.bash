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
  return 1
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
