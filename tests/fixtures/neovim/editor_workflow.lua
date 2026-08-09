vim.g.mapleader = " "
require("config.options")
require("config.keymaps")

assert(vim.o.splitright and vim.o.splitbelow, "split direction is not configured")
assert(vim.o.scrolloff == 4, "scrolloff is not configured")
assert(not vim.o.wrap, "line wrapping is enabled")
assert(vim.o.confirm, "confirmation is not enabled")
assert(vim.o.inccommand == "split", "substitution preview is not configured")

local function assert_map(mode, lhs, rhs)
  local mapping = vim.fn.maparg(lhs, mode, false, true)
  assert(mapping.rhs == rhs, "unexpected mapping for " .. lhs .. ": " .. vim.inspect(mapping))
end

local window_mappings = {
  ["<C-h>"] = "<C-W>h",
  ["<C-j>"] = "<C-W>j",
  ["<C-k>"] = "<C-W>k",
  ["<C-l>"] = "<C-W>l",
}
for lhs, rhs in pairs(window_mappings) do
  assert_map("n", lhs, rhs)
end

local delete_map = vim.fn.maparg("<leader>bd", "n", false, true)
assert(type(delete_map.callback) == "function", "buffer delete mapping is not callback-based")
assert_map("x", "<", "<gv")
assert_map("x", ">", ">gv")

local function plugin_spec(module, repository)
  for _, spec in ipairs(require(module)) do
    if spec[1] == repository then
      return spec
    end
  end
end

local function plugin_key(module, repository, lhs)
  local spec = plugin_spec(module, repository)
  for _, key in ipairs(spec and spec.keys or {}) do
    if key[1] == lhs then
      return key[2]
    end
  end
end

local function has_dependency(spec, repository)
  for _, dependency in ipairs(spec.dependencies or {}) do
    if dependency[1] == repository then
      return true
    end
  end
  return false
end

local snacks_picker_keys = {
  "<leader>ff",
  "<leader>fF",
  "<leader>fg",
  "<leader>fG",
  "<leader>fb",
  "<leader>fh",
  "<leader>fd",
  "<leader>fs",
  "<leader>fS",
  "<leader>fr",
}
for _, lhs in ipairs(snacks_picker_keys) do
  assert(
    type(plugin_key("plugins.ui", "folke/snacks.nvim", lhs)) == "function",
    "missing Snacks picker mapping: " .. lhs
  )
end

local snacks = assert(plugin_spec("plugins.ui", "folke/snacks.nvim"), "Snacks spec is missing")
local picker = assert(snacks.opts.picker, "Snacks picker must be configured")
assert(picker.ui_select == false, "Snacks must not take over vim.ui.select")
assert(picker.sources.files.cmd == "rg", "The files picker must not depend on a personally-installed fd")
assert(picker.sources.files.icons.files.enabled == false, "The files picker should hide the leading file icon")
assert(picker.sources.grep.icons.files.enabled == false, "The grep picker should hide the leading file icon")
assert(picker.sources.buffers.icons.files.enabled == false, "The buffers picker should hide the leading file icon")
assert(picker.sources.diagnostics.filter.cwd == false, "Diagnostics must not be limited to the cwd")

local tree = assert(plugin_spec("plugins.ui", "nvim-tree/nvim-tree.lua"), "nvim-tree spec is missing")
assert(not has_dependency(tree, "nvim-tree/nvim-web-devicons"), "nvim-web-devicons dependency should be removed")
assert(type(tree.opts.view.width) == "function", "NvimTree width is not a function")
local original_columns = vim.o.columns
vim.o.columns = 60
assert(tree.opts.view.width() == 20, "width should clamp to the 20-column minimum")
vim.o.columns = 100
assert(tree.opts.view.width() == 25, "width should scale to 25% of columns")
vim.o.columns = 200
assert(tree.opts.view.width() == 30, "width should clamp to the 30-column maximum")
vim.o.columns = original_columns
assert(type(tree.opts.on_attach) == "function", "NvimTree does not preserve window navigation mappings")
assert(tree.opts.renderer.group_empty, "NvimTree should compact single-child directory chains")
assert(tree.opts.renderer.indent_markers.enable, "NvimTree indent markers should be enabled")
assert(tree.opts.renderer.indent_markers.inline_arrows, "NvimTree arrows should align with indent markers")
assert(tree.opts.renderer.icons.glyphs.folder.arrow_closed == ">", "NvimTree closed folder arrow must be portable")
assert(tree.opts.renderer.icons.glyphs.folder.arrow_open == "v", "NvimTree open folder arrow must be portable")
assert(not tree.opts.renderer.icons.padding, "NvimTree folder arrows should use the default padding")
assert(tree.opts.renderer.icons.show.file == false, "NvimTree file icons should remain hidden")
assert(tree.opts.renderer.icons.show.folder == false, "NvimTree folder icons should remain hidden")
assert(
  plugin_key("plugins.ui", "nvim-tree/nvim-tree.lua", "<leader>E") == "<cmd>NvimTreeFindFile<CR>",
  "missing current-file tree mapping"
)

