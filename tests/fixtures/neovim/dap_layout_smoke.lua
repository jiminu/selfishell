require("lazy").load({ plugins = { "nvim-tree.lua", "nvim-dap-ui" } })
vim.o.columns, vim.o.lines = 180, 55

local tree = require("nvim-tree.api").tree
local dap, ui = require("dap"), require("dapui")
local editor = vim.api.nvim_get_current_win()
tree.open()
local explorer = tree.winid()
vim.api.nvim_set_current_win(editor)

local function assert_width(expected, context)
  assert(vim.api.nvim_win_get_width(explorer) == expected,
    context .. ": explorer width " .. vim.api.nvim_win_get_width(explorer) .. ", expected " .. expected)
end

local function close_session(session)
  session.on_close.selfishell()
  assert(vim.wait(1000, function()
    for _, win in ipairs(vim.api.nvim_list_wins()) do
      local ft = vim.bo[vim.api.nvim_win_get_buf(win)].filetype
      if ft:match("^dapui_") or ft == "dap-repl" then return false end
    end
    return true
  end), "closed session left debug panels open")
end

assert_width(30, "initial")
for cycle = 1, 3 do
  local session = { on_close = {} }
  dap.listeners.after.event_initialized.selfishell(session)
  assert_width(30, "session opened " .. cycle)
  close_session(session)
  assert_width(30, "session closed " .. cycle)
end

-- Preserve a manual width, including changes made while debugging.
vim.api.nvim_win_set_width(explorer, 37)
local session = { on_close = {} }
dap.listeners.after.event_initialized.selfishell(session)
assert_width(37, "custom width at session start")
vim.api.nvim_win_set_width(explorer, 43)
close_session(session)
assert_width(43, "custom width at session close")

-- Closing the UI manually before disconnect must not open it again.
session = { on_close = {} }
dap.listeners.after.event_initialized.selfishell(session)
ui.toggle()
close_session(session)
assert_width(43, "manual toggle before session close")
print("DAP layout smoke: OK")
