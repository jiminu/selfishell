#!/usr/bin/env bash

doctor_ok() {
  printf '[OK] %s\n' "$*"
}

doctor_error() {
  printf '[ERROR] %s\n' "$*"
}

command_doctor() {
  require_no_arguments doctor "$@" || return

  local platform
  local architecture
  local package_manager
  local result="$SELFISHELL_EXIT_OK"

  platform="$(detect_platform)"
  architecture="$(detect_architecture)"

  printf 'Selfishell doctor\n\n'

  if platform_is_supported "$platform"; then
    doctor_ok "Platform: $(platform_label "$platform")"
  else
    doctor_error "Platform: $(platform_label "$platform")"
    case "$platform" in
      unsupported-wsl)
        printf '        Only Ubuntu on WSL is currently supported.\n'
        ;;
      unsupported-linux)
        printf '        Ubuntu is the only supported native Linux distribution.\n'
        ;;
      *)
        printf '        Use macOS, Ubuntu, or Ubuntu on WSL.\n'
        ;;
    esac
    result="$SELFISHELL_EXIT_ERROR"
  fi

  case "$architecture" in
    amd64 | arm64)
      doctor_ok "Architecture: $architecture"
      ;;
    *)
      doctor_error "Architecture: $architecture (supported: amd64, arm64)"
      result="$SELFISHELL_EXIT_ERROR"
      ;;
  esac

  package_manager="$(platform_package_manager "$platform")"
  if [[ "$package_manager" == "unknown" ]]; then
    doctor_error "Package manager: unavailable for this platform"
  elif have_command "$package_manager"; then
    doctor_ok "Package manager: $package_manager"
  else
    doctor_error "Package manager: $package_manager was not found"
    printf "        Run 'selfishell install' to set up the supported toolchain.\n"
    result="$SELFISHELL_EXIT_ERROR"
  fi

  return "$result"
}
