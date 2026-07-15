#!/usr/bin/env bash

command_install() {
  local platform
  platform="$(detect_platform)"

  case "$platform" in
    macos)
      macos_legacy_install "$@"
      ;;
    ubuntu-wsl)
      ubuntu_wsl_legacy_install "$@"
      ;;
    ubuntu)
      cli_error "Native Ubuntu installation is not implemented yet."
      cli_error "Ubuntu on WSL remains available during the CLI transition."
      return "$SELFISHELL_EXIT_ERROR"
      ;;
    *)
      cli_error "Installation is unavailable on $(platform_label "$platform")."
      cli_error "Run 'selfishell doctor' for supported platform details."
      return "$SELFISHELL_EXIT_ERROR"
      ;;
  esac
}
