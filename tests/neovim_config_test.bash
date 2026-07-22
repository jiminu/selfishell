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
  # This headless nvim build emits CRLF line endings for print() on some
  # platforms (observed on WSL2); normalize before the line-spanning match.
  output="${output//$'\r'/}"

  [[ "$output" == *$'sh\nterraform'* ]] ||
    fail "Tree-sitter autocmd did not use FileType values and Terraform mapping: $output"
}

test_treesitter_install_rejects_false_and_missing_results() {
  local output

  if ! command -v nvim >/dev/null 2>&1; then
    printf 'SKIP: test_treesitter_install_rejects_false_and_missing_results (Neovim unavailable)\n'
    return
  fi

  output="$(NVIM_LOG_FILE="$TEST_ROOT/nvim.log" TMPDIR="$TEST_ROOT/tmp" nvim --headless -u NONE -i NONE \
    --cmd "set runtimepath^=$ROOT_DIR/common/nvim" \
    '+lua local function run(install_result, installed) package.loaded["nvim-treesitter"] = nil; package.preload["nvim-treesitter"] = function() return { install = function() return { wait = function() return install_result end } end, get_installed = function() return installed end } end; package.loaded["config.treesitter"] = nil; return pcall(require("config.treesitter").install, { "lua", "python" }) end; local ok, message = run(false, {}); assert(not ok and tostring(message):find("failed to install", 1, true), "false install result was accepted: " .. tostring(message)); ok, message = run(true, { "lua" }); assert(not ok and tostring(message):find("python", 1, true), "missing parser was accepted: " .. tostring(message)); ok, message = run(true, { "lua", "python" }); assert(ok, tostring(message)); print("Tree-sitter install verification: OK")' \
    +qa 2>&1)"

  [[ "$output" == *'Tree-sitter install verification: OK'* ]] ||
    fail "Tree-sitter install verification is incomplete: $output"
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

test_editor_workflow_options_and_keymaps() {
  local output

  if ! command -v nvim >/dev/null 2>&1; then
    printf 'SKIP: test_editor_workflow_options_and_keymaps (Neovim unavailable)\n'
    return
  fi

  mkdir -p "$XDG_CONFIG_HOME/nvim"
  cp "$ROOT_DIR/dependencies.conf" "$XDG_CONFIG_HOME/nvim/plugin-versions.conf"
  output="$(NVIM_LOG_FILE="$TEST_ROOT/nvim.log" TMPDIR="$TEST_ROOT/tmp" nvim --headless -u NONE -i NONE \
    --cmd "set runtimepath^=$ROOT_DIR/common/nvim" \
    '+lua vim.g.mapleader = " "; require("config.options"); require("config.keymaps"); assert(vim.o.splitright and vim.o.splitbelow, "split direction is not configured"); assert(vim.o.scrolloff == 4, "scrolloff is not configured"); assert(vim.o.confirm, "confirmation is not enabled"); assert(vim.o.inccommand == "split", "substitution preview is not configured"); local function assert_map(mode, lhs, rhs) local mapping = vim.fn.maparg(lhs, mode, false, true); assert(mapping.rhs == rhs, "unexpected mapping for " .. lhs .. ": " .. vim.inspect(mapping)) end; assert_map("n", "<leader>bd", "<cmd>confirm bdelete<CR>"); assert_map("x", "<", "<gv"); assert_map("x", ">", ">gv"); local function plugin_spec(module, repository) for _, spec in ipairs(require(module)) do if spec[1] == repository then return spec end end end; local function plugin_key(module, repository, lhs) local spec = plugin_spec(module, repository); for _, key in ipairs(spec and spec.keys or {}) do if key[1] == lhs then return key[2] end end end; local telescope = { ["<leader>fd"] = "<cmd>Telescope diagnostics<CR>", ["<leader>fs"] = "<cmd>Telescope lsp_document_symbols<CR>", ["<leader>fS"] = "<cmd>Telescope lsp_dynamic_workspace_symbols<CR>", ["<leader>fr"] = "<cmd>Telescope resume<CR>" }; for lhs, rhs in pairs(telescope) do assert(plugin_key("plugins.telescope", "nvim-telescope/telescope.nvim", lhs) == rhs, "missing Telescope mapping: " .. lhs) end; local tree = assert(plugin_spec("plugins.ui", "nvim-tree/nvim-tree.lua"), "nvim-tree spec is missing"); assert(tree.opts.view.width == 40, "unexpected NvimTree width"); assert(plugin_key("plugins.ui", "nvim-tree/nvim-tree.lua", "<leader>E") == "<cmd>NvimTreeFindFile<CR>", "missing current-file tree mapping"); local rainbow = assert(plugin_spec("plugins.editor", "HiPhish/rainbow-delimiters.nvim"), "rainbow-delimiters spec is missing"); assert(vim.deep_equal(rainbow.event, { "BufReadPre", "BufNewFile" }), "rainbow-delimiters loads after the initial FileType event"); print("editor workflows: OK")' \
    +qa 2>&1)"

  [[ "$output" == *'editor workflows: OK'* ]] || fail "Editor workflow configuration is invalid: $output"
}

test_lazy_revision_prefers_detached_head() {
  local lazy_path
  local output
  local revision

  if ! command -v nvim >/dev/null 2>&1; then
    printf 'SKIP: test_lazy_revision_prefers_detached_head (Neovim unavailable)\n'
    return
  fi

  revision="$(awk '$1 == "nvim-plugin" && $2 == "folke/lazy.nvim" { print $3 }' "$ROOT_DIR/dependencies.conf")"
  lazy_path="$XDG_DATA_HOME/selfishell/nvim/lazy/lazy.nvim"
  mkdir -p "$lazy_path/.git"
  printf '%s\n' "$revision" >"$lazy_path/.git/HEAD"

  output="$(SELFISHELL_TEST_REVISION="$revision" NVIM_LOG_FILE="$TEST_ROOT/nvim.log" TMPDIR="$TEST_ROOT/tmp" \
    nvim --headless -u NONE -i NONE \
    --cmd "set runtimepath^=$ROOT_DIR/common/nvim" \
    '+lua package.preload["config.plugin_versions"] = function() return { revision = function() return vim.env.SELFISHELL_TEST_REVISION end } end; package.preload["lazy"] = function() return { setup = function() vim.g.selfishell_lazy_setup = true end } end; vim.fn.system = function() error("Git fallback should not run for a detached HEAD") end; require("config.lazy"); print(vim.g.selfishell_lazy_setup)' \
    +qa 2>&1)"

  [[ "$output" == *'true'* ]] || fail "lazy.nvim did not use its detached HEAD directly: $output"
}

test_lazy_revision_falls_back_for_symbolic_head() {
  local lazy_path
  local output
  local revision

  if ! command -v nvim >/dev/null 2>&1; then
    printf 'SKIP: test_lazy_revision_falls_back_for_symbolic_head (Neovim unavailable)\n'
    return
  fi

  lazy_path="$XDG_DATA_HOME/selfishell/nvim/lazy/lazy.nvim"
  rm -rf "$lazy_path/.git"
  git -C "$lazy_path" init --quiet
  git -C "$lazy_path" -c user.name=Selfishell -c user.email=selfishell@example.invalid \
    commit --allow-empty --quiet --message test
  revision="$(git -C "$lazy_path" rev-parse HEAD)"
  grep -Eq '^ref: refs/heads/' "$lazy_path/.git/HEAD" ||
    fail "Test repository did not create a symbolic HEAD"

  output="$(SELFISHELL_TEST_REVISION="$revision" NVIM_LOG_FILE="$TEST_ROOT/nvim.log" TMPDIR="$TEST_ROOT/tmp" \
    nvim --headless -u NONE -i NONE \
    --cmd "set runtimepath^=$ROOT_DIR/common/nvim" \
    '+lua package.preload["config.plugin_versions"] = function() return { revision = function() return vim.env.SELFISHELL_TEST_REVISION end } end; package.preload["lazy"] = function() return { setup = function() vim.g.selfishell_lazy_setup = true end } end; require("config.lazy"); print(vim.g.selfishell_lazy_setup)' \
    +qa 2>&1)"

  [[ "$output" == *'true'* ]] ||
    fail "lazy.nvim did not fall back to Git for a symbolic HEAD: $output"
}

test_lsp_loads_only_for_supported_filetypes() {
  local output

  if ! command -v nvim >/dev/null 2>&1; then
    printf 'SKIP: test_lsp_loads_only_for_supported_filetypes (Neovim unavailable)\n'
    return
  fi

  mkdir -p "$XDG_CONFIG_HOME/nvim"
  cp "$ROOT_DIR/dependencies.conf" "$XDG_CONFIG_HOME/nvim/plugin-versions.conf"
  output="$(NVIM_LOG_FILE="$TEST_ROOT/nvim.log" TMPDIR="$TEST_ROOT/tmp" nvim --headless -u NONE -i NONE \
    --cmd "set runtimepath^=$ROOT_DIR/common/nvim" \
    '+lua local languages = require("config.languages"); local target; for _, spec in ipairs(require("plugins.lsp")) do if spec[1] == "mason-org/mason-lspconfig.nvim" then target = spec end end; assert(target, "mason-lspconfig spec is missing"); assert(vim.deep_equal(target.ft, languages.lsp_filetypes), "LSP filetypes are not centralized"); for _, filetype in ipairs({ "lua", "python", "sh", "bash" }) do assert(vim.tbl_contains(target.ft, filetype), "supported LSP filetype is missing: " .. filetype) end; assert(not vim.tbl_contains(target.ft, "terraform"), "Terraform should not load unsupported LSP plugins"); assert(not vim.tbl_contains(target.ft, "zsh"), "Zsh should not load unsupported LSP plugins"); print("LSP filetype scope: OK")' \
    +qa 2>&1)"

  [[ "$output" == *'LSP filetype scope: OK'* ]] || fail "LSP filetype scope is invalid: $output"
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
test_treesitter_install_rejects_false_and_missing_results
printf 'PASS: test_treesitter_install_rejects_false_and_missing_results\n'
test_every_neovim_plugin_has_an_approved_revision
printf 'PASS: test_every_neovim_plugin_has_an_approved_revision\n'
test_pinned_neovim_plugin_specs_load
printf 'PASS: test_pinned_neovim_plugin_specs_load\n'
test_editor_workflow_options_and_keymaps
printf 'PASS: test_editor_workflow_options_and_keymaps\n'
test_lazy_revision_prefers_detached_head
printf 'PASS: test_lazy_revision_prefers_detached_head\n'
test_lazy_revision_falls_back_for_symbolic_head
printf 'PASS: test_lazy_revision_falls_back_for_symbolic_head\n'
test_lsp_loads_only_for_supported_filetypes
printf 'PASS: test_lsp_loads_only_for_supported_filetypes\n'
test_mason_lsp_servers_are_versioned
printf 'PASS: test_mason_lsp_servers_are_versioned\n'

trap - EXIT
teardown_test_home
