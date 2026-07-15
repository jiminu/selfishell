#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

source "$ROOT_DIR/tests/test_helper.bash"
source "$ROOT_DIR/ubuntu/ubuntu.sh"

MOCK_AVAILABLE_PACKAGES=""
MOCK_INSTALLED_PACKAGES=""
MOCK_UPDATE_COUNT=0
MOCK_INSTALL_FAILURE=0

have() {
  [[ "$1" == "apt-get" ]]
}

dpkg() {
  return 1
}

apt-cache() {
  local package="$2"
  [[ " $MOCK_AVAILABLE_PACKAGES " == *" $package "* ]]
}

sudo() {
  if [[ "$1 $2" == "apt-get update" ]]; then
    MOCK_UPDATE_COUNT=$((MOCK_UPDATE_COUNT + 1))
  elif [[ "$1 $2" == "apt-get install" ]]; then
    shift 3
    MOCK_INSTALLED_PACKAGES="$*"
    ((MOCK_INSTALL_FAILURE == 0))
  fi
}

reset_apt_mocks() {
  APT_INDEX_UPDATED=0
  APT_SKIPPED_OPTIONAL_PACKAGES=()
  MOCK_AVAILABLE_PACKAGES=""
  MOCK_INSTALLED_PACKAGES=""
  MOCK_UPDATE_COUNT=0
  MOCK_INSTALL_FAILURE=0
}

test_required_package_unavailable_fails() {
  reset_apt_mocks
  MOCK_AVAILABLE_PACKAGES="available"

  if apt_install_required_packages available missing; then
    fail "Missing required package must fail"
    return
  fi

  [[ "$MOCK_INSTALLED_PACKAGES" == "available" ]] ||
    fail "Available required packages should still be installed"
}

test_optional_package_unavailable_succeeds() {
  reset_apt_mocks
  MOCK_AVAILABLE_PACKAGES="available"

  apt_install_optional_packages available missing
  [[ "$MOCK_INSTALLED_PACKAGES" == "available" ]] ||
    fail "Available optional packages should be installed"
  [[ "${APT_SKIPPED_OPTIONAL_PACKAGES[*]}" == "missing" ]] ||
    fail "Missing optional packages should be retained for the final summary"
}

test_optional_install_failure_succeeds() {
  reset_apt_mocks
  MOCK_AVAILABLE_PACKAGES="optional-tool"
  MOCK_INSTALL_FAILURE=1

  apt_install_optional_packages optional-tool
  [[ "${APT_SKIPPED_OPTIONAL_PACKAGES[*]}" == "optional-tool" ]] ||
    fail "Failed optional packages should be retained for the final summary"
}

test_required_install_failure_fails() {
  reset_apt_mocks
  MOCK_AVAILABLE_PACKAGES="required-tool"
  MOCK_INSTALL_FAILURE=1

  if apt_install_required_packages required-tool; then
    fail "Failed required package installation must fail"
  fi
}

test_apt_index_updates_once() {
  reset_apt_mocks
  MOCK_AVAILABLE_PACKAGES="first second"

  apt_install_required_packages first
  apt_install_optional_packages second
  [[ "$MOCK_UPDATE_COUNT" -eq 1 ]] || fail "Expected one apt index update"
}

run_test() {
  local test_name="$1"

  setup_test_home
  trap teardown_test_home RETURN
  "$test_name"
  trap - RETURN
  teardown_test_home
  printf 'PASS: %s\n' "$test_name"
}

main() {
  local test_name
  local failures=0

  while IFS= read -r test_name; do
    if ! run_test "$test_name"; then
      failures=$((failures + 1))
    fi
  done < <(declare -F | awk '{print $3}' | grep '^test_' | sort)

  if ((failures > 0)); then
    printf '%d test(s) failed\n' "$failures" >&2
    return 1
  fi
}

main "$@"
