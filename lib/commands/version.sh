#!/usr/bin/env bash

command_version() {
  require_no_arguments version "$@" || return

  local version_file="$SELFISHELL_ROOT/VERSION"
  local version

  if [[ ! -r "$version_file" ]]; then
    cli_error "Version file not found: $version_file"
    return "$SELFISHELL_EXIT_ERROR"
  fi

  version="$(<"$version_file")"
  printf 'selfishell %s\n' "$version"
}
