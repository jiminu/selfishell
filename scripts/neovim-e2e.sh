#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/selfishell-neovim-e2e.XXXXXX")"
MISE_COMMAND="$(command -v mise 2>/dev/null || true)"
MISE_DATA_ROOT="${MISE_DATA_DIR:-$HOME/.local/share/mise}"

cleanup() {
  rm -rf "$TEST_ROOT"
}

fail() {
  printf 'Neovim E2E failed: %s\n' "$*" >&2
  exit 1
}

verify_mise_global_config_ownership() {
  local selfishell_mise_config
  local user_mise_config
  local mise_config_link
  local selfishell_mise_before

  selfishell_mise_config="$XDG_CONFIG_HOME/selfishell/mise/selfishell.toml"
  user_mise_config="$XDG_CONFIG_HOME/mise/config.toml"
  mise_config_link="$XDG_CONFIG_HOME/mise/conf.d/selfishell.toml"
  selfishell_mise_before="$TEST_ROOT/selfishell-mise-before.toml"

  unset MISE_GLOBAL_CONFIG_FILE
  unset MISE_DEFAULT_CONFIG_FILENAME
  unset MISE_OVERRIDE_CONFIG_FILENAMES

  [[ -f "$selfishell_mise_config" ]] ||
    fail "Selfishell-managed mise config is missing"

  [[ -f "$user_mise_config" ]] ||
    fail "User-owned mise global config is missing"

  [[ -L "$mise_config_link" ]] ||
    fail "Selfishell mise conf.d link is missing"

  cp "$selfishell_mise_config" "$selfishell_mise_before"

  "$MISE_COMMAND" settings set pin true >/dev/null

  [[ -f "$user_mise_config" ]] ||
    fail "mise global settings did not write to the user config"

  grep -Eq \
    '^[[:space:]]*pin[[:space:]]*=[[:space:]]*true[[:space:]]*$' \
    "$user_mise_config" ||
    fail "mise did not persist the global pin setting in the user config"

  cmp -s "$selfishell_mise_before" "$selfishell_mise_config" ||
    fail "mise global settings modified the Selfishell-managed config"

  [[ -L "$mise_config_link" ]] ||
    fail "mise global settings replaced the Selfishell conf.d link"

  [[ "$(readlink "$mise_config_link")" == "$selfishell_mise_config" ]] ||
    fail "mise global settings changed the Selfishell conf.d link target"
}

trap cleanup EXIT HUP INT TERM

command -v nvim >/dev/null 2>&1 || fail "Neovim is unavailable"
command -v tree-sitter >/dev/null 2>&1 || fail "Tree-sitter CLI is unavailable"
[[ -x "$MISE_COMMAND" ]] || fail "mise is unavailable"
command -v git >/dev/null 2>&1 || fail "Git is unavailable"
command -v cc >/dev/null 2>&1 || fail "a C compiler is unavailable"

export HOME="$TEST_ROOT/home"
export XDG_CACHE_HOME="$HOME/.cache"
export XDG_CONFIG_HOME="$HOME/.config"
export XDG_DATA_HOME="$HOME/.local/share"
export XDG_STATE_HOME="$HOME/.local/state"
export NVIM_LOG_FILE="$TEST_ROOT/nvim.log"
export TMPDIR="$TEST_ROOT/tmp"
export MISE_DATA_DIR="$MISE_DATA_ROOT"
mkdir -p "$HOME/.local/bin" "$TMPDIR"
ln -s "$MISE_COMMAND" "$HOME/.local/bin/mise"

if zsh_path="$(command -v zsh 2>/dev/null)"; then
  export SHELL="$zsh_path"
fi

bash "$ROOT_DIR/bin/selfishell" install --profile developer --skip-packages --yes >/dev/null

verify_mise_global_config_ownership

PATH=/usr/local/bin:/usr/bin:/bin bash -c '
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

treesitter_languages="$(bash -c 'source "$1/lib/installers.sh"; selfishell_nvim_treesitter_languages' _ "$ROOT_DIR")"
for parser in $treesitter_languages; do
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

printf 'def nested(value):\n    return {"items": [(value,)]}\n' >"$TEST_ROOT/main.py"
if ! python_smoke_output="$(nvim --headless "$TEST_ROOT/main.py" \
  '+lua local bufnr = vim.api.nvim_get_current_buf(); assert(vim.bo.filetype == "python", "unexpected filetype: " .. vim.bo.filetype); local parser_ok, parser_error = pcall(vim.treesitter.get_parser, bufnr, "python"); assert(parser_ok, tostring(parser_error)); assert(vim.treesitter.highlighter.active[bufnr], "Tree-sitter highlighting is not active for Python"); local rainbow = require("rainbow-delimiters.lib"); local attached = vim.wait(5000, function() local settings = rainbow.buffers[bufnr]; if not settings then return false end; local marks = vim.api.nvim_buf_get_extmarks(bufnr, rainbow.nsids.python, 0, -1, { details = true }); for _, mark in ipairs(marks) do local hl = mark[4].hl_group; if type(hl) == "string" and hl:find("RainbowDelimiter", 1, true) == 1 then return true end end return false end); assert(attached, "rainbow-delimiters did not highlight the initial Python buffer"); print("Python highlighting smoke: OK")' \
  +qa 2>&1)"; then
  printf '%s\n' "$python_smoke_output" >&2
  fail "Python Tree-sitter and rainbow-delimiters smoke failed"
fi
[[ "$python_smoke_output" == *'Python highlighting smoke: OK'* ]] || {
  printf '%s\n' "$python_smoke_output" >&2
  fail "Python highlighting smoke did not complete"
}

printf 'PASS: pinned Neovim developer installation\n'
