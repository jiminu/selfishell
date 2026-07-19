#!/usr/bin/env bash

doctor_ok() {
  printf '[OK] %s\n' "$*"
}

doctor_error() {
  printf '[ERROR] %s\n' "$*"
}

doctor_info() {
  printf '[INFO] %s\n' "$*"
}

doctor_report_package() {
  local package="$1"
  local manager="$2"
  local requirement="$3"
  local dependency_platform="$4"
  local architecture="$5"

  tool_status_detect "$manager" "$package" "$dependency_platform" "$architecture"
  if [[ "$TOOL_STATUS_INSTALLED" == missing ]]; then
    if [[ "$requirement" == required ]]; then
      doctor_error "Tool: $package is missing ($manager)"
      DOCTOR_RESULT="$SELFISHELL_EXIT_ERROR"
    else
      doctor_info "Optional tool: $package is not installed ($manager)"
    fi
  else
    doctor_ok "Tool: $package $TOOL_STATUS_INSTALLED ($TOOL_STATUS_SOURCE)"
  fi
}

command_doctor() {
  require_no_arguments doctor "$@" || return

  local platform
  local architecture
  local package_manager
  local profile profile_platform dependency_platform
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

  if have_command gcc; then
    doctor_ok "C compiler: gcc ($(gcc --version | head -n 1))"
  elif have_command clang; then
    doctor_ok "C compiler: clang ($(clang --version | head -n 1))"
  else
    doctor_error "C compiler: gcc or clang was not found (required for compiling Treesitter parsers)"
    if [[ "$platform" == "macos" ]]; then
      printf "        Install Xcode Command Line Tools by running: xcode-select --install\n"
    else
      printf "        Install build tools by running: sudo apt install build-essential\n"
    fi
    result="$SELFISHELL_EXIT_ERROR"
  fi

  selfishell_initialize_paths
  if [[ -r "$SELFISHELL_STATE_DIR/profile" ]] && platform_is_supported "$platform"; then
    tool_status_reset_cache
    profile="$(<"$SELFISHELL_STATE_DIR/profile")"
    doctor_info "Profile: $profile"
    if [[ "$profile" == developer ]]; then
      if [[ -d "$HOME/.nvm" ]]; then
        doctor_info "Legacy runtime manager detected: $HOME/.nvm (preserved; mise is active)"
      fi
      if [[ -d "$HOME/.pyenv" ]]; then
        doctor_info "Legacy runtime manager detected: $HOME/.pyenv (preserved; mise is active)"
      fi
    fi
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
    DOCTOR_RESULT="$result"
    selfishell_scan_profile_packages "$profile" "$dependency_platform" "$architecture" doctor_report_package "$profile_platform"
    result="$DOCTOR_RESULT"
  fi

  return "$result"
}
