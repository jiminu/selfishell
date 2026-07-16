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
  tool_status_reset_cache
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

test_reuses_homebrew_inventory() {
  setup_tool_status_home
  export MOCK_BREW_LOG="$TEST_ROOT/brew.log"
  cat >"$TEST_ROOT/bin/brew" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$MOCK_BREW_LOG"
case "$*" in
  'list --formula --versions')
    printf 'starship 1.26.0\nfzf 0.74.0\n'
    ;;
  'list --cask --versions')
    printf 'ghostty 1.3.1\nfont-meslo-lg-nerd-font 3.4.0\n'
    ;;
  *)
    exit 1
    ;;
esac
EOF
  chmod +x "$TEST_ROOT/bin/brew"

  tool_status_detect formula starship macos arm64
  tool_status_detect formula fzf macos arm64
  tool_status_detect cask ghostty macos arm64
  tool_status_detect cask font-meslo-lg-nerd-font macos arm64

  [[ "$(wc -l <"$MOCK_BREW_LOG" | tr -d ' ')" == 2 ]] ||
    fail "Homebrew inventory should be loaded once per package type"
  teardown_tool_status_home
  unset MOCK_BREW_LOG
}

test_detects_apt_package_version() {
  setup_tool_status_home
  printf '#!/usr/bin/env bash\nprintf "git\\t2.43.0-1ubuntu7\\n"\n' >"$TEST_ROOT/bin/dpkg-query"
  chmod +x "$TEST_ROOT/bin/dpkg-query"

  tool_status_detect apt git linux amd64

  [[ "$TOOL_STATUS_INSTALLED" == 2.43.0-1ubuntu7 ]] || fail "Apt version was not detected"
  [[ "$TOOL_STATUS_SOURCE" == apt ]] || fail "Apt source was not reported"
  teardown_tool_status_home
}

test_reuses_apt_inventory() {
  setup_tool_status_home
  export MOCK_DPKG_LOG="$TEST_ROOT/dpkg.log"
  cat >"$TEST_ROOT/bin/dpkg-query" <<'EOF'
#!/usr/bin/env bash
printf 'query\n' >>"$MOCK_DPKG_LOG"
printf 'git\t2.43.0\ncurl\t8.5.0\n'
EOF
  chmod +x "$TEST_ROOT/bin/dpkg-query"

  tool_status_detect apt git linux amd64
  tool_status_detect apt curl linux amd64

  [[ "$(wc -l <"$MOCK_DPKG_LOG" | tr -d ' ')" == 1 ]] ||
    fail "Apt inventory should be loaded once"
  [[ "$TOOL_STATUS_INSTALLED" == 8.5.0 ]] || fail "Cached Apt inventory returned the wrong version"
  teardown_tool_status_home
  unset MOCK_DPKG_LOG
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

test_maps_package_name_to_executable() {
  [[ "$(tool_status_executable ripgrep)" == "rg" ]] ||
    fail "Ripgrep package name was not mapped to the rg executable"
}

test_reports_package_manager_updates_without_upgrading() {
  setup_tool_status_home
  export MOCK_BREW_LOG="$TEST_ROOT/brew.log"
  cat >"$TEST_ROOT/bin/brew" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$MOCK_BREW_LOG"
case "$*" in
  'outdated --formula') printf 'starship\n' ;;
  'outdated --cask') printf 'ghostty\n' ;;
esac
EOF
  cat >"$TEST_ROOT/bin/apt" <<'EOF'
#!/usr/bin/env bash
[[ "$*" == 'list --upgradable' ]] || exit 1
printf 'Listing...\ngit/noble-updates 2.44 amd64 [upgradable from: 2.43]\n'
EOF
  chmod +x "$TEST_ROOT/bin/brew" "$TEST_ROOT/bin/apt"

  tool_status_package_update formula starship
  [[ "$TOOL_STATUS_UPDATE" == available ]] || fail "Homebrew formula update was not detected"
  tool_status_package_update formula fzf
  [[ "$TOOL_STATUS_UPDATE" == current ]] || fail "Current Homebrew formula was not reported"
  tool_status_package_update cask ghostty
  [[ "$TOOL_STATUS_UPDATE" == available ]] || fail "Homebrew cask update was not detected"
  tool_status_package_update apt git
  [[ "$TOOL_STATUS_UPDATE" == available ]] || fail "Apt update was not detected"
  [[ "$(grep -Fc 'outdated --formula' "$MOCK_BREW_LOG")" == 1 ]] || fail "Homebrew outdated inventory was not cached"

  teardown_tool_status_home
  unset MOCK_BREW_LOG
}

main() {
  test_detects_apt_package_version
  printf 'PASS: test_detects_apt_package_version\n'
  test_detects_homebrew_formula_version
  printf 'PASS: test_detects_homebrew_formula_version\n'
  test_reuses_homebrew_inventory
  printf 'PASS: test_reuses_homebrew_inventory\n'
  test_reuses_apt_inventory
  printf 'PASS: test_reuses_apt_inventory\n'
  test_detects_selfishell_managed_direct_dependency
  printf 'PASS: test_detects_selfishell_managed_direct_dependency\n'
  test_distinguishes_external_and_missing_direct_dependencies
  printf 'PASS: test_distinguishes_external_and_missing_direct_dependencies\n'
  test_maps_package_name_to_executable
  printf 'PASS: test_maps_package_name_to_executable\n'
  test_reports_package_manager_updates_without_upgrading
  printf 'PASS: test_reports_package_manager_updates_without_upgrading\n'
}

main "$@"
