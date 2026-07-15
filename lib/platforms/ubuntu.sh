#!/usr/bin/env bash

ubuntu_package_manager() {
  printf 'apt-get\n'
}

ubuntu_wsl_legacy_install() {
  exec bash "$SELFISHELL_ROOT/ubuntu/ubuntu.sh" "$@"
}
