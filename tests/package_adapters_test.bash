#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

source "$ROOT_DIR/tests/test_helper.bash"
source "$ROOT_DIR/lib/common.sh"
source "$ROOT_DIR/lib/package_managers/apt.sh"
source "$ROOT_DIR/lib/package_managers/homebrew.sh"

MOCK_AVAILABLE_PACKAGES=""
MOCK_INSTALL_FAILURE=0
MOCK_INSTALLED_PACKAGES=""
MOCK_UID=1000
MOCK_HAVE_SUDO=1
MOCK_SUDO_COUNT=0

have_command() {
  [[ "$1" != "sudo" || "$MOCK_HAVE_SUDO" == "1" ]]
}

id() {
  [[ "$1" == "-u" ]] || return 1
  printf '%s\n' "$MOCK_UID"
}

dpkg() {
  return 1
}

apt-cache() {
  [[ " $MOCK_AVAILABLE_PACKAGES " == *" $2 "* ]]
}

apt-get() {
  if [[ "$1" == "install" ]]; then
    shift 2
    MOCK_INSTALLED_PACKAGES="$*"
    ((MOCK_INSTALL_FAILURE == 0))
  fi
}

sudo() {
  MOCK_SUDO_COUNT=$((MOCK_SUDO_COUNT + 1))
  shift
  apt-get "$@"
}

brew() {
  if [[ "$1" == "install" ]]; then
    shift
    [[ "${1:-}" == "--cask" ]] && shift
    MOCK_INSTALLED_PACKAGES="$*"
    ((MOCK_INSTALL_FAILURE == 0))
  fi
}

reset_package_mocks() {
  SELFISHELL_APT_UPDATED=0
  SELFISHELL_SKIPPED_OPTIONAL_PACKAGES=()
  MOCK_AVAILABLE_PACKAGES=""
  MOCK_INSTALL_FAILURE=0
  MOCK_INSTALLED_PACKAGES=""
  MOCK_UID=1000
  MOCK_HAVE_SUDO=1
  MOCK_SUDO_COUNT=0
}

test_apt_non_root_requires_sudo() {
  reset_package_mocks
  MOCK_AVAILABLE_PACKAGES="available"

  apt_install_managed_packages required 0 available

  [[ "$MOCK_SUDO_COUNT" -eq 2 ]] || fail "Non-root apt operations must use sudo"
}

test_apt_non_root_without_sudo_fails() {
  reset_package_mocks
  MOCK_HAVE_SUDO=0

  if apt_install_managed_packages required 0 available; then
    fail "Non-root apt installation without sudo must fail"
  fi
}

test_apt_root_does_not_require_sudo() {
  reset_package_mocks
  MOCK_UID=0
  MOCK_HAVE_SUDO=0
  MOCK_AVAILABLE_PACKAGES="available"

  apt_install_managed_packages required 0 available

  [[ "$MOCK_INSTALLED_PACKAGES" == "available" ]] || fail "Root apt installation did not run"
  [[ "$MOCK_SUDO_COUNT" -eq 0 ]] || fail "Root apt operations must not use sudo"
}

test_apt_installs_available_optional_packages() {
  reset_package_mocks
  MOCK_AVAILABLE_PACKAGES="available"

  apt_install_managed_packages optional 0 available missing

  [[ "$MOCK_INSTALLED_PACKAGES" == "available" ]] ||
    fail "Available optional apt package was not installed"
  [[ "${SELFISHELL_SKIPPED_OPTIONAL_PACKAGES[*]}" == "missing" ]] ||
    fail "Unavailable optional apt package was not reported"
}

test_apt_required_unavailable_fails() {
  reset_package_mocks

  if apt_install_managed_packages required 0 missing; then
    fail "Unavailable required apt package must fail"
  fi
}

test_homebrew_optional_failure_is_reported() {
  reset_package_mocks
  MOCK_INSTALL_FAILURE=1

  homebrew_install_packages optional formula 0 optional-tool
  [[ "${SELFISHELL_SKIPPED_OPTIONAL_PACKAGES[*]}" == "optional-tool" ]] ||
    fail "Failed optional Homebrew formula was not reported"
}

test_homebrew_required_failure_fails() {
  reset_package_mocks
  MOCK_INSTALL_FAILURE=1

  if homebrew_install_packages required cask 0 required-tool; then
    fail "Failed required Homebrew cask must fail"
  fi
}

run_test() {
  local test_name="$1"
  "$test_name"
  printf 'PASS: %s\n' "$test_name"
}

main() {
  local test_name

  while IFS= read -r test_name; do
    run_test "$test_name"
  done < <(declare -F | awk '{print $3}' | grep '^test_' | sort)
}

main "$@"
