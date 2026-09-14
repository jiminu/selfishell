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
  printf '#!/usr/bin/env bash\nprintf "starship 1.26.0\\n"\n' >"$TEST_ROOT/bin/brew"
  chmod +x "$TEST_ROOT/bin/brew"

  tool_status_detect formula starship macos arm64

  [[ "$TOOL_STATUS_INSTALLED" == 1.26.0 ]] || fail "Homebrew version was not detected"
  [[ "$TOOL_STATUS_SOURCE" == homebrew ]] || fail "Homebrew source was not reported"
  [[ "$TOOL_STATUS_APPROVED" == package-manager ]] || fail "Formula approval source was incorrect"
}

test_reuses_homebrew_inventory() {
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
  unset MOCK_BREW_LOG
}

test_detects_apt_package_version() {
  printf '#!/usr/bin/env bash\nprintf "git\\t2.43.0-1ubuntu7\\n"\n' >"$TEST_ROOT/bin/dpkg-query"
  chmod +x "$TEST_ROOT/bin/dpkg-query"

  tool_status_detect apt git linux amd64

  [[ "$TOOL_STATUS_INSTALLED" == 2.43.0-1ubuntu7 ]] || fail "Apt version was not detected"
  [[ "$TOOL_STATUS_SOURCE" == apt ]] || fail "Apt source was not reported"
}

test_reuses_apt_inventory() {
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
  unset MOCK_DPKG_LOG
}

test_detects_selfishell_managed_direct_dependency() {
  printf 'git zinit v3.15.0 all all file:///unused - .local/share/zinit/zinit.git zinit.zsh\n' >"$SELFISHELL_DEPENDENCIES_FILE"
  mkdir -p "$HOME/.local/share/zinit/zinit.git/.git" "$SELFISHELL_STATE_DIR/dependencies"
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
}

test_distinguishes_external_and_missing_direct_dependencies() {
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
}

test_external_dangling_symlink_direct_dependency_is_not_detected() {
  printf 'git zinit v3.15.0 all all file:///unused - .local/share/zinit/zinit.git zinit.zsh\n' >"$SELFISHELL_DEPENDENCIES_FILE"
  mkdir -p "$HOME/.local/share/zinit"
  ln -s "$TEST_ROOT/missing-zinit-checkout" "$HOME/.local/share/zinit/zinit.git"

  tool_status_detect direct zinit macos arm64
  [[ "$TOOL_STATUS_INSTALLED" == missing && "$TOOL_STATUS_SOURCE" == none ]] ||
    fail "A dangling-symlink external direct dependency was reported as detected"
}

test_maps_package_name_to_executable() {
  [[ "$(tool_status_executable ripgrep)" == "rg" ]] ||
    fail "Ripgrep package name was not mapped to the rg executable"
  [[ "$(tool_status_executable kubectl@1.36.2)" == "kubectl" ]] ||
    fail "mise selector was not mapped to its executable"
}

setup_mise_inventory() {
  cat >"$TEST_ROOT/bin/mise" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$HOME/mise-calls"
[[ "$*" == current ]] || exit 1
[[ "$MISE_GLOBAL_CONFIG_FILE" == "$SELFISHELL_CONFIG_DIR/mise/selfishell.toml" ]] || exit 1
cat "$HOME/mise-inventory"
[[ ! -f "$HOME/mise-fail" ]]
EOF
  chmod +x "$TEST_ROOT/bin/mise"
  export SELFISHELL_CONFIG_DIR
  export SELFISHELL_ROOT="$TEST_ROOT/selfishell-root"
  mkdir -p "$SELFISHELL_ROOT/config/shared"
  cat >"$SELFISHELL_ROOT/config/shared/mise.toml" <<'EOF'
[tools]
node = "24.18.0"
python = "3.13.14"
neovim = "0.12.5"
tree-sitter = "0.27.0"
uv = "0.12.13"
gh = "2.100.0"

[settings]
node = "ignored"
EOF
  printf 'node 24.18.0\npython 3.13.14 3.12.0\n' >"$HOME/mise-inventory"
}

