#!/usr/bin/env bash

# These constants are consumed by command modules after this file is sourced.
# shellcheck disable=SC2034
SELFISHELL_EXIT_OK=0
# shellcheck disable=SC2034
SELFISHELL_EXIT_ERROR=1
SELFISHELL_EXIT_USAGE=2

cli_error() {
  printf 'selfishell: %s\n' "$*" >&2
}

have_command() {
  command -v "$1" >/dev/null 2>&1
}

require_no_arguments() {
  local command="$1"
  shift

  if (("$#" > 0)); then
    cli_error "$command does not accept arguments"
    return "$SELFISHELL_EXIT_USAGE"
  fi
}
