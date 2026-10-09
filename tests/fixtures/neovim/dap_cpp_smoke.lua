local project = vim.env.SELFISHELL_DAP_PROJECT .. "/cpp"
vim.cmd.cd(project)
vim.cmd.edit(project .. "/main.cpp")
local function press(key) vim.api.nvim_feedkeys(vim.keycode(key), "xt", false) end
vim.api.nvim_win_set_cursor(0, { 8, 0 })
press("<F9>")
local dap = require("dap")
assert(dap.adapters.codelldb, "CodeLLDB did not register after restart")
local stopped, continued = 0, 0
dap.listeners.after.event_stopped.consumer = function() stopped = stopped + 1 end
dap.listeners.after.event_continued.consumer = function() continued = continued + 1 end
local select, input = vim.ui.select, vim.fn.input
vim.ui.select = function(items, _, callback)
  for i, item in ipairs(items) do
    if item.name == "LLDB: Launch (args)" then callback(item, i); return end
  end
  error("CodeLLDB argument launch was not offered")
end
vim.fn.input = function(prompt)
  if prompt:find("Path to executable", 1, true) then return project .. "/cpp-probe" end
  return [["hello world" ""]]
end
press("<F5>")
assert(vim.wait(30000, function()
  local session = dap.session()
  return session and session.current_frame and session.current_frame.line == 8
end, 20), "C++ did not stop at the source breakpoint")
vim.ui.select, vim.fn.input = select, input
local session = assert(dap.session())
local result, failure
session:request("evaluate", { expression = "value", context = "watch", frameId = session.current_frame.id }, function(err, body)
  failure, result = err, body
end)
assert(vim.wait(10000, function() return failure ~= nil or result ~= nil end, 20), "C++ variable query timed out")
assert(not failure and result.result == "42", "C++ quoted/empty arguments or variable evaluation failed")

-- Stacks uses the same panel navigation as Breakpoints, with a live session.
local source = vim.api.nvim_get_current_buf()
local editor = vim.api.nvim_get_current_win()
local stack_win, stack_line
assert(vim.wait(1000, function()
  for _, win in ipairs(vim.api.nvim_list_wins()) do
    local buf = vim.api.nvim_win_get_buf(win)
    if vim.bo[buf].filetype == "dapui_stacks" then
      for i, line in ipairs(vim.api.nvim_buf_get_lines(buf, 0, -1, false)) do
        if line:find("main.cpp:8", 1, true) then stack_win, stack_line = win, i; return true end
      end
    end
  end
end), "C++ stack frame did not render")
vim.api.nvim_win_close(editor, true)
vim.api.nvim_set_current_win(stack_win)
vim.api.nvim_win_set_cursor(stack_win, { stack_line, 0 })
vim.fn.maparg("o", "n", false, true).callback()
assert(vim.wait(1000, function() return #vim.fn.win_findbuf(source) > 0 end),
  "Stacks could not reopen the closed C++ source")
editor = vim.fn.win_findbuf(source)[1]
vim.api.nvim_win_set_cursor(editor, { 1, 0 })
for _, ft in ipairs({ "dapui_breakpoints", "dapui_scopes" }) do
  for _, win in ipairs(vim.api.nvim_list_wins()) do
    if vim.bo[vim.api.nvim_win_get_buf(win)].filetype == ft then vim.api.nvim_set_current_win(win) end
  end
end
press(" Df")
assert(vim.wait(1000, function()
  return vim.api.nvim_get_current_win() == editor and vim.api.nvim_win_get_cursor(editor)[1] == 8
end), "current-frame shortcut did not return to the C++ source")
press("<F10>")
assert(vim.wait(10000, function()
  local frame = session.current_frame
  return frame and frame.line ~= 8
end, 20), "C++ step did not advance")
local before = continued
press("<F5>")
assert(vim.wait(10000, function() return continued > before and not session.stopped_thread_id end, 20),
  "C++ loop did not resume")
before = stopped
press(" Dp")
assert(vim.wait(10000, function()
  return stopped > before and session.stopped_thread_id and session.current_frame
end, 20), "pause shortcut did not stop the running C++ loop")
press("<F17>")
assert(vim.wait(10000, function() return not dap.session() end, 20), "C++ termination failed")
assert(vim.wait(1000, function()
  for _, win in ipairs(vim.api.nvim_list_wins()) do
    if vim.bo[vim.api.nvim_win_get_buf(win)].filetype:match("^dapui_") then return false end
  end
  return true
end), "CodeLLDB session closed but panels remained")
print("DAP consumer: OK")
