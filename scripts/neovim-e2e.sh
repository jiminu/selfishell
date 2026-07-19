#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/selfishell-neovim-e2e.XXXXXX")"

cleanup() {
  rm -rf "$TEST_ROOT"
}

fail() {
  printf 'Neovim E2E failed: %s\n' "$*" >&2
  exit 1
}

trap cleanup EXIT HUP INT TERM

command -v nvim >/dev/null 2>&1 || fail "Neovim is unavailable"
command -v tree-sitter >/dev/null 2>&1 || fail "Tree-sitter CLI is unavailable"
command -v git >/dev/null 2>&1 || fail "Git is unavailable"
command -v cc >/dev/null 2>&1 || fail "a C compiler is unavailable"

export HOME="$TEST_ROOT/home"
export XDG_CACHE_HOME="$HOME/.cache"
export XDG_CONFIG_HOME="$HOME/.config"
export XDG_DATA_HOME="$HOME/.local/share"
export XDG_STATE_HOME="$HOME/.local/state"
export NVIM_LOG_FILE="$TEST_ROOT/nvim.log"
export TMPDIR="$TEST_ROOT/tmp"
mkdir -p "$HOME" "$TMPDIR"

if zsh_path="$(command -v zsh 2>/dev/null)"; then
  export SHELL="$zsh_path"
fi

bash "$ROOT_DIR/bin/selfishell" install --profile developer --skip-packages --yes >/dev/null

bash -c '
  set -euo pipefail
  SELFISHELL_ROOT="$1"
  export SELFISHELL_ROOT
  source "$SELFISHELL_ROOT/lib/common.sh"
  source "$SELFISHELL_ROOT/lib/paths.sh"
  source "$SELFISHELL_ROOT/lib/dependencies.sh"
  source "$SELFISHELL_ROOT/lib/installers.sh"
  selfishell_initialize_paths
  install_neovim_plugins 0
' _ "$ROOT_DIR"

while read -r type repository revision _ _ source _; do
  [[ "$type" == nvim-plugin ]] || continue
  if [[ "$repository" == folke/lazy.nvim ]]; then
    plugin_dir="$XDG_DATA_HOME/selfishell/nvim/lazy/lazy.nvim"
  else
    plugin_name="${source##*/}"
    plugin_name="${plugin_name%.git}"
    plugin_dir="$XDG_DATA_HOME/nvim/lazy/$plugin_name"
  fi
  [[ -d "$plugin_dir/.git" ]] || fail "plugin checkout is missing: $repository"
  [[ "$(git -C "$plugin_dir" rev-parse HEAD)" == "$revision" ]] ||
    fail "plugin revision does not match: $repository"
done <"$ROOT_DIR/dependencies.conf"

[[ -r "$XDG_STATE_HOME/selfishell/nvim/lazy-lock.json" ]] || fail "lazy.nvim runtime lock is missing"
[[ ! -e "$XDG_CONFIG_HOME/selfishell/nvim/lazy-lock.json" ]] || fail "lazy.nvim lock polluted managed configuration"

for parser in bash gitcommit lua terraform typescript; do
  [[ -r "$XDG_DATA_HOME/nvim/site/parser/$parser.so" ]] || fail "Tree-sitter parser is missing: $parser"
done

printf 'terraform { required_version = ">= 1.0" }\n' >"$TEST_ROOT/main.tf"
if ! smoke_output="$(nvim --headless "$TEST_ROOT/main.tf" \
  '+lua local language = vim.treesitter.language.get_lang(vim.bo.filetype); local parser_ok, parser_error = pcall(vim.treesitter.get_parser, 0, language); assert(vim.bo.filetype == "tf" or vim.bo.filetype == "terraform", "unexpected filetype: " .. vim.bo.filetype); assert(language == "terraform", "unexpected language: " .. tostring(language)); assert(parser_ok, tostring(parser_error)); print("Neovim developer smoke: OK")' \
  +qa 2>&1)"; then
  printf '%s\n' "$smoke_output" >&2
  fail "Terraform Tree-sitter smoke failed"
fi
[[ "$smoke_output" == *'Neovim developer smoke: OK'* ]] || {
  printf '%s\n' "$smoke_output" >&2
  fail "Terraform Tree-sitter smoke did not complete"
}

printf 'PASS: pinned Neovim developer installation\n'
