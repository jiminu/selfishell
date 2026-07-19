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
NVIM_PLUGINS_VERIFIED=0

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
  elif [[ "$1" == "exec" && "$2" == "--" ]]; then
    shift 2
    "$@"
  fi
}

git() {
  GIT_ARGUMENTS="$*"
  GIT_CALLS+=("$*")
  if [[ "$1" == "clone" ]]; then
    mkdir -p "${@: -1}"
  fi
}

verify_neovim_plugins() {
  NVIM_PLUGINS_VERIFIED=1
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
  [[ "${NVIM_CALLS[1]}" == *'require("config.treesitter").install(languages)'* ]] ||
    fail "Current Tree-sitter parser installation API was not invoked"
  [[ "$NVIM_TREESITTER_LANGUAGES" == 'lua vim' ]] ||
    fail "Tree-sitter parser languages were not passed to Neovim"
  [[ "$NVIM_PLUGINS_VERIFIED" == "1" ]] || fail "Installed Neovim plugin revisions were not verified"
}

test_runs_neovim_inside_mise_environment() {
  MISE_ARGUMENTS=""
  MISE_CONFIG=""
  NVIM_CALLS=()
  FAKE_NVIM_PATH=""

  selfishell_run_nvim nvim --headless +qa

  [[ "$MISE_ARGUMENTS" == 'exec -- nvim --headless +qa' ]] ||
    fail "Neovim did not run through mise exec: $MISE_ARGUMENTS"
  [[ "$MISE_CONFIG" == "$ROOT_DIR/common/mise.toml" ]] ||
    fail "Neovim mise environment did not use the Selfishell config"
  [[ "${NVIM_CALLS[0]}" == '--headless +qa' ]] ||
    fail "mise exec did not invoke Neovim: ${NVIM_CALLS[*]}"
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
  [[ "${NVIM_CALLS[*]}" == *'require("config.treesitter").install(languages)'* ]] ||
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

test_resolves_neovim_with_managed_mise_outside_path() {
  local config_file
  local managed_mise
  local managed_nvim
  local output

  managed_mise="$HOME/.local/bin/mise"
  managed_nvim="$HOME/.local/share/mise/installs/neovim/0.12.4/bin/nvim"
  config_file="$HOME/mise-config-used"
  command mkdir -p "$(dirname "$managed_mise")" "$(dirname "$managed_nvim")"
  # Variables in these lines must expand when the generated mise stub runs.
  # shellcheck disable=SC2016
  printf '%s\n' \
    '#!/bin/sh' \
    'printf '\''%s\n'\'' "$MISE_GLOBAL_CONFIG_FILE" >"$SELFISHELL_TEST_MISE_CONFIG_FILE"' \
    'printf '\''%s\n'\'' "$SELFISHELL_TEST_NVIM_PATH"' >"$managed_mise"
  printf '%s\n' '#!/bin/sh' 'exit 0' >"$managed_nvim"
  chmod +x "$managed_mise" "$managed_nvim"

  output="$(
    HOME="$HOME" \
      PATH=/usr/bin:/bin \
      SELFISHELL_ROOT="$ROOT_DIR" \
      SELFISHELL_TEST_MISE_CONFIG_FILE="$config_file" \
      SELFISHELL_TEST_NVIM_PATH="$managed_nvim" \
      bash -c '
        source "$SELFISHELL_ROOT/lib/common.sh"
        source "$SELFISHELL_ROOT/lib/installers.sh"
        selfishell_nvim_command
      '
  )"

  [[ "$output" == "$managed_nvim" ]] ||
    fail "Managed mise outside PATH did not resolve Neovim: $output"
  [[ "$(<"$config_file")" == "$ROOT_DIR/common/mise.toml" ]] ||
    fail "Neovim resolution did not use the Selfishell mise config"
}

test_fails_neovim_plugins_when_neovim_is_unavailable() {
  local lazy_path
  local output

  lazy_path="$HOME/.local/share/selfishell/nvim/lazy/lazy.nvim"
  rm -rf "$HOME/.local/share/selfishell/nvim"
  GIT_ARGUMENTS=""
  selfishell_nvim_command() {
    return 1
  }

  if output="$(install_neovim_plugins 0 2>&1)"; then
    fail "Missing Neovim did not fail plugin installation"
  fi

  [[ -z "$GIT_ARGUMENTS" ]] || fail "Missing Neovim still cloned lazy.nvim"
  [[ ! -e "$lazy_path" ]] || fail "Missing Neovim still prepared lazy.nvim"
  [[ "$output" == *'Could not locate Neovim after installing the developer profile.'* ]] ||
    fail "Missing Neovim failure was not actionable: $output"
}

setup_test_home
export SELFISHELL_ROOT="$ROOT_DIR"

test_default_treesitter_languages_match_supported_parsers
printf 'PASS: test_default_treesitter_languages_match_supported_parsers\n'
test_installs_declared_mise_tools_with_managed_config
printf 'PASS: test_installs_declared_mise_tools_with_managed_config\n'
test_installs_declared_neovim_plugins
printf 'PASS: test_installs_declared_neovim_plugins\n'
test_runs_neovim_inside_mise_environment
printf 'PASS: test_runs_neovim_inside_mise_environment\n'
test_offline_mode_skips_neovim_plugins
printf 'PASS: test_offline_mode_skips_neovim_plugins\n'
test_neovim_plugin_dry_run_is_non_mutating
printf 'PASS: test_neovim_plugin_dry_run_is_non_mutating\n'
test_installs_lazy_nvim_before_syncing_plugins
printf 'PASS: test_installs_lazy_nvim_before_syncing_plugins\n'
test_installs_neovim_plugins_via_mise_resolution
printf 'PASS: test_installs_neovim_plugins_via_mise_resolution\n'
test_resolves_neovim_with_managed_mise_outside_path
printf 'PASS: test_resolves_neovim_with_managed_mise_outside_path\n'
test_fails_neovim_plugins_when_neovim_is_unavailable
printf 'PASS: test_fails_neovim_plugins_when_neovim_is_unavailable\n'

teardown_test_home
