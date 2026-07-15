#!/usr/bin/env bash

macos_package_manager() {
  printf 'brew\n'
}

macos_legacy_install() {
  exec bash "$SELFISHELL_ROOT/legacy/macos.sh" "$@"
}
