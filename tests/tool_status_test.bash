#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/tests/test_helper.bash"
source "$ROOT_DIR/lib/common.sh"
source "$ROOT_DIR/lib/paths.sh"
source "$ROOT_DIR/lib/dependencies.sh"
source "$ROOT_DIR/lib/tool_status.sh"

setup_tool_status_home() {
  setup_test_home
  export XDG_STATE_HOME="$HOME/.local/state"
  export SELFISHELL_DEPENDENCIES_FILE="$TEST_ROOT/dependencies.conf"
  mkdir -p "$TEST_ROOT/bin"
  ORIGINAL_PATH="$PATH"
  export PATH="$TEST_ROOT/bin:/usr/bin:/bin"
  selfishell_initialize_paths
}

teardown_tool_status_home() {
  export PATH="$ORIGINAL_PATH"
  unset XDG_STATE_HOME SELFISHELL_DEPENDENCIES_FILE ORIGINAL_PATH
  teardown_test_home
}

test_detects_homebrew_formula_version() {
  setup_tool_status_home
  printf '#!/usr/bin/env bash\nprintf "starship 1.26.0\\n"\n' >"$TEST_ROOT/bin/brew"
  chmod +x "$TEST_ROOT/bin/brew"

  tool_status_detect formula starship macos arm64

  [[ "$TOOL_STATUS_INSTALLED" == 1.26.0 ]] || fail "Homebrew version was not detected"
  [[ "$TOOL_STATUS_SOURCE" == homebrew ]] || fail "Homebrew source was not reported"
  [[ "$TOOL_STATUS_APPROVED" == package-manager ]] || fail "Formula approval source was incorrect"
  teardown_tool_status_home
}

test_detects_apt_package_version() {
  setup_tool_status_home
  printf '#!/usr/bin/env bash\nprintf "2.43.0-1ubuntu7\\n"\n' >"$TEST_ROOT/bin/dpkg-query"
  chmod +x "$TEST_ROOT/bin/dpkg-query"

  tool_status_detect apt git linux amd64

  [[ "$TOOL_STATUS_INSTALLED" == 2.43.0-1ubuntu7 ]] || fail "Apt version was not detected"
  [[ "$TOOL_STATUS_SOURCE" == apt ]] || fail "Apt source was not reported"
  teardown_tool_status_home
}

test_detects_selfishell_managed_direct_dependency() {
  setup_tool_status_home
  printf 'git zinit v3.15.0 all all file:///unused - .local/share/zinit/zinit.git zinit.zsh\n' >"$SELFISHELL_DEPENDENCIES_FILE"
  mkdir -p "$HOME/.local/share/zinit/zinit.git" "$SELFISHELL_STATE_DIR/dependencies"
  printf ':\n' >"$HOME/.local/share/zinit/zinit.git/zinit.zsh"
  printf 'v3.15.0\n' >"$SELFISHELL_STATE_DIR/dependencies/zinit"

  tool_status_detect direct zinit macos arm64

  [[ "$TOOL_STATUS_INSTALLED" == v3.15.0 ]] || fail "Managed dependency version was not detected"
  [[ "$TOOL_STATUS_SOURCE" == selfishell ]] || fail "Managed dependency source was not reported"
  [[ "$TOOL_STATUS_APPROVED" == v3.15.0 ]] || fail "Managed dependency approval was not reported"

  rm "$HOME/.local/share/zinit/zinit.git/zinit.zsh"
  tool_status_detect direct zinit macos arm64
  [[ "$TOOL_STATUS_INSTALLED" == missing && "$TOOL_STATUS_SOURCE" == selfishell ]] ||
    fail "Missing managed dependency marker was not detected"
  teardown_tool_status_home
}

test_distinguishes_external_and_missing_direct_dependencies() {
  setup_tool_status_home
  printf 'git zinit v3.15.0 all all file:///unused - .local/share/zinit/zinit.git zinit.zsh\n' >"$SELFISHELL_DEPENDENCIES_FILE"
  mkdir -p "$HOME/.local/share/zinit/zinit.git"
  printf ':\n' >"$HOME/.local/share/zinit/zinit.git/zinit.zsh"

  tool_status_detect direct zinit macos arm64
  [[ "$TOOL_STATUS_INSTALLED" == detected && "$TOOL_STATUS_SOURCE" == external ]] ||
    fail "External direct dependency was not distinguished"

  rm -rf "$HOME/.local/share/zinit"
  tool_status_detect direct zinit macos arm64
  [[ "$TOOL_STATUS_INSTALLED" == missing && "$TOOL_STATUS_SOURCE" == none ]] ||
    fail "Missing direct dependency was not distinguished"
  teardown_tool_status_home
}

main() {
  test_detects_apt_package_version
  printf 'PASS: test_detects_apt_package_version\n'
  test_detects_homebrew_formula_version
  printf 'PASS: test_detects_homebrew_formula_version\n'
  test_detects_selfishell_managed_direct_dependency
  printf 'PASS: test_detects_selfishell_managed_direct_dependency\n'
  test_distinguishes_external_and_missing_direct_dependencies
  printf 'PASS: test_distinguishes_external_and_missing_direct_dependencies\n'
}

main "$@"
