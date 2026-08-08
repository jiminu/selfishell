#!/usr/bin/env bash

command_rollback() {
  local assume_yes=0
  local requested=""
  local current_target target previous_target

  while (("$#" > 0)); do
    case "$1" in
      --yes) assume_yes=1 ;;
      help | --help | -h)
        printf 'Usage: selfishell rollback [VERSION] [--yes]\n'
        return
        ;;
      -*)
        cli_error "Unknown rollback option: $1"
        return "$SELFISHELL_EXIT_USAGE"
        ;;
      *)
        [[ -z "$requested" ]] || {
          cli_error "rollback accepts only one version"
          return "$SELFISHELL_EXIT_USAGE"
        }
        requested="${1#v}"
        ;;
    esac
    shift
  done

  release_installation_paths || return
  current_target="$(readlink "$SELFISHELL_SHARE_DIR/current")"
  if [[ -n "$requested" ]]; then
    selfishell_version_is_valid "$requested" || {
      cli_error "Invalid semantic version: $requested"
      return "$SELFISHELL_EXIT_USAGE"
    }
    release_directory_is_valid "$requested" || {
      cli_error "Retained release not found: $requested"
      return "$SELFISHELL_EXIT_ERROR"
    }
    target="releases/$requested"
  else
    [[ -L "$SELFISHELL_SHARE_DIR/previous" ]] || {
      cli_error "No previous release is retained."
      return "$SELFISHELL_EXIT_ERROR"
    }
    target="$(readlink "$SELFISHELL_SHARE_DIR/previous")"
    # `previous` is only ever written by release_atomic_link as
    # "releases/<version>" (see release_install, rollback below), so require
    # it to still have exactly that shape before trusting it: a hand-edited
    # or corrupted link must never repoint `current` at an unvalidated path.
    requested="${target#releases/}"
    if [[ "$target" != "releases/$requested" ]] ||
      ! selfishell_version_is_valid "$requested" ||
      ! release_directory_is_valid "$requested"; then
      cli_error "Retained previous release is invalid: ${target##*/}"
      return "$SELFISHELL_EXIT_ERROR"
    fi
  fi
  [[ "$target" != "$current_target" ]] || {
    printf '%sRelease is already active:%s %s\n' "$SELFISHELL_COLOR_CYAN" "$SELFISHELL_COLOR_RESET" "${target##*/}"
    return 0
  }
  confirm_action "Roll back Selfishell CLI to ${target##*/}?" "$assume_yes" 0 || return

  previous_target="$current_target"
  release_atomic_link "$target" "$SELFISHELL_SHARE_DIR/current" || {
    cli_error "Failed to roll back to ${target##*/}."
    return "$SELFISHELL_EXIT_ERROR"
  }
  # A failure here only loses the rollback link, not the rollback itself, so
  # warn and continue rather than aborting an otherwise-successful rollback.
  if ! release_atomic_link "$previous_target" "$SELFISHELL_SHARE_DIR/previous"; then
    cli_warn "Failed to update the previous release link; continuing."
  fi
  printf '%sSelfishell CLI rolled back to %s.%s\n' "$SELFISHELL_COLOR_GREEN" "${target##*/}" "$SELFISHELL_COLOR_RESET"
}
