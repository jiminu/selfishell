#!/usr/bin/env bash

command_update() {
  local assume_yes=0
  local dry_run=0
  local profile platform profile_platform dependency_platform architecture index package

  while (("$#" > 0)); do
    case "$1" in
      --dry-run) dry_run=1 ;;
      --yes) assume_yes=1 ;;
      help | --help | -h)
        printf 'Usage: selfishell update [--dry-run] [--yes]\n'
        return
        ;;
      *)
        cli_error "Unknown update option: $1"
        return "$SELFISHELL_EXIT_USAGE"
        ;;
    esac
    shift
  done

  selfishell_initialize_paths
  [[ -r "$SELFISHELL_STATE_DIR/profile" ]] || {
    cli_error "Selfishell configuration is not installed."
    return "$SELFISHELL_EXIT_ERROR"
  }
  profile="$(<"$SELFISHELL_STATE_DIR/profile")"
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
  install_managed_configuration "$platform" "$dry_run" "$profile"
  [[ "$dry_run" == 1 ]] && printf 'Dry run complete; no files were changed.\n' || printf 'Selfishell tools and configuration updated.\n'
}
