#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/tests/test_helper.bash"
source "$ROOT_DIR/lib/common.sh"
source "$ROOT_DIR/lib/dependencies.sh"
source "$ROOT_DIR/lib/installers.sh"

NVIM_ARGUMENTS=""
NVIM_TREESITTER_LANGUAGES=""
MISE_ARGUMENTS=""
MISE_CONFIG=""
GIT_ARGUMENTS=""
GIT_CALLS=()
NVIM_CALLS=()
FAKE_NVIM_PATH=""

nvim() {
  NVIM_ARGUMENTS="$*"
  NVIM_TREESITTER_LANGUAGES="${SELFISHELL_NVIM_TREESITTER_LANGUAGES:-}"
  NVIM_CALLS+=("$*")
}

mise() {
  MISE_ARGUMENTS="$*"
  MISE_CONFIG="$MISE_GLOBAL_CONFIG_FILE"
  if [[ "$1" == "which" && "$2" == "nvim" ]]; then
    printf '%s\n' "$FAKE_NVIM_PATH"
  fi
}

git() {
  GIT_ARGUMENTS="$*"
  GIT_CALLS+=("$*")
  if [[ "$1" == "clone" ]]; then
    mkdir -p "${@: -1}"
  fi
}

selfishell_nvim_treesitter_languages() {
  printf '%s\n' 'lua vim'
}

test_default_treesitter_languages_match_supported_parsers() {
  local languages

  languages="$(bash -c 'source "$1/lib/installers.sh"; selfishell_nvim_treesitter_languages' _ "$ROOT_DIR")"

  for language in gitcommit git_rebase git_config gitignore gitattributes diff; do
    [[ " $languages " == *" $language "* ]] ||
      fail "Default Tree-sitter languages are missing Git parser: $language"
  done
  for language in jsonc markdown_inline helm; do
    [[ " $languages " != *" $language "* ]] ||
      fail "Default Tree-sitter languages include redundant parser: $language"
  done
}

test_installs_declared_mise_tools_with_managed_config() {
  # shellcheck disable=SC2034 # Read by install_mise_tools in the sourced module.
  SELFISHELL_SKIPPED_OPTIONAL_PACKAGES=()
  install_mise_tools required 0 node@24.18.0 python@3.13.14

  [[ "$MISE_ARGUMENTS" == 'install node@24.18.0 python@3.13.14' ]] ||
    fail "mise tools were not installed together"
  [[ "$MISE_CONFIG" == "$ROOT_DIR/common/mise.toml" ]] ||
    fail "mise install did not use the Selfishell config"
}

test_installs_declared_neovim_plugins() {
  NVIM_CALLS=()
  install_neovim_plugins 0
  [[ "${NVIM_CALLS[0]}" == *'pcall(vim.cmd, "Lazy! sync")'* ]] ||
    fail "Neovim plugin installation was not invoked"
  [[ "${NVIM_CALLS[1]}" == *'require("nvim-treesitter").install(languages):wait(300000)'* ]] ||
    fail "Current Tree-sitter parser installation API was not invoked"
  [[ "$NVIM_TREESITTER_LANGUAGES" == 'lua vim' ]] ||
    fail "Tree-sitter parser languages were not passed to Neovim"
}

test_offline_mode_skips_neovim_plugins() {
  NVIM_ARGUMENTS=""
  SELFISHELL_OFFLINE=1 install_neovim_plugins 0
  [[ -z "$NVIM_ARGUMENTS" ]] || fail "Offline mode invoked Neovim plugin installation"
}

test_minimal_profile_installs_vimrc() {
  local vimrc_path="$HOME/.config/selfishell/vim/vimrc"

  export SELFISHELL_ROOT="$ROOT_DIR"
  source "$ROOT_DIR/lib/common.sh"
  source "$ROOT_DIR/lib/paths.sh"
  source "$ROOT_DIR/lib/resources.sh"
  source "$ROOT_DIR/lib/installers.sh"

  selfishell_initialize_paths
  install_managed_configuration "ubuntu" 0 0

  cmp -s "$ROOT_DIR/common/vimrc" "$vimrc_path" ||
    fail "Vim configuration was not installed for the minimal profile"
  [[ -L "$HOME/.config/vim/vimrc" ]] ||
    fail "User vimrc link was not created"
}

test_neovim_plugin_dry_run_is_non_mutating() {
  local output
  NVIM_ARGUMENTS=""
  output="$(install_neovim_plugins 1)"
  [[ "$output" == *'Would install declared Neovim plugins.'* ]] ||
    fail "Neovim plugin dry run was not reported"
  [[ "$output" == *'Would install lazy.nvim bootstrap repository.'* ]] ||
    fail "lazy.nvim dry run was not reported"
  [[ "$output" == *'Would install Tree-sitter parsers.'* ]] ||
    fail "Tree-sitter dry run was not reported"
  [[ -z "$NVIM_ARGUMENTS" ]] || fail "Neovim plugin dry run invoked Neovim"
}

