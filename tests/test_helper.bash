#!/usr/bin/env bash

set -euo pipefail

TEST_ROOT=""

setup_test_home() {
  TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/selfishell-test.XXXXXX")"
  export HOME="$TEST_ROOT/home"
  mkdir -p "$HOME"
  source "$ROOT_DIR/lib/paths.sh"
  selfishell_initialize_paths
  export SELFISHELL_CONFIG_DIR SELFISHELL_STATE_DIR SELFISHELL_CACHE_DIR SELFISHELL_RESOURCE_STATE_DIR
}

teardown_test_home() {
  if [[ -n "$TEST_ROOT" && -d "$TEST_ROOT" ]]; then
    rm -rf "$TEST_ROOT"
  fi
}

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
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
