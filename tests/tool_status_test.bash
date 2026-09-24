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

# Serves the case arms on stdin and logs each call to $HOME/brew-calls.
mock_brew() {
  {
    cat <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$HOME/brew-calls"
case "$*" in
EOF
    cat
    printf '  *) exit 1 ;;\nesac\n'
  } >"$TEST_ROOT/bin/brew"
  chmod +x "$TEST_ROOT/bin/brew"
}

link_host_jq() {
  local jq_command

  jq_command="$(PATH="$ORIGINAL_PATH" command -v jq || true)"
  [[ -n "$jq_command" ]] || skip "${FUNCNAME[1]} (jq unavailable)"
  ln -s "$jq_command" "$TEST_ROOT/bin/jq"
}

test_reuses_homebrew_inventory() {
  link_host_jq
  mock_brew <<'EOF'
  'list --versions --json')
    cat <<'JSON'
{"formulae":[{"name":"starship","versions":["1.26.0"],"linked_version":"1.26.0","optlinked_version":"1.26.0","pinned_version":null},{"name":"fzf","versions":["0.74.0"],"linked_version":"0.74.0","optlinked_version":"0.74.0","pinned_version":null}],"casks":[{"token":"ghostty","versions":["1.3.1"],"pinned_version":null},{"token":"font-meslo-lg-nerd-font","versions":["3.4.0"],"pinned_version":null}]}
JSON
    ;;
EOF

  tool_status_detect formula starship macos arm64
  [[ "$TOOL_STATUS_INSTALLED" == 1.26.0 && "$TOOL_STATUS_SOURCE" == homebrew ]] ||
    fail "Typed Homebrew inventory did not report the first formula version"
  tool_status_detect formula fzf macos arm64
  [[ "$TOOL_STATUS_INSTALLED" == 0.74.0 && "$TOOL_STATUS_SOURCE" == homebrew ]] ||
    fail "Typed Homebrew inventory did not report the second formula version"
  tool_status_detect cask ghostty macos arm64
  [[ "$TOOL_STATUS_INSTALLED" == 1.3.1 && "$TOOL_STATUS_SOURCE" == homebrew-cask ]] ||
    fail "Typed Homebrew inventory did not report the first cask version"
  tool_status_detect cask font-meslo-lg-nerd-font macos arm64

  [[ "$TOOL_STATUS_INSTALLED" == 3.4.0 && "$TOOL_STATUS_SOURCE" == homebrew-cask ]] ||
    fail "Typed Homebrew inventory did not report the cask version"
  [[ "$(<"$HOME/brew-calls")" == 'list --versions --json' ]] ||
    fail "Homebrew formulae and casks should use one typed inventory"
}

test_falls_back_when_homebrew_json_is_invalid() {
  link_host_jq
  mock_brew <<'EOF'
  'list --versions --json') printf 'invalid JSON\n' ;;
  'list --formula --versions') printf 'starship 1.26.0\n' ;;
  'list --cask --versions') printf 'ghostty 1.3.1\n' ;;
EOF

  tool_status_detect formula starship macos arm64
  [[ "$TOOL_STATUS_INSTALLED" == 1.26.0 && "$TOOL_STATUS_SOURCE" == homebrew ]] ||
    fail "Invalid JSON prevented formula fallback"
  tool_status_detect cask ghostty macos arm64
  [[ "$TOOL_STATUS_INSTALLED" == 1.3.1 && "$TOOL_STATUS_SOURCE" == homebrew-cask ]] ||
    fail "Invalid JSON prevented cask fallback"
  [[ "$(<"$HOME/brew-calls")" == $'list --versions --json\nlist --formula --versions\nlist --cask --versions' ]] ||
    fail "Invalid JSON did not reuse the legacy Homebrew inventories"
}

test_falls_back_when_homebrew_json_is_empty() {
  link_host_jq
  mock_brew <<'EOF'
  'list --versions --json') printf ' \n' ;;
  'list --cask --versions') printf 'ghostty 1.3.1\n' ;;
EOF

  tool_status_detect cask ghostty macos arm64
  [[ "$TOOL_STATUS_INSTALLED" == 1.3.1 && "$TOOL_STATUS_SOURCE" == homebrew-cask ]] ||
    fail "Empty JSON prevented cask fallback"
  [[ "$(<"$HOME/brew-calls")" == $'list --versions --json\nlist --cask --versions' ]] ||
    fail "Empty JSON did not use the legacy cask inventory"
}

test_uses_legacy_homebrew_inventory_without_jq() {
  ln -s "$(PATH="$ORIGINAL_PATH" command -v bash)" "$TEST_ROOT/bin/bash"
  mock_brew <<'EOF'
  'list --formula --versions') printf 'starship 1.26.0\n' ;;
  'list --cask --versions') printf 'ghostty 1.3.1\n' ;;
EOF

  PATH="$TEST_ROOT/bin" tool_status_detect formula starship macos arm64
  [[ "$TOOL_STATUS_INSTALLED" == 1.26.0 && "$TOOL_STATUS_SOURCE" == homebrew ]] ||
    fail "Formula was not detected without jq"
  PATH="$TEST_ROOT/bin" tool_status_detect cask ghostty macos arm64
  [[ "$TOOL_STATUS_INSTALLED" == 1.3.1 && "$TOOL_STATUS_SOURCE" == homebrew-cask ]] ||
    fail "Cask was not detected without jq"
  [[ "$(<"$HOME/brew-calls")" == $'list --formula --versions\nlist --cask --versions' ]] ||
    fail "Homebrew JSON was attempted without jq"
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
if [[ "$*" == current || "$*" == "-C $SELFISHELL_ROOT/config/shared current" ]]; then
  printf 'gh 2.100.0\n'
  exit 0
elif [[ "$*" != "-C $SELFISHELL_ROOT/config/shared ls --current --installed --no-header --no-truncate" ]]; then
  exit 1
fi
[[ "$MISE_GLOBAL_CONFIG_FILE" == "$SELFISHELL_ROOT/config/shared/mise.toml" ]] || exit 1
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
  printf 'node 24.18.0 /config/mise.toml 24.18.0\npython 3.13.14 /config/mise.toml 3.13.14\npython 3.12.0 /config/mise.toml 3.12.0\n' >"$HOME/mise-inventory"
}

test_configured_but_uninstalled_mise_tool_is_missing() {
  setup_mise_inventory
  # An existing mise shim does not prove the underlying tool is installed.
  export MISE_DATA_DIR="$TEST_ROOT/mise-data"
  mkdir -p "$MISE_DATA_DIR/shims"
  ln -s "$TEST_ROOT/bin/mise" "$MISE_DATA_DIR/shims/gh"
  export PATH="$MISE_DATA_DIR/shims:$PATH"

  tool_status_detect mise gh linux amd64
  [[ "$TOOL_STATUS_INSTALLED" == missing && "$TOOL_STATUS_SOURCE" == none ]] ||
    fail "Configured but uninstalled tool was reported present: $TOOL_STATUS_INSTALLED ($TOOL_STATUS_SOURCE)"
  [[ "$TOOL_STATUS_APPROVED" == 2.100.0 ]] || fail "Missing tool lost its approved version"
  unset MISE_DATA_DIR
}

# Approved mise versions come from mise.toml, not the manifest's bare tool names.
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
  # This test runs in isolation; keep its missing-tool case independent of CI's PATH.
  have_command() {
    [[ "$1" != gh ]] && command -v "$1" >/dev/null 2>&1
  }
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
