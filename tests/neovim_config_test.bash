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

  [[ "$plugin_count" -eq 23 ]] || fail "Expected 23 pinned Neovim plugins, got $plugin_count"
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
    '+lua vim.g.mapleader = " "; require("config.options"); require("config.keymaps"); assert(vim.o.splitright and vim.o.splitbelow, "split direction is not configured"); assert(vim.o.scrolloff == 4, "scrolloff is not configured"); assert(vim.o.confirm, "confirmation is not enabled"); assert(vim.o.inccommand == "split", "substitution preview is not configured"); local function assert_map(mode, lhs, rhs) local mapping = vim.fn.maparg(lhs, mode, false, true); assert(mapping.rhs == rhs, "unexpected mapping for " .. lhs .. ": " .. vim.inspect(mapping)) end; for lhs, rhs in pairs({ ["<C-h>"] = "<C-W>h", ["<C-j>"] = "<C-W>j", ["<C-k>"] = "<C-W>k", ["<C-l>"] = "<C-W>l" }) do assert_map("n", lhs, rhs) end; local delete_map = vim.fn.maparg("<leader>bd", "n", false, true); assert(type(delete_map.callback) == "function", "buffer delete mapping is not callback-based"); assert_map("x", "<", "<gv"); assert_map("x", ">", ">gv"); local function plugin_spec(module, repository) for _, spec in ipairs(require(module)) do if spec[1] == repository then return spec end end end; local function plugin_key(module, repository, lhs) local spec = plugin_spec(module, repository); for _, key in ipairs(spec and spec.keys or {}) do if key[1] == lhs then return key[2] end end end; local telescope = { ["<leader>fd"] = "<cmd>Telescope diagnostics<CR>", ["<leader>fs"] = "<cmd>Telescope lsp_document_symbols<CR>", ["<leader>fS"] = "<cmd>Telescope lsp_dynamic_workspace_symbols<CR>", ["<leader>fr"] = "<cmd>Telescope resume<CR>" }; for lhs, rhs in pairs(telescope) do assert(plugin_key("plugins.telescope", "nvim-telescope/telescope.nvim", lhs) == rhs, "missing Telescope mapping: " .. lhs) end; local tree = assert(plugin_spec("plugins.ui", "nvim-tree/nvim-tree.lua"), "nvim-tree spec is missing"); assert(type(tree.opts.view.width) == "function", "NvimTree width is not a function"); local original_columns = vim.o.columns; vim.o.columns = 60; assert(tree.opts.view.width() == 20, "width should clamp to the 20-column minimum: " .. tostring(tree.opts.view.width())); vim.o.columns = 100; assert(tree.opts.view.width() == 25, "width should scale to 25% of columns: " .. tostring(tree.opts.view.width())); vim.o.columns = 200; assert(tree.opts.view.width() == 30, "width should clamp to the 30-column maximum: " .. tostring(tree.opts.view.width())); vim.o.columns = original_columns; assert(type(tree.opts.on_attach) == "function", "NvimTree does not preserve window navigation mappings"); assert(plugin_key("plugins.ui", "nvim-tree/nvim-tree.lua", "<leader>E") == "<cmd>NvimTreeFindFile<CR>", "missing current-file tree mapping"); local bufferline = assert(plugin_spec("plugins.ui", "akinsho/bufferline.nvim"), "bufferline spec is missing"); assert(bufferline.event == "VeryLazy", "bufferline is not deferred"); assert(bufferline.opts.options.always_show_bufferline == false, "bufferline should hide for one buffer"); assert(bufferline.opts.options.offsets[1].filetype == "NvimTree", "bufferline is not aligned with NvimTree"); assert(plugin_key("plugins.ui", "akinsho/bufferline.nvim", "[b") == "<cmd>BufferLineCyclePrev<CR>", "missing previous-buffer mapping"); assert(plugin_key("plugins.ui", "akinsho/bufferline.nvim", "]b") == "<cmd>BufferLineCycleNext<CR>", "missing next-buffer mapping"); local rainbow = assert(plugin_spec("plugins.editor", "HiPhish/rainbow-delimiters.nvim"), "rainbow-delimiters spec is missing"); assert(vim.deep_equal(rainbow.event, { "BufReadPre", "BufNewFile" }), "rainbow-delimiters loads after the initial FileType event"); print("editor workflows: OK")' \
    +qa 2>&1)"

  [[ "$output" == *'editor workflows: OK'* ]] || fail "Editor workflow configuration is invalid: $output"
}

