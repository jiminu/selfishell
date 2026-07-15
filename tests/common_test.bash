#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

source "$ROOT_DIR/tests/test_helper.bash"
source "$ROOT_DIR/common/common.sh"

test_backup_preserves_same_second_files() {
  local target="$HOME/config"
  local backups=()

  printf 'first' >"$target"
  backup_path "$target"
  printf 'second' >"$target"
  backup_path "$target"

  backups=("$HOME"/config.backup.*)
  if [[ "${#backups[@]}" -ne 2 ]]; then
    fail "Expected two distinct backups"
    return
  fi
  assert_file_content first "${backups[0]}"
  assert_file_content second "${backups[1]}"
}

test_link_file_creates_link() {
  local source_file="$TEST_ROOT/source"
  local target_file="$HOME/.config/example/config"

  printf 'managed' >"$source_file"
  link_file "$source_file" "$target_file"

  assert_symlink_to "$source_file" "$target_file"
}

test_link_file_is_idempotent() {
  local source_file="$TEST_ROOT/source"
  local target_file="$HOME/config"

  printf 'managed' >"$source_file"
  ln -s "$source_file" "$target_file"
  link_file "$source_file" "$target_file"

  assert_symlink_to "$source_file" "$target_file"
  [[ -z "$(find "$HOME" -maxdepth 1 -name 'config.backup.*' -print -quit)" ]] ||
    fail "An unchanged link must not create a backup"
}

test_link_file_backs_up_regular_file() {
  local source_file="$TEST_ROOT/source"
  local target_file="$HOME/config"
  local backup

  printf 'managed' >"$source_file"
  printf 'original' >"$target_file"
  link_file "$source_file" "$target_file"
  backup="$(find "$HOME" -maxdepth 1 -type f -name 'config.backup.*' -print -quit)"

  assert_file_content original "$backup"
  assert_symlink_to "$source_file" "$target_file"
}

test_link_file_backs_up_directory() {
  local source_file="$TEST_ROOT/source"
  local target_file="$HOME/config"
  local backup

  printf 'managed' >"$source_file"
  mkdir -p "$target_file"
  printf 'original' >"$target_file/value"
  link_file "$source_file" "$target_file"
  backup="$(find "$HOME" -maxdepth 1 -type d -name 'config.backup.*' -print -quit)"

  assert_file_content original "$backup/value"
  assert_symlink_to "$source_file" "$target_file"
}

test_link_file_replaces_dangling_link() {
  local source_file="$TEST_ROOT/source"
  local target_file="$HOME/config"
  local backup

  printf 'managed' >"$source_file"
  ln -s "$TEST_ROOT/missing" "$target_file"
  link_file "$source_file" "$target_file"
  backup="$(find "$HOME" -maxdepth 1 -type l -name 'config.backup.*' -print -quit)"

  assert_symlink_to "$TEST_ROOT/missing" "$backup"
  assert_symlink_to "$source_file" "$target_file"
}

run_test() {
  local test_name="$1"

  setup_test_home
  trap teardown_test_home RETURN
  "$test_name"
  trap - RETURN
  teardown_test_home
  printf 'PASS: %s\n' "$test_name"
}

main() {
  local test_name
  local failures=0

  while IFS= read -r test_name; do
    if ! run_test "$test_name"; then
      failures=$((failures + 1))
    fi
  done < <(declare -F | awk '{print $3}' | grep '^test_' | sort)

  if ((failures > 0)); then
    printf '%d test(s) failed\n' "$failures" >&2
    return 1
  fi
}

main "$@"
