#!/usr/bin/env bash

print_self_update_help() {
  cat <<'EOF'
Usage:
  selfishell self-update [--version VERSION] [--yes]

Without --version, the version published in the latest release is installed.
EOF
}

command_self_update() {
  local version=""
  local assume_yes=0

  while (("$#" > 0)); do
    case "$1" in
      --version)
        shift
        (("$#" > 0)) || {
          cli_error "--version requires a value"
          return "$SELFISHELL_EXIT_USAGE"
        }
        version="${1#v}"
        ;;
      --yes) assume_yes=1 ;;
      help | --help | -h)
        print_self_update_help
        return
        ;;
      *)
        cli_error "Unknown self-update option: $1"
        return "$SELFISHELL_EXIT_USAGE"
        ;;
    esac
    shift
  done

  if [[ -z "$version" ]]; then
    version="$(curl -fsSL "$(release_root_url)/latest/download/VERSION")"
    version="${version#v}"
  fi
  [[ "$version" =~ ^[0-9A-Za-z][0-9A-Za-z.-]*$ ]] || {
    cli_error "Invalid version: $version"
    return "$SELFISHELL_EXIT_USAGE"
  }
  if [[ -r "$SELFISHELL_ROOT/VERSION" && "$(<"$SELFISHELL_ROOT/VERSION")" == "$version" ]]; then
    printf 'Selfishell CLI is already at %s.\n' "$version"
    return
  fi
  confirm_action "Update Selfishell CLI to $version?" "$assume_yes" 0 || return
  release_install "$version"
}
