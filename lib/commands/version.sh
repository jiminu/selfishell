#!/usr/bin/env bash

command_version() {
  local version_file="$SELFISHELL_ROOT/VERSION"
  local version

  case "${1:-}" in
    "") ;;
    --available)
      (($# == 1)) || {
        cli_error "Usage: selfishell version [--available]"
        return "$SELFISHELL_EXIT_USAGE"
      }
      release_latest_version || {
        cli_error "Unable to determine the latest Selfishell release."
        return "$SELFISHELL_EXIT_ERROR"
      }
      return
      ;;
    help | --help | -h)
      printf 'Usage: selfishell version [--available]\n'
      return
      ;;
    *)
      cli_error "Usage: selfishell version [--available]"
      return "$SELFISHELL_EXIT_USAGE"
      ;;
  esac

  if [[ ! -r "$version_file" ]]; then
    cli_error "Version file not found: $version_file"
    return "$SELFISHELL_EXIT_ERROR"
  fi

  version="$(<"$version_file")"
  printf 'selfishell %s\n' "$version"
}
