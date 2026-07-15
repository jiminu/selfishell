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
    [[ -d "$SELFISHELL_RELEASES_DIR/$requested" && -x "$SELFISHELL_RELEASES_DIR/$requested/bin/selfishell" ]] || {
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
  fi
  [[ "$target" != "$current_target" ]] || {
    cli_error "Release is already active: ${target##*/}"
    return 1
  }
  confirm_action "Roll back Selfishell CLI to ${target##*/}?" "$assume_yes" 0 || return

  previous_target="$current_target"
  release_atomic_link "$target" "$SELFISHELL_SHARE_DIR/current"
  release_atomic_link "$previous_target" "$SELFISHELL_SHARE_DIR/previous"
  printf 'Selfishell CLI rolled back to %s.\n' "${target##*/}"
}