local lualine = assert(
  plugin_spec("plugins.ui", "nvim-lualine/lualine.nvim"),
  "lualine spec is missing"
)
local branch = lualine.opts.sections.lualine_c[1]
assert(branch.icon == "", "lualine branch icon should be hidden")
assert(branch.color.fg == "#5fd700" and branch.color.gui == "bold", "lualine branch color is incorrect")
assert(branch.padding.left == 0 and branch.padding.right == 1, "lualine branch spacing is incorrect")
local filetype = lualine.opts.sections.lualine_x[1]
assert(
  filetype[1] == "filetype" and filetype.icons_enabled == false,
  "lualine filetype icon should be hidden"
)

assert_map("n", "[b", "<cmd>bprevious<CR>")
assert_map("n", "]b", "<cmd>bnext<CR>")

assert(not pcall(require, "plugins.completion"), "plugins.completion should no longer exist")

local mason_lspconfig = assert(
  plugin_spec("plugins.lsp", "mason-org/mason-lspconfig.nvim"),
  "mason-lspconfig spec is missing"
)
assert(
  not has_dependency(mason_lspconfig, "hrsh7th/cmp-nvim-lsp"),
  "cmp-nvim-lsp dependency should be removed"
)

assert(
  vim.tbl_contains(vim.opt.completeopt:get(), "popup"),
  "completeopt should show per-item documentation"
)

local completion_autocmds = vim.api.nvim_get_autocmds({ event = "InsertCharPre", group = "UserLspKeymaps" })
assert(#completion_autocmds > 0, "generic identifier completion trigger is missing")

local cspace_map = vim.fn.maparg("<C-Space>", "i", false, true)
assert(type(cspace_map.callback) == "function", "manual completion mapping is missing")

local tab_map = vim.fn.maparg("<Tab>", "i", false, true)
assert(type(tab_map.callback) == "function" and tab_map.expr == 1, "Tab completion/snippet mapping is missing")

local stab_map = vim.fn.maparg("<S-Tab>", "i", false, true)
assert(
  type(stab_map.callback) == "function" and stab_map.expr == 1,
  "Shift-Tab completion/snippet mapping is missing"
)

local cr_map = vim.fn.maparg("<CR>", "i", false, true)
assert(type(cr_map.callback) == "function" and cr_map.expr == 1, "Enter completion-confirm mapping is missing")

local scrollbar = assert(
  plugin_spec("plugins.ui", "petertriho/nvim-scrollbar"),
  "nvim-scrollbar spec is missing"
)
assert(scrollbar.opts.handlers.cursor == false, "scrollbar cursor tracking should stay disabled")
assert(scrollbar.opts.handlers.diagnostic == true, "scrollbar diagnostics should remain enabled")
assert(scrollbar.opts.handlers.handle == true, "scrollbar viewport handle should remain enabled")
assert(scrollbar.opts.marks == nil, "scrollbar should not carry dead cursor-mark configuration")

local rainbow = assert(
  plugin_spec("plugins.editor", "HiPhish/rainbow-delimiters.nvim"),
  "rainbow-delimiters spec is missing"
)
assert(
  vim.deep_equal(rainbow.event, { "BufReadPre", "BufNewFile" }),
  "rainbow-delimiters loads after the initial FileType event"
)

print("editor workflows: OK")
