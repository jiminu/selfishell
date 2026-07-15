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

confirm_action() {
  local prompt="$1"
  local assume_yes="$2"
  local dry_run="$3"
  local answer

  if [[ "$dry_run" == "1" || "$assume_yes" == "1" ]]; then
    return 0
  fi

  if [[ ! -t 0 ]]; then
    cli_error "Confirmation requires an interactive terminal; use --yes."
    return "$SELFISHELL_EXIT_USAGE"
  fi

  printf '%s [y/N] ' "$prompt"
  IFS= read -r answer
  case "$answer" in
    y | Y | yes | YES) return 0 ;;
    *)
      printf 'Cancelled.\n'
      return "$SELFISHELL_EXIT_ERROR"
      ;;
  esac
}