# Approved mise versions come from mise.toml, not the profile's bare tool names.
test_detects_mise_tool_version() {
  setup_mise_inventory

  tool_status_detect mise node linux amd64

  [[ "$TOOL_STATUS_INSTALLED" == 24.18.0 ]] || fail "mise tool version was not detected"
  [[ "$TOOL_STATUS_SOURCE" == mise ]] || fail "mise tool source was not reported"
  [[ "$TOOL_STATUS_APPROVED" == 24.18.0 ]] || fail "mise approved version was not read from config/shared/mise.toml"
}

test_reuses_mise_inventory_and_approved_versions_until_reset() {
  local tool expected
  setup_mise_inventory
  tool_status_detect mise node linux amd64

  printf 'python 3.14.0\n' >"$HOME/mise-inventory"
  printf '[tools]\npython = "3.14.0"\n' >"$SELFISHELL_ROOT/config/shared/mise.toml"
  tool_status_detect mise python linux amd64
  [[ "$TOOL_STATUS_INSTALLED" == '3.13.14 3.12.0' && "$TOOL_STATUS_SOURCE" == mise ]] ||
    fail "Cached mise inventory lost multiple versions or was reloaded"
  [[ "$TOOL_STATUS_APPROVED" == 3.13.14 ]] || fail "Approved mise versions were reloaded before reset"
  while read -r tool expected; do
    tool_status_detect mise "$tool" linux amd64
    [[ "$TOOL_STATUS_APPROVED" == "$expected" ]] || fail "Wrong approved version for $tool"
  done <<'EOF'
neovim 0.12.5
tree-sitter 0.27.0
uv 0.12.13
gh 2.100.0
EOF
  [[ "$(wc -l <"$HOME/mise-calls" | tr -d ' ')" == 1 ]] || fail "mise inventory should be loaded once"

  tool_status_reset_cache
  tool_status_detect mise python linux amd64
  [[ "$TOOL_STATUS_INSTALLED" == 3.14.0 && "$TOOL_STATUS_APPROVED" == 3.14.0 ]] ||
    fail "Reset did not refresh both mise inventories"
  [[ "$(wc -l <"$HOME/mise-calls" | tr -d ' ')" == 2 ]] || fail "Reset did not reload mise inventory once"
}

test_mise_inventory_missing_and_failed_queries_use_executable_fallback() {
  setup_mise_inventory
  printf '#!/usr/bin/env bash\nexit 0\n' >"$TEST_ROOT/bin/uv"
  chmod +x "$TEST_ROOT/bin/uv"
  tool_status_detect mise uv linux amd64
  [[ "$TOOL_STATUS_INSTALLED" == detected && "$TOOL_STATUS_SOURCE" == external ]] ||
    fail "Tool absent from mise inventory did not use executable fallback"
  tool_status_detect mise gh linux amd64
  [[ "$TOOL_STATUS_INSTALLED" == missing && "$TOOL_STATUS_SOURCE" == none ]] ||
    fail "Tool absent from mise inventory was not reported missing"

  touch "$HOME/mise-fail"
  printf 'uv 0.12.13\ngh 2.100.0\n' >"$HOME/mise-inventory"
  tool_status_reset_cache
  tool_status_detect mise uv linux amd64
  [[ "$TOOL_STATUS_INSTALLED" == detected && "$TOOL_STATUS_SOURCE" == external ]] ||
    fail "Failed mise query did not discard partial output and use executable fallback"
  tool_status_detect mise gh linux amd64
  [[ "$TOOL_STATUS_INSTALLED" == missing && "$TOOL_STATUS_SOURCE" == none && "$TOOL_STATUS_APPROVED" == 2.100.0 ]] ||
    fail "Failed mise query did not preserve missing status and approved version"
  [[ "$(wc -l <"$HOME/mise-calls" | tr -d ' ')" == 2 ]] || fail "Failed mise inventory was queried again before reset"
}

test_mise_inventory_uses_managed_mise_outside_path() {
  setup_mise_inventory
  mkdir -p "$HOME/.local/bin"
  mv "$TEST_ROOT/bin/mise" "$HOME/.local/bin/mise"

  tool_status_detect mise node linux amd64
  [[ "$TOOL_STATUS_INSTALLED" == 24.18.0 && "$TOOL_STATUS_SOURCE" == mise ]] ||
    fail "Managed mise outside PATH was not used for inventory"
}

run_discovered_tests setup_tool_status_home teardown_tool_status_home
