#!/usr/bin/env bash

run_selfishell() {
  local executable="${SELFISHELL_TEST_CLI:-$ROOT_DIR/bin/selfishell}"

  if [[ -n "${SELFISHELL_TEST_CLI:-}" ]]; then
    case "$executable" in
      /*) ;;
      *)
        printf 'SELFISHELL_TEST_CLI must be an absolute executable path: %s\n' "$executable" >&2
        return 2
        ;;
    esac
    if [[ ! -f "$executable" || ! -x "$executable" ]]; then
      printf 'SELFISHELL_TEST_CLI is not an executable file: %s\n' "$executable" >&2
      return 2
    fi
  fi

  "$executable" "$@"
}
