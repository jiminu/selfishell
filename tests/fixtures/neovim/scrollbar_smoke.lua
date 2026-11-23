local renderer = require("scrollbar.renderer")
local scheduler = require("scrollbar.scheduler")
local win = vim.api.nvim_get_current_win()

-- Headless commands run before Neovim sends its next WinScrolled event.
local function scroll(keys)
  vim.cmd("normal! " .. keys)
  vim.cmd.redraw()
  vim.api.nvim_exec_autocmds("WinScrolled", { pattern = tostring(win) })
  scheduler.flush()
  return assert(renderer.get_state(win), "missing scrollbar in long file")
end

local first = scroll("ggzt").handle.first_row
local bar = scroll("150Gzz")
assert(bar.handle.first_row > first, "scrollbar did not follow the viewport")
assert(#bar.highlights[1] == 0, "scrollbar painted a background outside the thumb")

-- Lualine sets 'statusline' in every split while the editing cursor moves.
vim.cmd("topleft vnew")
local sidebar = vim.api.nvim_get_current_win()
vim.bo.buftype = "nofile"
vim.api.nvim_set_current_win(win)
local thumb = scroll("ggzt").float_win
vim.api.nvim_win_call(sidebar, function()
  vim.api.nvim_exec_autocmds("OptionSet", { pattern = "statusline" })
end)
assert(vim.api.nvim_win_is_valid(thumb), "inactive statusline refresh closed the editing scrollbar")
print("Scrollbar smoke: OK")
