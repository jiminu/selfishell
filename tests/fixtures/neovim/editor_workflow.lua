vim.g.mapleader = " "
require("config.options")
require("config.keymaps")

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

local rainbow = assert(
  plugin_spec("plugins.editor", "HiPhish/rainbow-delimiters.nvim"),
  "rainbow-delimiters spec is missing"
)
assert(
  vim.deep_equal(rainbow.event, { "BufReadPre", "BufNewFile" }),
  "rainbow-delimiters loads after the initial FileType event"
)

rainbow.init()
local rainbow_enabled = vim.g.rainbow_delimiters.condition
local function scratch(lines)
  local buf = vim.api.nvim_create_buf(false, true)
  vim.api.nvim_buf_set_lines(buf, 0, -1, false, lines)
  return buf
end
local long_file = {}
for index = 1, 5001 do
  long_file[index] = "x = f(a[" .. index .. "])"
end
assert(rainbow_enabled(scratch({ "local x = { f(1) }" })), "rainbow-delimiters is off for an ordinary file")
assert(not rainbow_enabled(scratch(long_file)), "rainbow-delimiters stays on beyond 5,000 lines")
assert(not rainbow_enabled(scratch({ string.rep("[1,{}],", 100) })), "rainbow-delimiters stays on for minified text")
for background, colors in pairs({ dark = { 0xFFD700, 0xDA70D6, 0x179FFF }, light = { 0x0431FA, 0x319331, 0x7B3814 } }) do
  vim.o.background = background
  vim.api.nvim_exec_autocmds("ColorScheme", { pattern = "vscode" })
  for index, group in ipairs(vim.g.rainbow_delimiters.highlight) do
    assert(vim.api.nvim_get_hl(0, { name = group }).fg == colors[index], background .. " bracket color differs from VS Code: " .. group)
  end
end

local listchars = vim.opt.listchars:get()
assert(listchars.tab == "  " and listchars.nbsp == "␣", "tabs or non-breaking spaces are not listed: " .. vim.inspect(listchars))
assert(vim.opt.fileencodings:get()[1] == "ucs-bom", "byte order marks are not detected first")

local function resize(keys)
  vim.api.nvim_feedkeys(vim.keycode("<Space>w" .. keys .. "<Esc>"), "xt", false)
end
vim.o.columns, vim.o.lines = 160, 50
vim.cmd.vsplit()
local other_column = vim.fn.win_getid(vim.fn.winnr("h"))
vim.cmd.split()
local target = vim.api.nvim_get_current_win()
local width, height = vim.api.nvim_win_get_width(target), vim.api.nvim_win_get_height(target)
resize("ll")
assert(vim.api.nvim_win_get_width(target) == width + 10, "repeated resize did not grow the focused split")
resize("h")
assert(vim.api.nvim_win_get_width(target) == width + 5, "resize did not shrink the focused split")
resize("j")
assert(vim.api.nvim_win_get_height(target) == height - 2, "resize did not shrink the focused split's height")
resize("k")
assert(vim.api.nvim_win_get_height(target) == height, "resize did not grow the focused split's height")
resize("=")
assert(math.abs(vim.api.nvim_win_get_width(target) - vim.api.nvim_win_get_width(other_column)) <= 1,
  "resize equalize did not balance columns")
assert(vim.api.nvim_get_current_win() == target, "resizing moved focus")
vim.cmd.only()

print("editor workflows: OK")
