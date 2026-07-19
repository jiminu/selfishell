#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/tests/test_helper.bash"

setup_update_home() {
  setup_test_home
  export XDG_STATE_HOME="$HOME/.local/state"
  export SELFISHELL_TEST_SYSTEM_NAME=Linux
  export SELFISHELL_TEST_MACHINE_ARCH=x86_64
  export SELFISHELL_TEST_OS_RELEASE_FILE="$TEST_ROOT/os-release"
  export SELFISHELL_TEST_PROC_VERSION_FILE="$TEST_ROOT/proc-version"
  printf 'ID=ubuntu\n' >"$SELFISHELL_TEST_OS_RELEASE_FILE"
  printf 'Linux version 6.8.0\n' >"$SELFISHELL_TEST_PROC_VERSION_FILE"
}

teardown_update_home() {
  unset XDG_CONFIG_HOME XDG_STATE_HOME SELFISHELL_DEPENDENCIES_FILE
  unset SELFISHELL_TEST_SYSTEM_NAME SELFISHELL_TEST_MACHINE_ARCH
  unset SELFISHELL_TEST_OS_RELEASE_FILE SELFISHELL_TEST_PROC_VERSION_FILE
  teardown_test_home
}

test_tools_update_synchronizes_profile_packages() {
  local output
  setup_update_home
  export XDG_CONFIG_HOME="$HOME/.config"
  mkdir -p "$XDG_STATE_HOME/selfishell"
  printf 'minimal\n' >"$XDG_STATE_HOME/selfishell/profile"

  output="$(bash "$ROOT_DIR/bin/selfishell" update --tools-only --dry-run)"
  [[ "$output" == *'Would install required apt packages:'* ]] ||
    fail "Tools update did not synchronize package-manager packages"
  [[ "$output" == *'ripgrep'* ]] || fail "Tools update did not include the current profile packages"
  teardown_update_home
}

test_download_dependency_is_checksum_verified_and_recorded() {
  local payload checksum output
  setup_update_home
  payload="$TEST_ROOT/tool"
  printf '#!/bin/sh\nprintf tool-1.0\\n\n' >"$payload"
  checksum="$(fixture_sha256 "$payload")"
  export SELFISHELL_DEPENDENCIES_FILE="$TEST_ROOT/dependencies.conf"
  printf 'download tool 1.0 linux amd64 file://%s %s .local/bin/tool raw\n' "$payload" "$checksum" >"$SELFISHELL_DEPENDENCIES_FILE"

  output="$(bash -c 'source "$1/lib/common.sh"; source "$1/lib/paths.sh"; source "$1/lib/dependencies.sh"; dependency_install tool linux amd64' _ "$ROOT_DIR")"
  [[ "$output" == *'Installed approved dependency: tool 1.0'* ]] || fail "Dependency install was not reported"
  assert_file_content '1.0' "$XDG_STATE_HOME/selfishell/dependencies/tool"
  [[ -x "$HOME/.local/bin/tool" ]] || fail "Verified tool was not installed"
  teardown_update_home
}

test_checksum_failure_preserves_existing_managed_tool() {
  local status
  setup_update_home
  mkdir -p "$HOME/.local/bin" "$XDG_STATE_HOME/selfishell/dependencies"
  printf 'old tool\n' >"$HOME/.local/bin/tool"
  printf '0.9\n' >"$XDG_STATE_HOME/selfishell/dependencies/tool"
  printf 'new tool\n' >"$TEST_ROOT/tool"
  export SELFISHELL_DEPENDENCIES_FILE="$TEST_ROOT/dependencies.conf"
  printf 'download tool 1.0 linux amd64 file://%s %064d .local/bin/tool raw\n' "$TEST_ROOT/tool" 0 >"$SELFISHELL_DEPENDENCIES_FILE"

  set +e
  bash -c 'source "$1/lib/common.sh"; source "$1/lib/paths.sh"; source "$1/lib/dependencies.sh"; dependency_install tool linux amd64' _ "$ROOT_DIR" >/dev/null 2>&1
  status=$?
  set -e
  [[ "$status" -ne 0 ]] || fail "Invalid checksum should fail"
  assert_file_content 'old tool' "$HOME/.local/bin/tool"
  assert_file_content '0.9' "$XDG_STATE_HOME/selfishell/dependencies/tool"
  teardown_update_home
}

test_tools_update_removes_legacy_neovim_installation() {
  local fake_bin
  setup_update_home
  export XDG_CONFIG_HOME="$HOME/.config"
  fake_bin="$TEST_ROOT/bin"
  mkdir -p "$fake_bin" "$XDG_STATE_HOME/selfishell/dependencies" "$HOME/.local/bin" \
    "$HOME/.config/selfishell" "$HOME/.local/share/nvim/lazy/lazy.nvim"
  printf 'developer\n' >"$XDG_STATE_HOME/selfishell/profile"
  printf '0.2.1\n' >"$XDG_STATE_HOME/selfishell/dependencies/neovim"
  printf '#!/usr/bin/env bash\nexit 0\n' >"$HOME/.local/bin/nvim"
  chmod +x "$HOME/.local/bin/nvim"
  cat >"$fake_bin/apt" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
  cat >"$fake_bin/mise" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
  cat >"$fake_bin/nvim" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
  chmod +x "$fake_bin/apt" "$fake_bin/mise" "$fake_bin/nvim"

  output="$(bash "$ROOT_DIR/bin/selfishell" update --tools-only --dry-run)"
  [[ "$output" == *'Would remove legacy Neovim installation:'* ]] ||
    fail "Legacy Neovim removal was not previewed"

  output="$(PATH="$fake_bin:/usr/bin:/bin" bash "$ROOT_DIR/bin/selfishell" update --tools-only --yes)"
  [[ "$output" == *'Removed legacy Neovim installation:'* ]] ||
    fail "Legacy Neovim removal was not reported"
  [[ ! -e "$HOME/.local/bin/nvim" ]] || fail "Legacy Neovim binary was not removed"
  [[ ! -e "$XDG_STATE_HOME/selfishell/dependencies/neovim" ]] ||
    fail "Legacy Neovim state was not removed"
  teardown_update_home
}

main() {
  test_tools_update_synchronizes_profile_packages
  printf 'PASS: test_tools_update_synchronizes_profile_packages\n'
  test_download_dependency_is_checksum_verified_and_recorded
  printf 'PASS: test_download_dependency_is_checksum_verified_and_recorded\n'
  test_checksum_failure_preserves_existing_managed_tool
  printf 'PASS: test_checksum_failure_preserves_existing_managed_tool\n'
  test_tools_update_removes_legacy_neovim_installation
  printf 'PASS: test_tools_update_removes_legacy_neovim_installation\n'
}

main "$@"
