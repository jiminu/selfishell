require("lazy").load({ plugins = { "which-key.nvim" } })
-- The consumer invokes this fixture before the normal startup event.
vim.api.nvim_exec_autocmds("VimEnter", {})
assert(vim.wait(1000, function() return require("which-key.config").loaded end), "which-key did not initialize")
vim.o.columns, vim.o.lines = 160, 50
vim.cmd.enew()
vim.bo.buftype = "nofile"
vim.api.nvim_buf_set_lines(0, 0, -1, false, { "abcdefghij" })
vim.cmd.vsplit()
local left = vim.fn.win_getid(vim.fn.winnr("h"))
vim.cmd.split()
local target = vim.api.nvim_get_current_win()
local width, height = vim.api.nvim_win_get_width(target), vim.api.nvim_win_get_height(target)
local function press(keys)
  vim.api.nvim_feedkeys(vim.keycode(keys), "xt", false)
end

-- Buffered input must stay in resize mode until Escape, including fast repeats.
press("<Space>wllhjk<Esc>")
assert(vim.api.nvim_win_get_width(target) == width + 5, "buffered resize keys were lost")
assert(vim.api.nvim_win_get_height(target) == height, "buffered height resize keys were lost")
assert(vim.api.nvim_win_get_cursor(target)[2] == 0, "resize keys leaked into normal movement")
press("l")
assert(vim.api.nvim_win_get_cursor(target)[2] == 1, "Escape did not restore normal movement")

-- An unrelated key exits and is handled normally, without replaying Space w.
press("<Space>w:let g:resize_command_completed = 1<CR>")
assert(not vim.fn.execute("messages"):find("Recursion detected", 1, true), "resize mode recursed on colon")
assert(vim.g.resize_command_completed == 1, "resize mode swallowed command-line input")
press("h")
assert(vim.api.nvim_win_get_cursor(target)[2] == 0, "command-line exit reopened resize mode")
press("<Space>w<C-h>")
assert(vim.api.nvim_get_current_win() == left, "resize mode swallowed window navigation")
press("l")
assert(vim.api.nvim_win_get_cursor(left)[2] == 1, "window navigation reopened resize mode")

vim.api.nvim_set_current_win(target)
press("<Space>w=<Esc>")
assert(math.abs(vim.api.nvim_win_get_width(target) - vim.api.nvim_win_get_width(left)) <= 1,
  "resize mode did not equalize columns")
assert(not vim.fn.execute("messages"):find("Recursion detected", 1, true), "resize mode recursed")
vim.bo.modified = false
vim.cmd.only()
print("Window resize input smoke: OK")
