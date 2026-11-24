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

assert(vim.fn.exists(":MinimapToggle") == 2, "minimap is not enabled")
local minimap = require("scrollbar.minimap.renderer")
local minimap_scheduler = require("scrollbar.minimap.scheduler")
minimap_scheduler.flush()
assert(minimap.get_state(win), "minimap did not open with the file")

local function toggle_minimap()
  vim.api.nvim_feedkeys(vim.keycode("<Space>uM"), "xt", false)
  minimap_scheduler.flush()
end
toggle_minimap()
assert(not minimap.get_state(win), "minimap shortcut did not hide the map")
toggle_minimap()
assert(minimap.get_state(win), "minimap shortcut did not show the map again")

-- Cursor yielding applies only within the minimap's float, including its edge.
local map_config = vim.api.nvim_win_get_config(minimap.get_state(win).float_win)
local bar_config = vim.api.nvim_win_get_config(renderer.get_state(win).float_win)
assert(map_config.anchor == bar_config.anchor and map_config.col == bar_config.col,
  "minimap would reappear when the cursor reaches the scrollbar")
assert(bar_config.zindex > map_config.zindex, "scrollbar must draw over the minimap")
print("Scrollbar smoke: OK")