test_buffer_delete_preserves_editor_window() {
  local output

  if ! command -v nvim >/dev/null 2>&1; then
    printf 'SKIP: test_buffer_delete_preserves_editor_window (Neovim unavailable)\n'
    return
  fi

  output="$(NVIM_LOG_FILE="$TEST_ROOT/nvim.log" TMPDIR="$TEST_ROOT/tmp" nvim --headless -u NONE -i NONE \
    --cmd "set runtimepath^=$ROOT_DIR/common/nvim" \
    '+lua vim.g.mapleader = " "; vim.bo.buflisted = false; local buffers = require("config.keymaps"); local editor_win = vim.api.nvim_get_current_win(); local tree_buf = vim.api.nvim_create_buf(false, true); vim.bo[tree_buf].filetype = "NvimTree"; local tree_win = vim.api.nvim_open_win(tree_buf, false, { split = "left", win = editor_win }); local first = vim.api.nvim_create_buf(true, false); local second = vim.api.nvim_create_buf(true, false); vim.api.nvim_win_set_buf(editor_win, first); buffers.delete_buffer(first); assert(vim.api.nvim_win_is_valid(editor_win), "editor window was closed"); assert(vim.api.nvim_win_get_buf(editor_win) == second, "next listed buffer did not replace the deleted buffer"); assert(vim.api.nvim_win_is_valid(tree_win) and vim.api.nvim_win_get_buf(tree_win) == tree_buf, "NvimTree window was changed"); buffers.delete_buffer(second); assert(vim.api.nvim_win_is_valid(editor_win), "editor window was closed with the last file buffer"); assert(vim.api.nvim_win_get_buf(editor_win) ~= tree_buf, "NvimTree replaced the editor buffer"); print("buffer delete layout: OK")' \
    +qa 2>&1)"

  [[ "$output" == *'buffer delete layout: OK'* ]] ||
    fail "Buffer deletion did not preserve the editor layout: $output"
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

test_last_cursor_restore_targets_correct_window_and_skips_invalid_cases() {
  local output

  if ! command -v nvim >/dev/null 2>&1; then
    printf 'SKIP: test_last_cursor_restore_targets_correct_window_and_skips_invalid_cases (Neovim unavailable)\n'
    return
  fi

  output="$(NVIM_LOG_FILE="$TEST_ROOT/nvim.log" TMPDIR="$TEST_ROOT/tmp" nvim --headless -u NONE -i NONE \
    --cmd "set runtimepath^=$ROOT_DIR/common/nvim" \
    '+lua local ok, err = pcall(function() require("config.autocmds"); local current_win = vim.api.nvim_get_current_win(); local buf_b = vim.api.nvim_get_current_buf(); vim.api.nvim_buf_set_lines(buf_b, 0, -1, false, { "a", "b", "c" }); vim.api.nvim_win_set_cursor(current_win, { 1, 0 }); local buf_a = vim.api.nvim_create_buf(true, false); vim.api.nvim_buf_set_lines(buf_a, 0, -1, false, { "0123456789", "0123456789", "0123456789", "0123456789", "0123456789" }); vim.api.nvim_buf_set_mark(buf_a, [["]], 3, 2, {}); local win_a = vim.api.nvim_open_win(buf_a, false, { relative = "editor", width = 10, height = 5, row = 0, col = 0 }); vim.api.nvim_win_set_cursor(win_a, { 1, 0 }); vim.api.nvim_exec_autocmds("BufReadPost", { buffer = buf_a }); local a_cursor = vim.api.nvim_win_get_cursor(win_a); local b_cursor = vim.api.nvim_win_get_cursor(current_win); assert(a_cursor[1] == 3 and a_cursor[2] == 2, "cursor was not restored in the window displaying the target buffer: " .. vim.inspect(a_cursor)); assert(b_cursor[1] == 1 and b_cursor[2] == 0, "cursor moved in an unrelated window: " .. vim.inspect(b_cursor)); local original_get_mark = vim.api.nvim_buf_get_mark; local buf_c = vim.api.nvim_create_buf(true, false); vim.api.nvim_buf_set_lines(buf_c, 0, -1, false, { "only one line" }); vim.api.nvim_buf_get_mark = function(buf, name) if buf == buf_c and name == [["]] then return { 999, 0 } end return original_get_mark(buf, name) end; local win_c = vim.api.nvim_open_win(buf_c, false, { relative = "editor", width = 10, height = 5, row = 0, col = 20 }); vim.api.nvim_win_set_cursor(win_c, { 1, 0 }); vim.api.nvim_exec_autocmds("BufReadPost", { buffer = buf_c }); local c_cursor = vim.api.nvim_win_get_cursor(win_c); assert(c_cursor[1] == 1 and c_cursor[2] == 0, "an out-of-range mark moved the cursor: " .. vim.inspect(c_cursor)); vim.api.nvim_buf_get_mark = original_get_mark; local buf_special = vim.api.nvim_create_buf(false, true); vim.api.nvim_buf_set_lines(buf_special, 0, -1, false, { "x", "y", "z" }); vim.api.nvim_buf_set_mark(buf_special, [["]], 2, 0, {}); local win_special = vim.api.nvim_open_win(buf_special, false, { relative = "editor", width = 10, height = 5, row = 0, col = 40 }); vim.api.nvim_win_set_cursor(win_special, { 1, 0 }); vim.api.nvim_exec_autocmds("BufReadPost", { buffer = buf_special }); local special_cursor = vim.api.nvim_win_get_cursor(win_special); assert(special_cursor[1] == 1 and special_cursor[2] == 0, "a special buffer moved the cursor: " .. vim.inspect(special_cursor)) end); for _, buf in ipairs(vim.api.nvim_list_bufs()) do pcall(function() vim.bo[buf].modified = false end) end; print(ok and "cursor restore targeting: OK" or ("cursor restore targeting: FAIL " .. tostring(err)))' \
    +qa 2>&1)"

  [[ "$output" == *'cursor restore targeting: OK'* ]] ||
    fail "Last-cursor-position restore did not target windows correctly: $output"
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
test_buffer_delete_preserves_editor_window
printf 'PASS: test_buffer_delete_preserves_editor_window\n'
test_lazy_revision_prefers_detached_head
printf 'PASS: test_lazy_revision_prefers_detached_head\n'
test_lazy_revision_falls_back_for_symbolic_head
printf 'PASS: test_lazy_revision_falls_back_for_symbolic_head\n'
test_lsp_loads_only_for_supported_filetypes
printf 'PASS: test_lsp_loads_only_for_supported_filetypes\n'
test_last_cursor_restore_targets_correct_window_and_skips_invalid_cases
printf 'PASS: test_last_cursor_restore_targets_correct_window_and_skips_invalid_cases\n'
test_mason_lsp_servers_are_versioned
printf 'PASS: test_mason_lsp_servers_are_versioned\n'

trap - EXIT
teardown_test_home
