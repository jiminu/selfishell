#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/tests/test_helper.bash"

test_treesitter_autocmd_uses_detected_filetypes() {
  local output

  if ! command -v nvim >/dev/null 2>&1; then
    printf 'SKIP: test_treesitter_autocmd_uses_detected_filetypes (Neovim unavailable)\n'
    return
  fi

  output="$(NVIM_LOG_FILE="$TEST_ROOT/nvim.log" TMPDIR="$TEST_ROOT/tmp" nvim --headless -u NONE -i NONE \
    --cmd "set runtimepath^=$ROOT_DIR/common/nvim" \
    '+lua vim.treesitter.start = function(buf) vim.g.selfishell_test_filetype = vim.bo[buf].filetype end; require("config.autocmds"); vim.cmd("enew"); vim.bo.filetype = "sh"; vim.api.nvim_exec_autocmds("FileType", { buffer = 0 }); print(vim.g.selfishell_test_filetype); print(vim.treesitter.language.get_lang("tf"))' \
    +qa 2>&1)"

  [[ "$output" == *$'sh\nterraform'* ]] ||
    fail "Tree-sitter autocmd did not use FileType values and Terraform mapping: $output"
}

test_every_neovim_plugin_has_an_approved_revision() {
  local repository revision
  local plugin_count=0

  while IFS= read -r repository; do
    revision="$(awk -v repository="$repository" '$1 == "nvim-plugin" && $2 == repository { print $3 }' "$ROOT_DIR/dependencies.conf")"
    [[ "$revision" =~ ^[0-9a-f]{40}$ ]] ||
      fail "Neovim plugin is missing an approved revision: $repository"
    plugin_count=$((plugin_count + 1))
  done < <(sed -n 's/.*plugin("\([^"]*\)".*/\1/p' "$ROOT_DIR"/common/nvim/lua/plugins/*.lua | sort -u)

  [[ "$plugin_count" -eq 20 ]] || fail "Expected 20 pinned Neovim plugins, got $plugin_count"
  revision="$(awk '$1 == "nvim-plugin" && $2 == "folke/lazy.nvim" { print $3 }' "$ROOT_DIR/dependencies.conf")"
  [[ "$revision" =~ ^[0-9a-f]{40}$ ]] || fail "lazy.nvim is missing an approved revision"
}

test_pinned_neovim_plugin_specs_load() {
  local output

  if ! command -v nvim >/dev/null 2>&1; then
    printf 'SKIP: test_pinned_neovim_plugin_specs_load (Neovim unavailable)\n'
    return
  fi

  mkdir -p "$XDG_CONFIG_HOME/nvim"
  cp "$ROOT_DIR/dependencies.conf" "$XDG_CONFIG_HOME/nvim/plugin-versions.conf"
  output="$(NVIM_LOG_FILE="$TEST_ROOT/nvim.log" TMPDIR="$TEST_ROOT/tmp" nvim --headless -u NONE -i NONE \
    --cmd "set runtimepath^=$ROOT_DIR/common/nvim" \
    '+lua for _, module in ipairs({ "plugins.completion", "plugins.editor", "plugins.lsp", "plugins.telescope", "plugins.ui" }) do assert(type(require(module)) == "table", "Invalid plugin spec: " .. module) end; print("pinned plugin specs: OK")' \
    +qa 2>&1)"

  [[ "$output" == *'pinned plugin specs: OK'* ]] || fail "Pinned Neovim plugin specs did not load: $output"
}

test_mason_lsp_servers_are_versioned() {
  local server

  for server in lua_ls pyright bashls; do
    grep -Eq '"'"$server"'@[0-9]+\.[0-9]+\.[0-9]+"' "$ROOT_DIR/common/nvim/lua/config/languages.lua" ||
      fail "Mason LSP server is not versioned: $server"
  done
}

setup_test_home
trap teardown_test_home EXIT
export XDG_CACHE_HOME="$HOME/.cache"
export XDG_CONFIG_HOME="$HOME/.config"
export XDG_DATA_HOME="$HOME/.local/share"
export XDG_STATE_HOME="$HOME/.local/state"
mkdir -p "$TEST_ROOT/tmp" "$XDG_CACHE_HOME" "$XDG_CONFIG_HOME" "$XDG_DATA_HOME" "$XDG_STATE_HOME"

test_treesitter_autocmd_uses_detected_filetypes
printf 'PASS: test_treesitter_autocmd_uses_detected_filetypes\n'
test_every_neovim_plugin_has_an_approved_revision
printf 'PASS: test_every_neovim_plugin_has_an_approved_revision\n'
test_pinned_neovim_plugin_specs_load
printf 'PASS: test_pinned_neovim_plugin_specs_load\n'
test_mason_lsp_servers_are_versioned
printf 'PASS: test_mason_lsp_servers_are_versioned\n'

trap - EXIT
teardown_test_home
