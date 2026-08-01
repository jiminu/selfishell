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

local telescope_mappings = {
  ["<leader>fd"] = "<cmd>Telescope diagnostics<CR>",
  ["<leader>fs"] = "<cmd>Telescope lsp_document_symbols<CR>",
  ["<leader>fS"] = "<cmd>Telescope lsp_dynamic_workspace_symbols<CR>",
  ["<leader>fr"] = "<cmd>Telescope resume<CR>",
}
for lhs, rhs in pairs(telescope_mappings) do
  assert(
    plugin_key("plugins.telescope", "nvim-telescope/telescope.nvim", lhs) == rhs,
    "missing Telescope mapping: " .. lhs
  )
end

local telescope = assert(
  plugin_spec("plugins.telescope", "nvim-telescope/telescope.nvim"),
  "Telescope spec is missing"
)
assert(
  telescope.opts
    and telescope.opts.pickers.find_files.disable_devicons
    and telescope.opts.pickers.live_grep.disable_devicons,
  "Telescope file and grep pickers should hide devicons"
)
assert(
  not telescope.opts.defaults or not telescope.opts.defaults.disable_devicons,
  "Telescope devicons should not be disabled globally"
)
assert(
  not telescope.opts.pickers.buffers or not telescope.opts.pickers.buffers.disable_devicons,
  "Telescope buffer icons should remain enabled"
)

local tree = assert(plugin_spec("plugins.ui", "nvim-tree/nvim-tree.lua"), "nvim-tree spec is missing")
assert(has_dependency(tree, "nvim-tree/nvim-web-devicons"), "nvim-web-devicons dependency is missing")
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

local bufferline = assert(
  plugin_spec("plugins.ui", "akinsho/bufferline.nvim"),
  "bufferline spec is missing"
)
assert(bufferline.event == "VeryLazy", "bufferline is not deferred")
assert(bufferline.opts.options.always_show_bufferline == false, "bufferline should hide for one buffer")
assert(bufferline.opts.options.offsets[1].filetype == "NvimTree", "bufferline is not aligned with NvimTree")
assert(
  plugin_key("plugins.ui", "akinsho/bufferline.nvim", "[b") == "<cmd>BufferLineCyclePrev<CR>",
  "missing previous-buffer mapping"
)
assert(
  plugin_key("plugins.ui", "akinsho/bufferline.nvim", "]b") == "<cmd>BufferLineCycleNext<CR>",
  "missing next-buffer mapping"
)

local rainbow = assert(
  plugin_spec("plugins.editor", "HiPhish/rainbow-delimiters.nvim"),
  "rainbow-delimiters spec is missing"
)
assert(
  vim.deep_equal(rainbow.event, { "BufReadPre", "BufNewFile" }),
  "rainbow-delimiters loads after the initial FileType event"
)

print("editor workflows: OK")
