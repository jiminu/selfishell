#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

source "$ROOT_DIR/tests/test_helper.bash"
source "$ROOT_DIR/lib/common.sh"
source "$ROOT_DIR/lib/platform.sh"

set_platform_fixture() {
  local system_name="$1"
  local architecture="$2"
  local distribution="$3"
  local proc_version="$4"

  export SELFISHELL_TEST_SYSTEM_NAME="$system_name"
  export SELFISHELL_TEST_MACHINE_ARCH="$architecture"
  export SELFISHELL_TEST_OS_RELEASE_FILE="$TEST_ROOT/os-release"
  export SELFISHELL_TEST_PROC_VERSION_FILE="$TEST_ROOT/proc-version"

  printf 'ID=%s\n' "$distribution" >"$SELFISHELL_TEST_OS_RELEASE_FILE"
  printf '%s\n' "$proc_version" >"$SELFISHELL_TEST_PROC_VERSION_FILE"
}

clear_platform_fixture() {
  unset SELFISHELL_TEST_SYSTEM_NAME
  unset SELFISHELL_TEST_MACHINE_ARCH
  unset SELFISHELL_TEST_OS_RELEASE_FILE
  unset SELFISHELL_TEST_PROC_VERSION_FILE
}

teardown_platform_test() {
  clear_platform_fixture
  teardown_test_home
}

test_detects_macos_arm64() {
  set_platform_fixture Darwin arm64 ignored ignored

  [[ "$(detect_platform)" == "macos" ]] || fail "Expected macOS"
  [[ "$(detect_architecture)" == "arm64" ]] || fail "Expected arm64"
}

test_detects_ubuntu_amd64() {
  set_platform_fixture Linux x86_64 ubuntu 'Linux version 6.8.0-generic'

  [[ "$(detect_platform)" == "ubuntu" ]] || fail "Expected native Ubuntu"
  [[ "$(detect_architecture)" == "amd64" ]] || fail "Expected amd64"
}

test_detects_ubuntu_wsl() {
  set_platform_fixture Linux x86_64 ubuntu 'Linux version 6.6.87.2-microsoft-standard-WSL2'

  [[ "$(detect_platform)" == "ubuntu-wsl" ]] || fail "Expected Ubuntu on WSL"
}

test_detects_unsupported_linux() {
  set_platform_fixture Linux x86_64 fedora 'Linux version 6.8.0'

  [[ "$(detect_platform)" == "unsupported-linux" ]] ||
    fail "Expected unsupported Linux"
}

test_detects_unsupported_wsl_distribution() {
  set_platform_fixture Linux x86_64 debian 'Linux microsoft WSL2'

  [[ "$(detect_platform)" == "unsupported-wsl" ]] ||
    fail "Expected unsupported WSL distribution"
}

test_preserves_unknown_architecture() {
  set_platform_fixture Linux riscv64 ubuntu 'Linux version 6.8.0'

  [[ "$(detect_architecture)" == "riscv64" ]] ||
    fail "Expected unknown architecture to be preserved"
}

test_selects_platform_package_manager() {
  [[ "$(platform_package_manager macos)" == "brew" ]] ||
    fail "Expected Homebrew on macOS"
  [[ "$(platform_package_manager ubuntu)" == "apt-get" ]] ||
    fail "Expected apt-get on Ubuntu"
  [[ "$(platform_package_manager ubuntu-wsl)" == "apt-get" ]] ||
    fail "Expected apt-get on Ubuntu WSL"
}

run_discovered_tests setup_test_home teardown_platform_test