test_installs_lazy_nvim_before_syncing_plugins() {
  local lazy_path

  lazy_path="$HOME/.local/share/selfishell/nvim/lazy/lazy.nvim"
  rm -rf "$HOME/.local/share/selfishell/nvim"
  NVIM_CALLS=()
  GIT_CALLS=()

  install_neovim_plugins 0

  [[ -d "$lazy_path" ]] || fail "lazy.nvim was not prepared before plugin sync"
  [[ "${GIT_CALLS[*]}" == *'clone --quiet --filter=blob:none https://github.com/folke/lazy.nvim.git'* ]] ||
    fail "lazy.nvim bootstrap did not clone the approved repository"
  [[ "${GIT_CALLS[*]}" == *'checkout --quiet --detach 306a05526ada86a7b30af95c5cc81ffba93fef97'* ]] ||
    fail "lazy.nvim bootstrap did not check out the approved revision"
  [[ "${NVIM_CALLS[*]}" == *'Lazy! sync'* ]] || fail "Neovim plugin sync did not run"
  [[ "${NVIM_CALLS[*]}" == *'require("nvim-treesitter").install(languages):wait(300000)'* ]] ||
    fail "Tree-sitter parsers were not prepared"
}

test_installs_neovim_plugins_via_mise_resolution() {
  local fake_bin
  local original_path

  fake_bin="$HOME/fake-bin"
  mkdir -p "$fake_bin"
  FAKE_NVIM_PATH="$HOME/.local/share/mise/installs/neovim/0.12.4/bin/nvim"
  command mkdir -p "$(dirname "$FAKE_NVIM_PATH")"
  printf '#!/usr/bin/env bash\nexit 0\n' >"$FAKE_NVIM_PATH"
  chmod +x "$FAKE_NVIM_PATH"
  cat >"$fake_bin/mise" <<EOF
#!/bin/sh
if [ "\$1" = "which" ] && [ "\$2" = "nvim" ]; then
  printf '%s\n' "$FAKE_NVIM_PATH"
fi
EOF
  chmod +x "$fake_bin/mise"
  cat >"$fake_bin/nvim" <<'EOF'
#!/bin/sh
exit 0
EOF
  chmod +x "$fake_bin/nvim"
  original_path="$PATH"
  PATH="$fake_bin:$PATH"
  hash -r

  [[ "$(selfishell_nvim_command)" == "$FAKE_NVIM_PATH" ]] ||
    fail "mise fallback was not used to resolve Neovim"
  PATH="$original_path"
}

test_skips_neovim_plugins_when_neovim_is_unavailable() {
  local lazy_path

  lazy_path="$HOME/.local/share/selfishell/nvim/lazy/lazy.nvim"
  rm -rf "$HOME/.local/share/selfishell/nvim"
  GIT_ARGUMENTS=""
  selfishell_nvim_command() {
    return 1
  }

  install_neovim_plugins 0

  [[ -z "$GIT_ARGUMENTS" ]] || fail "Missing Neovim still cloned lazy.nvim"
  [[ ! -e "$lazy_path" ]] || fail "Missing Neovim still prepared lazy.nvim"
}

setup_test_home
export SELFISHELL_ROOT="$ROOT_DIR"

test_default_treesitter_languages_match_supported_parsers
printf 'PASS: test_default_treesitter_languages_match_supported_parsers\n'
test_installs_declared_mise_tools_with_managed_config
printf 'PASS: test_installs_declared_mise_tools_with_managed_config\n'
test_installs_declared_neovim_plugins
printf 'PASS: test_installs_declared_neovim_plugins\n'
test_offline_mode_skips_neovim_plugins
printf 'PASS: test_offline_mode_skips_neovim_plugins\n'
test_neovim_plugin_dry_run_is_non_mutating
printf 'PASS: test_neovim_plugin_dry_run_is_non_mutating\n'
test_installs_lazy_nvim_before_syncing_plugins
printf 'PASS: test_installs_lazy_nvim_before_syncing_plugins\n'
test_installs_neovim_plugins_via_mise_resolution
printf 'PASS: test_installs_neovim_plugins_via_mise_resolution\n'
test_skips_neovim_plugins_when_neovim_is_unavailable
printf 'PASS: test_skips_neovim_plugins_when_neovim_is_unavailable\n'

teardown_test_home
