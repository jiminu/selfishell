vim.g.mapleader = " "
vim.bo.buflisted = false

local buffers = require("config.keymaps")
local editor_win = vim.api.nvim_get_current_win()
local tree_buf = vim.api.nvim_create_buf(false, true)
vim.bo[tree_buf].filetype = "NvimTree"
local tree_win = vim.api.nvim_open_win(tree_buf, false, { split = "left", win = editor_win })
local first = vim.api.nvim_create_buf(true, false)
local second = vim.api.nvim_create_buf(true, false)

vim.api.nvim_win_set_buf(editor_win, first)
buffers.delete_buffer(first)
assert(vim.api.nvim_win_is_valid(editor_win), "editor window was closed")
assert(
  vim.api.nvim_win_get_buf(editor_win) == second,
  "next listed buffer did not replace the deleted buffer"
)
assert(
  vim.api.nvim_win_is_valid(tree_win) and vim.api.nvim_win_get_buf(tree_win) == tree_buf,
  "NvimTree window was changed"
)

buffers.delete_buffer(second)
assert(vim.api.nvim_win_is_valid(editor_win), "editor window was closed with the last file buffer")
assert(vim.api.nvim_win_get_buf(editor_win) ~= tree_buf, "NvimTree replaced the editor buffer")
print("buffer delete layout: OK")
