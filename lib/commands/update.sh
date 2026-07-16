#!/usr/bin/env bash

print_update_help() {
  cat <<'EOF'
Usage:
  selfishell update [--cli-only | --tools-only] [--version VERSION]
                     [--dry-run] [--yes]

By default, update approved tools and managed configuration first, then update
the Selfishell CLI release. Use --cli-only or --tools-only to limit the scope.
--version selects an exact CLI release and cannot be used with --tools-only.
EOF
}

update_tools_and_configuration() {
  local assume_yes="$1"
  local dry_run="$2"
  local require_configuration="$3"
  local profile platform profile_platform dependency_platform architecture index package ghostty_enabled=0

  selfishell_initialize_paths
  if [[ ! -r "$SELFISHELL_STATE_DIR/profile" ]]; then
    if [[ "$require_configuration" == 1 ]]; then
      cli_error "Selfishell configuration is not installed."
      return "$SELFISHELL_EXIT_ERROR"
    fi
    printf 'Selfishell configuration is not installed; skipping tools and configuration.\n'
    return
  fi
  profile="$(<"$SELFISHELL_STATE_DIR/profile")"
  [[ ! -r "$SELFISHELL_STATE_DIR/ghostty" ]] || ghostty_enabled="$(<"$SELFISHELL_STATE_DIR/ghostty")"
  [[ "$ghostty_enabled" == "1" ]] || ghostty_enabled=0
  platform="$(detect_platform)"
  architecture="$(detect_architecture)"
  case "$platform" in
    ubuntu | ubuntu-wsl)
      dependency_platform=linux
      profile_platform=ubuntu
      ;;
    *)
      dependency_platform="$platform"
      profile_platform="$platform"
      ;;
  esac
  profile_load "$profile" "${SELFISHELL_LOCAL_PROFILE:-}"
  confirm_action "Update approved tools and $profile configuration?" "$assume_yes" "$dry_run" || return

  for ((index = 0; index < ${#PROFILE_PACKAGES[@]}; index++)); do
    [[ "${PROFILE_MANAGERS[$index]}" == direct ]] || continue
    [[ "${PROFILE_PLATFORMS[$index]}" == all || "${PROFILE_PLATFORMS[$index]}" == "$profile_platform" ]] || continue
    package="${PROFILE_PACKAGES[$index]}"
    if [[ "$dry_run" == 1 ]]; then
      dependency_load "$package" "$dependency_platform" "$architecture"
      printf 'Would ensure approved dependency: %s %s\n' "$package" "$DEPENDENCY_VERSION"
    else
      dependency_install "$package" "$dependency_platform" "$architecture"
    fi
  done
  install_managed_configuration "$platform" "$dry_run" "$ghostty_enabled"
  install_vim_plugins "$dry_run"
  [[ "$dry_run" == 1 ]] && printf 'Tool/configuration dry run complete.\n' || printf 'Selfishell tools and configuration updated.\n'
}

update_cli_release() {
  local version="$1"
  local assume_yes="$2"
  local dry_run="$3"

  if [[ -z "$version" ]]; then
    version="$(release_latest_version)" || {
      cli_error "Unable to determine the latest Selfishell release. Use --version VERSION to select one."
      return "$SELFISHELL_EXIT_ERROR"
    }
  fi
  [[ "$version" =~ ^[0-9A-Za-z][0-9A-Za-z.-]*$ ]] || {
    cli_error "Invalid version: $version"
    return "$SELFISHELL_EXIT_USAGE"
  }
  if [[ -r "$SELFISHELL_ROOT/VERSION" && "$(<"$SELFISHELL_ROOT/VERSION")" == "$version" ]]; then
    printf 'Selfishell CLI is already at %s.\n' "$version"
    return
  fi
  if [[ "$dry_run" == 1 ]]; then
    printf 'Would update Selfishell CLI to %s.\n' "$version"
    return
  fi
  confirm_action "Update Selfishell CLI to $version?" "$assume_yes" 0 || return
  release_install "$version"
}

command_update() {
  local assume_yes=0
  local dry_run=0
  local mode=all
  local version=""

  while (("$#" > 0)); do
    case "$1" in
      --cli-only)
        [[ "$mode" != tools ]] || {
          cli_error "--cli-only and --tools-only cannot be used together"
          return "$SELFISHELL_EXIT_USAGE"
        }
        mode=cli
        ;;
      --tools-only)
        [[ "$mode" != cli ]] || {
          cli_error "--cli-only and --tools-only cannot be used together"
          return "$SELFISHELL_EXIT_USAGE"
        }
        mode=tools
        ;;
      --version)
        shift
        (("$#" > 0)) || {
          cli_error "--version requires a value"
          return "$SELFISHELL_EXIT_USAGE"
        }
        version="${1#v}"
        ;;
      --dry-run) dry_run=1 ;;
      --yes) assume_yes=1 ;;
      help | --help | -h)
        print_update_help
        return
        ;;
      *)
        cli_error "Unknown update option: $1"
        return "$SELFISHELL_EXIT_USAGE"
        ;;
    esac
    shift
  done

  [[ "$mode" != tools || -z "$version" ]] || {
    cli_error "--version cannot be used with --tools-only"
    return "$SELFISHELL_EXIT_USAGE"
  }

  if [[ "$mode" != cli ]]; then
    if [[ "$mode" == tools ]]; then
      update_tools_and_configuration "$assume_yes" "$dry_run" 1 || return
    else
      update_tools_and_configuration "$assume_yes" "$dry_run" 0 || return
    fi
  fi

  if [[ "$mode" != tools ]]; then
    update_cli_release "$version" "$assume_yes" "$dry_run"
  fi
}
