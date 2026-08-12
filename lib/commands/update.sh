#!/usr/bin/env bash

print_update_help() {
  cat <<'EOF'
Usage:
  selfishell update [--cli-only | --tools-only] [--version VERSION]
                     [--skip-packages] [--dry-run] [--yes]

By default, update to the latest Selfishell release and synchronize that
release's profile packages, approved tools, and managed configuration. If the
target release is already installed, no changes are made. Use --tools-only to
explicitly resynchronize the current release's tools and configuration, or
--cli-only to limit the scope to the CLI release itself.
--version selects an exact CLI release and cannot be used with --tools-only.
When the tools/configuration phase runs, --skip-packages skips package and
tool installation and applies managed configuration only. A default update
whose target release is already installed exits before that phase; use
--tools-only --skip-packages to reapply the current release's configuration.

Already installed apt/Homebrew packages are left at their current version
(mise-managed and Selfishell direct tools are synced to their pinned
versions). Selfishell update does not perform a general Apt/Homebrew
upgrade.
EOF
}

update_tools_and_configuration() {
  local assume_yes="$1"
  local dry_run="$2"
  local require_configuration="$3"
  local skip_packages="$4"
  local profile platform ghostty_enabled=0

  SELFISHELL_UNCHANGED_COUNT=0

  selfishell_initialize_paths
  if [[ ! -r "$SELFISHELL_STATE_DIR/profile" ]]; then
    if [[ "$require_configuration" == 1 ]]; then
      cli_error "Selfishell configuration is not installed."
      return "$SELFISHELL_EXIT_ERROR"
    fi
    printf '%sSelfishell configuration is not installed; skipping tools and configuration.%s\n' "$SELFISHELL_COLOR_CYAN" "$SELFISHELL_COLOR_RESET"
    return
  fi
  profile="$(<"$SELFISHELL_STATE_DIR/profile")"
  [[ ! -r "$SELFISHELL_STATE_DIR/ghostty" ]] || ghostty_enabled="$(<"$SELFISHELL_STATE_DIR/ghostty")"
  [[ "$ghostty_enabled" == "1" ]] || ghostty_enabled=0
  confirm_action "Synchronize $profile profile packages and configuration?" "$assume_yes" "$dry_run" || return
  platform="$(detect_platform)"
  managed_preflight_zsh_loader "$assume_yes" "$dry_run" || return
  managed_preflight_block_target user-zprofile "$HOME/.zprofile" "$assume_yes" "$dry_run" || return
  case "$platform" in
    ubuntu | ubuntu-wsl)
      managed_preflight_block_target user-zshenv "$HOME/.zshenv" "$assume_yes" "$dry_run" || return
      ;;
  esac

  if [[ "$platform" == "macos" && "$ghostty_enabled" == "1" ]]; then
    managed_preflight_block_target user-ghostty \
      "${XDG_CONFIG_HOME:-$HOME/.config}/ghostty/config.ghostty" "$assume_yes" "$dry_run" || return
  fi

  profile_load "$profile"

  if [[ "$skip_packages" == "1" ]]; then
    printf '%sSkipping package and tool installation.%s\n' "$SELFISHELL_COLOR_CYAN" "$SELFISHELL_COLOR_RESET"
  else
    packages_install_profile "$platform" "$dry_run"
    if [[ "$platform" == "macos" && "$ghostty_enabled" == "1" ]]; then
      homebrew_install_packages optional cask "$dry_run" ghostty
    fi
  fi
  install_managed_configuration "$platform" "$dry_run" "$profile" "$ghostty_enabled" "$assume_yes"
  if [[ "$skip_packages" == "0" && "$profile" == "developer" ]]; then
    install_neovim_plugins "$dry_run" || return
  fi
  ((SELFISHELL_UNCHANGED_COUNT == 0)) ||
    printf '%s%d items unchanged.%s\n' "$SELFISHELL_COLOR_CYAN" "$SELFISHELL_UNCHANGED_COUNT" "$SELFISHELL_COLOR_RESET"
  if [[ "$dry_run" == 1 ]]; then
    printf '%sTool/configuration dry run complete.%s\n' "$SELFISHELL_COLOR_CYAN" "$SELFISHELL_COLOR_RESET"
  else
    printf '%sSelfishell tools and configuration updated.%s\n' "$SELFISHELL_COLOR_GREEN" "$SELFISHELL_COLOR_RESET"
  fi
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
    SELFISHELL_CLI_UP_TO_DATE=1
    SELFISHELL_CLI_TARGET_VERSION="$version"
    return
  fi
  if [[ "$dry_run" == 1 ]]; then
    printf '%sWould update Selfishell CLI to %s.%s\n' "$SELFISHELL_COLOR_CYAN" "$version" "$SELFISHELL_COLOR_RESET"
    return
  fi
  confirm_action "Update Selfishell CLI to $version?" "$assume_yes" 0 || return
  release_install "$version"
  SELFISHELL_CLI_UPDATED=1
}

continue_update_with_new_cli() {
  local assume_yes="$1"
  local skip_packages="$2"
  local arguments=(update --continue-after-cli-update)

  [[ "$assume_yes" == 0 ]] || arguments+=(--yes)
  [[ "$skip_packages" == 0 ]] || arguments+=(--skip-packages)
  exec "$SELFISHELL_SHARE_DIR/current/bin/selfishell" "${arguments[@]}"
}

command_update() {
  local assume_yes=0
  local dry_run=0
  local mode=all
  local version=""
  local continuation=0
  local skip_packages=0

  SELFISHELL_CLI_UPDATED=0
  SELFISHELL_CLI_UP_TO_DATE=0

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
      --skip-packages) skip_packages=1 ;;
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
    if [[ "$SELFISHELL_CLI_UP_TO_DATE" == 1 ]]; then
      if [[ "$mode" == all ]]; then
        # The target release is already active. Default update does not
        # resynchronize the current release; use --tools-only for that.
        printf '%sSelfishell is up to date (%s).%s\n' "$SELFISHELL_COLOR_GREEN" "$SELFISHELL_CLI_TARGET_VERSION" "$SELFISHELL_COLOR_RESET"
        return
      fi
      printf '%sSelfishell CLI is already at %s; skipping CLI update.%s\n' "$SELFISHELL_COLOR_CYAN" "$SELFISHELL_CLI_TARGET_VERSION" "$SELFISHELL_COLOR_RESET"
    fi
    if [[ "$mode" == all && "$dry_run" == 0 && "$SELFISHELL_CLI_UPDATED" == 1 ]]; then
      continue_update_with_new_cli "$assume_yes" "$skip_packages"
    fi
  fi

  if [[ "$mode" != cli ]]; then
    if [[ "$mode" == tools && "$continuation" == 0 ]]; then
      update_tools_and_configuration "$assume_yes" "$dry_run" 1 "$skip_packages"
    else
      update_tools_and_configuration "$assume_yes" "$dry_run" 0 "$skip_packages"
    fi
  fi
}
