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

local function panel_windows()
  return vim.tbl_filter(function(win)
    local ft = vim.bo[vim.api.nvim_win_get_buf(win)].filetype
    return ft:match("^dapui_") or ft == "dap-repl"
  end, vim.api.nvim_list_wins())
end

-- Starting after :q on one panel must rebuild the incomplete layout.
require("config.dap").toggle_ui()
for _, win in ipairs(panel_windows()) do
  if vim.bo[vim.api.nvim_win_get_buf(win)].filetype == "dapui_scopes" then vim.api.nvim_win_close(win, true) end
end
vim.api.nvim_set_current_win(editor)
local repaired = { on_close = {} }
local opened, message = pcall(dap.listeners.after.event_initialized.selfishell, repaired)
assert(opened, "partly closed panels broke initialization: " .. tostring(message))
assert(repaired.on_close.selfishell, "initialization did not register cleanup")
assert(#panel_windows() == 6, "initialization did not restore all panels")
close_session(repaired)

-- Hiding a partly closed UI must not reopen its missing layout.
require("config.dap").toggle_ui()
for _, win in ipairs(panel_windows()) do
  local ft = vim.bo[vim.api.nvim_win_get_buf(win)].filetype
  if ft == "dap-repl" or ft == "dapui_console" then vim.api.nvim_win_close(win, true) end
end
require("config.dap").toggle_ui()
assert(#panel_windows() == 0, "UI toggle exchanged visible layouts instead of hiding them")

-- A standalone REPL is independent of the UI toggle.
dap.repl.open()
local standalone = panel_windows()[1]
require("config.dap").toggle_ui()
assert(#panel_windows() > 1, "standalone REPL prevented the UI from opening")
require("config.dap").toggle_ui()
assert(#panel_windows() == 1 and vim.api.nvim_win_is_valid(standalone), "UI toggle closed the standalone REPL")
vim.api.nvim_win_close(standalone, true)
vim.api.nvim_set_current_win(editor)

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

-- :q closes the source window, but its buffer remains available for recovery.
local source = vim.api.nvim_win_get_buf(editor)
for _, with_tree in ipairs({ true, false }) do
  if not with_tree then tree.close() end
  vim.api.nvim_set_current_win(with_tree and explorer or editor)
  session = { on_close = {} }
  dap.listeners.after.event_initialized.selfishell(session)
  vim.api.nvim_set_current_win(editor)
  vim.cmd.quit()
  close_session(session)
  editor = vim.fn.win_findbuf(source)[1]
  assert(editor, "closing the last source window left no editor after debug termination")
  assert(vim.wo[editor].number and not vim.wo[editor].winfixwidth,
    "recovered source window inherited debug panel options")
  if with_tree then assert(tree.is_visible(), "recovering an editor removed the file explorer") end
end

-- The UI shortcut must recover an editor too, even before the session ends.
vim.api.nvim_set_current_win(editor)
session = { on_close = {} }
dap.listeners.after.event_initialized.selfishell(session)
vim.cmd.quit()
require("config.dap").toggle_ui()
close_session(session)
editor = vim.fn.win_findbuf(source)[1]
assert(editor, "hiding the UI with no source window left no editor")

-- A background session's cleanup must not change the user's current tab/window.
vim.api.nvim_set_current_win(editor)
session = { on_close = {} }
dap.listeners.after.event_initialized.selfishell(session)
vim.cmd.quit()
vim.cmd.tabnew()
local other_tab, other_win = vim.api.nvim_get_current_tabpage(), vim.api.nvim_get_current_win()
local second_session = { on_close = {} }
dap.listeners.after.event_initialized.selfishell(second_session)
assert(vim.api.nvim_get_current_win() == other_win, "initializing another session stole tab focus")
for _, win in ipairs(panel_windows()) do
  assert(vim.api.nvim_win_get_tabpage(win) ~= other_tab, "initializing another session moved the debug UI")
end
require("config.dap").toggle_ui()
close_session(second_session)
assert(vim.api.nvim_get_current_tabpage() == other_tab and vim.api.nvim_get_current_win() == other_win,
  "debug cleanup stole focus from another tab")
editor = vim.fn.win_findbuf(source)[1]
assert(editor, "background debug cleanup did not restore its editor")
vim.cmd.tabclose()

-- A deliberately deleted source buffer stays deleted; leave a normal empty editor.
vim.api.nvim_set_current_win(editor)
session = { on_close = {} }
dap.listeners.after.event_initialized.selfishell(session)
vim.cmd.quit()
vim.api.nvim_buf_delete(source, { force = true })
close_session(session)
assert(vim.bo.buftype == "" and vim.bo.filetype == "", "missing source did not leave an empty editor")
assert(vim.wo.number and vim.wo.relativenumber and vim.wo.winhighlight == "",
  "empty editor inherited debug panel options")
print("DAP layout smoke: OK")
