vim.g.mapleader = " "
require("config.options")
require("config.keymaps")

assert(vim.o.confirm, "confirmation is not enabled")
assert(vim.o.inccommand ~= "", "substitution preview is disabled")

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
  "<leader>/",
  "<leader>gs",
  "<leader>gd",
  "<leader>gl",
  "<leader>gf",
}
for _, lhs in ipairs(snacks_picker_keys) do
  assert(
    type(plugin_key("plugins.ui", "folke/snacks.nvim", lhs)) == "function",
    "missing Snacks picker mapping: " .. lhs
  )
end

local open_lazygit = plugin_key("plugins.ui", "folke/snacks.nvim", "<leader>gg")
assert(type(open_lazygit) == "function", "missing Lazygit mapping")
local lazygit_opened = false
_G.Snacks = {
  lazygit = function()
    lazygit_opened = true
  end,
}
open_lazygit()
assert(lazygit_opened, "Git UI mapping did not open Lazygit")
_G.Snacks = nil

local snacks = assert(plugin_spec("plugins.ui", "folke/snacks.nvim"), "Snacks spec is missing")
local picker = assert(snacks.opts.picker, "Snacks picker must be configured")
assert(picker.ui_select == false, "Snacks must not take over vim.ui.select")
assert(picker.sources.files.cmd == "rg", "The files picker must not depend on a personally-installed fd")
assert(picker.sources.diagnostics.filter.cwd == false, "Diagnostics must not be limited to the cwd")

local tree = assert(plugin_spec("plugins.ui", "nvim-tree/nvim-tree.lua"), "nvim-tree spec is missing")
assert(not has_dependency(tree, "nvim-tree/nvim-web-devicons"), "nvim-web-devicons dependency should be removed")
assert(type(tree.opts.view.width) == "function", "NvimTree width is not a function")
local original_columns = vim.o.columns
local widths = {}
for _, columns in ipairs({ 60, 100, 200 }) do
  vim.o.columns = columns
  local width = tree.opts.view.width()
  assert(width > 0 and width < columns and width == math.floor(width), "NvimTree width must fit the viewport")
  widths[#widths + 1] = width
end
assert(widths[1] <= widths[2] and widths[2] <= widths[3] and widths[1] < widths[3],
  "NvimTree width must adapt as the viewport grows")
vim.o.columns = original_columns
assert(type(tree.opts.on_attach) == "function", "NvimTree does not preserve window navigation mappings")
assert(
  plugin_key("plugins.ui", "nvim-tree/nvim-tree.lua", "<leader>E") == "<cmd>NvimTreeFindFile!<CR>",
  "current-file tree mapping does not update the tree root"
)

local lualine = assert(
  plugin_spec("plugins.ui", "nvim-lualine/lualine.nvim"),
  "lualine spec is missing"
)
assert(not has_dependency(lualine, "nvim-tree/nvim-web-devicons"), "nvim-web-devicons dependency should be removed")

local bufferline = assert(
  plugin_spec("plugins.ui", "akinsho/bufferline.nvim"),
  "bufferline spec is missing"
)
assert(bufferline.event == "VeryLazy", "bufferline is not deferred")
assert(
  not has_dependency(bufferline, "nvim-tree/nvim-web-devicons"),
  "nvim-web-devicons dependency should be removed"
)
assert(
  plugin_key("plugins.ui", "akinsho/bufferline.nvim", "[b") == "<cmd>BufferLineCyclePrev<CR>",
  "missing previous-buffer mapping"
)
assert(
  plugin_key("plugins.ui", "akinsho/bufferline.nvim", "]b") == "<cmd>BufferLineCycleNext<CR>",
  "missing next-buffer mapping"
)

local cmp = assert(plugin_spec("plugins.completion", "hrsh7th/nvim-cmp"), "nvim-cmp spec is missing")
assert(cmp.event == "InsertEnter", "nvim-cmp is not deferred")
assert(not has_dependency(cmp, "L3MON4D3/LuaSnip"), "LuaSnip dependency should be removed")
assert(not has_dependency(cmp, "saadparwaiz1/cmp_luasnip"), "cmp_luasnip dependency should be removed")

local rainbow = assert(
  plugin_spec("plugins.editor", "HiPhish/rainbow-delimiters.nvim"),
  "rainbow-delimiters spec is missing"
)
assert(
  vim.deep_equal(rainbow.event, { "BufReadPre", "BufNewFile" }),
  "rainbow-delimiters loads after the initial FileType event"
)

print("editor workflows: OK")
