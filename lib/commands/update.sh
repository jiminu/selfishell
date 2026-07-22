#!/usr/bin/env bash

print_update_help() {
  cat <<'EOF'
Usage:
  selfishell update [--cli-only | --tools-only] [--version VERSION]
                     [--dry-run] [--yes]

By default, update the Selfishell CLI release first, then synchronize all
profile packages, approved tools, and managed configuration. Use --cli-only or
--tools-only to limit the scope.
--version selects an exact CLI release and cannot be used with --tools-only.
EOF
}

update_tools_and_configuration() {
  local assume_yes="$1"
  local dry_run="$2"
  local require_configuration="$3"
  local profile platform ghostty_enabled=0

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
  managed_preflight_zsh_loader || return
  platform="$(detect_platform)"

  if [[ "$platform" == "macos" && "$ghostty_enabled" == "1" ]]; then
    managed_preflight_block_target user-ghostty \
      "${XDG_CONFIG_HOME:-$HOME/.config}/ghostty/config.ghostty" || return
  fi

  profile_load "$profile" "${SELFISHELL_LOCAL_PROFILE:-}"
  confirm_action "Synchronize $profile profile packages and configuration?" "$assume_yes" "$dry_run" || return

  packages_install_profile "$platform" "$dry_run"
  if [[ "$platform" == "macos" && "$ghostty_enabled" == "1" ]]; then
    homebrew_install_packages optional cask "$dry_run" ghostty
  fi
  install_managed_configuration "$platform" "$dry_run" "$profile" "$ghostty_enabled" "$assume_yes"
  if [[ "$profile" == "developer" ]]; then
    install_neovim_plugins "$dry_run" || return
  fi
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
  selfishell_version_is_valid "$version" || {
    cli_error "Invalid semantic version: $version"
    return "$SELFISHELL_EXIT_USAGE"
  }
  if [[ -r "$SELFISHELL_ROOT/VERSION" && "$(<"$SELFISHELL_ROOT/VERSION")" == "$version" ]]; then
    printf 'Selfishell CLI is already at %s; skipping CLI update.\n' "$version"
    return
  fi
  if [[ "$dry_run" == 1 ]]; then
    printf 'Would update Selfishell CLI to %s.\n' "$version"
    return
  fi
  confirm_action "Update Selfishell CLI to $version?" "$assume_yes" 0 || return
  release_install "$version"
  SELFISHELL_CLI_UPDATED=1
}

continue_update_with_new_cli() {
  local assume_yes="$1"
  local arguments=(update --continue-after-cli-update)

  [[ "$assume_yes" == 0 ]] || arguments+=(--yes)
  exec "$SELFISHELL_SHARE_DIR/current/bin/selfishell" "${arguments[@]}"
}

command_update() {
  local assume_yes=0
  local dry_run=0
  local mode=all
  local version=""
  local continuation=0

  SELFISHELL_CLI_UPDATED=0

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
      --continue-after-cli-update)
        continuation=1
        mode=tools
        ;;
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

  if [[ "$mode" != tools ]]; then
    # Keep update operations out of conditional command contexts. Bash disables
    # errexit inside functions used by `if`, `!`, `&&`, or `||`.
    update_cli_release "$version" "$assume_yes" "$dry_run"
    if [[ "$mode" == all && "$dry_run" == 0 && "$SELFISHELL_CLI_UPDATED" == 1 ]]; then
      continue_update_with_new_cli "$assume_yes"
    fi
  fi

  if [[ "$mode" != cli ]]; then
    if [[ "$mode" == tools && "$continuation" == 0 ]]; then
      update_tools_and_configuration "$assume_yes" "$dry_run" 1
    else
      update_tools_and_configuration "$assume_yes" "$dry_run" 0
    fi
  fi
}
