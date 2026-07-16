#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SELFISHELL_ROOT="$ROOT_DIR"
export SELFISHELL_ROOT

source "$ROOT_DIR/lib/common.sh"
source "$ROOT_DIR/lib/platform.sh"

platform="$(detect_platform)"
case "$platform" in
  macos) exec bash "$ROOT_DIR/legacy/macos.sh" "$@" ;;
  ubuntu-wsl) exec bash "$ROOT_DIR/legacy/ubuntu.sh" "$@" ;;
  ubuntu)
    cli_error "Native Ubuntu is supported by 'selfishell install', not the legacy bootstrap."
    ;;
  *)
    cli_error "Legacy installation is unavailable on $(platform_label "$platform")."
    ;;
esac
cli_error "Use the managed installer documented in README.md."
exit "$SELFISHELL_EXIT_ERROR"
