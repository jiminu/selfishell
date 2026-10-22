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

local listchars = vim.opt.listchars:get()
assert(listchars.tab == "  " and listchars.nbsp == "␣", "tabs or non-breaking spaces are not listed: " .. vim.inspect(listchars))
assert(vim.opt.fileencodings:get()[1] == "ucs-bom", "byte order marks are not detected first")

assert(vim.fn.maparg("<Space>w", "n") == "", "window prefix must not enter resize mode by itself")
local function resize(keys)
  vim.api.nvim_feedkeys(vim.keycode("<Space>wr" .. keys .. "<Esc>"), "xt", false)
end
vim.o.columns, vim.o.lines = 160, 50
vim.cmd.vsplit()
vim.cmd.split()
vim.cmd.wincmd("h")
vim.cmd.split()
for _, target in ipairs(vim.api.nvim_tabpage_list_wins(0)) do
  vim.api.nvim_set_current_win(target)
  local row, col = unpack(vim.api.nvim_win_get_position(target))
  local width, height = vim.api.nvim_win_get_width(target), vim.api.nvim_win_get_height(target)
  resize("hh")
  assert(vim.api.nvim_win_get_width(target) == width + (col == 0 and -10 or 10), "h did not move the border left")
  resize("ll")
  assert(vim.api.nvim_win_get_width(target) == width, "l did not move the border right")
  resize("k")
  assert(vim.api.nvim_win_get_height(target) == height + (row == 0 and -2 or 2), "k did not move the border up")
  resize("j")
  assert(vim.api.nvim_win_get_height(target) == height, "j did not move the border down")
  assert(vim.api.nvim_get_current_win() == target, "resizing moved focus")
end
local other_column = vim.fn.win_getid(vim.fn.winnr("h"))
local target = vim.api.nvim_get_current_win()
resize("=")
assert(math.abs(vim.api.nvim_win_get_width(target) - vim.api.nvim_win_get_width(other_column)) <= 1,
  "resize equalize did not balance columns")
vim.cmd.only()
local width, height, cmdheight = vim.api.nvim_win_get_width(0), vim.api.nvim_win_get_height(0), vim.o.cmdheight
resize("hjkl")
assert(vim.api.nvim_win_get_width(0) == width and vim.api.nvim_win_get_height(0) == height,
  "resizing a single window changed its dimensions")
assert(vim.o.cmdheight == cmdheight, "resizing changed the command-line height")

print("editor workflows: OK")
