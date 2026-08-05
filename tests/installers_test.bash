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
  if [[ "$1" == "-C" && "$3" == "rev-parse" && "$4" == "HEAD" ]]; then
    [[ -r "$2/.git/selfishell-approved-revision" ]] || return 1
    command cat "$2/.git/selfishell-approved-revision"
  elif [[ "$1" == "clone" ]]; then
    mkdir -p "${@: -1}"
  fi
}

verify_neovim_plugins() {
  NVIM_PLUGINS_VERIFIED=1
}

selfishell_nvim_treesitter_languages() {
  printf '%s\n' 'lua vim'
}

setup_installer_test() {
  setup_test_home
  export SELFISHELL_ROOT="$ROOT_DIR"
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

test_provisions_declared_zinit_plugins_without_loading_them() {
  local manifest
  local repository
  local revision
  local zinit_script

  manifest="$TEST_ROOT/dependencies.conf"
  zinit_script="$HOME/.local/share/zinit/zinit.git/zinit.zsh"
  mkdir -p "$(dirname "$zinit_script")"
  grep '^zsh-plugin ' "$ROOT_DIR/dependencies.conf" >"$manifest"
  cat >"$zinit_script" <<'EOF'
typeset -gi light_calls=0
typeset -g approved_revision
zinit() {
  print -r -- "$*" >>"$SELFISHELL_TEST_ZINIT_LOG"
  if [[ "$1" == ice ]]; then
    approved_revision="${3#ver}"
  elif [[ "$1" == light ]]; then
    light_calls=$((light_calls + 1))
    plugin_dir="${XDG_DATA_HOME:-$HOME/.local/share}/zinit/plugins/${2//\//---}"
    command mkdir -p "$plugin_dir/.git"
    print -r -- "$approved_revision" >"$plugin_dir/.git/selfishell-approved-revision"
    [[ "$light_calls" -eq 1 ]]
  fi
}
EOF
  export SELFISHELL_DEPENDENCIES_FILE="$manifest"
  export SELFISHELL_TEST_ZINIT_LOG="$TEST_ROOT/zinit-calls"

  install_zinit_plugins

  while read -r _ repository revision _; do
    grep -Fqx "ice cloneonly ver$revision" "$SELFISHELL_TEST_ZINIT_LOG" ||
      fail "$repository was not provisioned at its approved commit"
    grep -Fqx "light $repository" "$SELFISHELL_TEST_ZINIT_LOG" ||
      fail "$repository was not passed to Zinit"
  done <"$manifest"
  [[ "$(grep -c '^light ' "$SELFISHELL_TEST_ZINIT_LOG")" -eq 4 ]] ||
    fail "Installer did not provision exactly four Zsh plugins"
}

test_fails_when_zinit_plugin_provisioning_fails() {
  local manifest
  local status
  local zinit_script

  manifest="$TEST_ROOT/dependencies.conf"
  zinit_script="$HOME/.local/share/zinit/zinit.git/zinit.zsh"
  mkdir -p "$(dirname "$zinit_script")"
  grep '^zsh-plugin ' "$ROOT_DIR/dependencies.conf" >"$manifest"
  cat >"$zinit_script" <<'EOF'
typeset -gi light_calls=0
zinit() {
  if [[ "$1" == light ]]; then
    light_calls=$((light_calls + 1))
    [[ "$light_calls" -gt 1 ]]
  fi
}
EOF
  export SELFISHELL_DEPENDENCIES_FILE="$manifest"

  status="$(command bash -c '
    source "$SELFISHELL_ROOT/lib/common.sh"
    source "$SELFISHELL_ROOT/lib/dependencies.sh"
    source "$SELFISHELL_ROOT/lib/installers.sh"
    install_zinit_plugins
    printf "%s\n" "$?"
  ')"

  [[ "$status" -ne 0 ]] || fail "Zinit plugin provisioning failure was ignored"
}

test_cleans_failed_fresh_zinit_plugin_for_retry() {
  local manifest
  local plugin_dir
  local status=0
  local zinit_script

  manifest="$TEST_ROOT/dependencies.conf"
  zinit_script="$HOME/.local/share/zinit/zinit.git/zinit.zsh"
  plugin_dir="$HOME/.local/share/zinit/plugins/zsh-users---zsh-completions"
  mkdir -p "$(dirname "$zinit_script")"
  grep '^zsh-plugin ' "$ROOT_DIR/dependencies.conf" | head -n 1 >"$manifest"
  cat >"$zinit_script" <<'EOF'
typeset -g approved_revision
zinit() {
  if [[ "$1" == ice ]]; then
    approved_revision="${3#ver}"
  elif [[ "$1" == light ]]; then
    attempts=0
    [[ ! -r "$SELFISHELL_TEST_ZINIT_ATTEMPTS" ]] || attempts="$(<"$SELFISHELL_TEST_ZINIT_ATTEMPTS")"
    attempts=$((attempts + 1))
    print -r -- "$attempts" >"$SELFISHELL_TEST_ZINIT_ATTEMPTS"
    plugin_dir="${XDG_DATA_HOME:-$HOME/.local/share}/zinit/plugins/${2//\//---}"
    command mkdir -p "$plugin_dir/.git"
    if ((attempts > 1)); then
      print -r -- "$approved_revision" >"$plugin_dir/.git/selfishell-approved-revision"
    fi
  fi
}
EOF
  export SELFISHELL_DEPENDENCIES_FILE="$manifest"
  export SELFISHELL_TEST_ZINIT_ATTEMPTS="$TEST_ROOT/zinit-attempts"

  install_zinit_plugins || status=$?

  [[ "$status" -ne 0 ]] || fail "Installer accepted a fresh Zinit checkout without the approved revision"
  [[ ! -e "$plugin_dir" && ! -L "$plugin_dir" ]] || fail "Installer left a failed fresh Zinit checkout in place"

  install_zinit_plugins

  [[ -d "$plugin_dir/.git" ]] || fail "Installer could not retry Zinit provisioning after cleanup"
}

test_preserves_preexisting_zinit_plugin_path() {
  local manifest
  local plugin_dir
  local zinit_script

  manifest="$TEST_ROOT/dependencies.conf"
  zinit_script="$HOME/.local/share/zinit/zinit.git/zinit.zsh"
  plugin_dir="$HOME/.local/share/zinit/plugins/zsh-users---zsh-completions"
  mkdir -p "$(dirname "$zinit_script")" "$plugin_dir"
  printf '%s\n' 'user data' >"$plugin_dir/preserved"
  grep '^zsh-plugin ' "$ROOT_DIR/dependencies.conf" | head -n 1 >"$manifest"
  printf '%s\n' 'zinit() { :; }' >"$zinit_script"
  export SELFISHELL_DEPENDENCIES_FILE="$manifest"

  install_zinit_plugins

  [[ "$(<"$plugin_dir/preserved")" == 'user data' ]] || fail "Installer changed a pre-existing Zinit plugin path"
}

test_offline_mode_skips_zinit_plugin_provisioning() {
  export SELFISHELL_DEPENDENCIES_FILE="$TEST_ROOT/missing-dependencies.conf"

  SELFISHELL_OFFLINE=1 install_zinit_plugins
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

test_lazy_nvim_recovers_from_stale_temporary_path() {
  local lazy_path

  lazy_path="$HOME/.local/share/selfishell/nvim/lazy/lazy.nvim"
  rm -rf "$HOME/.local/share/selfishell/nvim"
  NVIM_CALLS=()
  GIT_CALLS=()

  # Simulates debris left by a process that was killed mid-install (a killed
  # prior run sharing this same PID): install_lazy_nvim must not hard-fail
  # just because its usual temporary name is already occupied.
  mkdir -p "${lazy_path}.tmp.$$"
  printf 'stale\n' >"${lazy_path}.tmp.$$/stale-marker"

  install_neovim_plugins 0

  [[ -d "$lazy_path" ]] || fail "lazy.nvim was not prepared despite a stale temporary path"
  [[ ! -e "$lazy_path/stale-marker" ]] ||
    fail "A stale temporary path leaked into the activated lazy.nvim checkout"
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

run_discovered_tests setup_installer_test teardown_test_home
