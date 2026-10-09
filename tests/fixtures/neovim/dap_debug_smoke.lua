local project = vim.env.SELFISHELL_DAP_PROJECT
vim.cmd.cd(project)
local function press(key)
  vim.api.nvim_feedkeys(vim.keycode(key), "xt", false)
end
vim.cmd.edit(project .. "/main.py")
vim.api.nvim_win_set_cursor(0, { 3, 0 })
press("<F9>")
local dap = require("dap")
assert(dap.adapters.python and dap.adapters.delve, "installed adapters were not registered after restart")
local python_launch = dap.configurations.python[1]
dap.configurations.python = {}
dap.providers.configs["consumer"] = function(bufnr)
  return vim.bo[bufnr].filetype == "python" and { python_launch } or {}
end
local stopped, exited = 0, false
dap.listeners.after.event_stopped.consumer = function() stopped = stopped + 1 end
dap.listeners.after.event_terminated.consumer = function() exited = true end
dap.listeners.after.event_exited.consumer = function() exited = true end

local function wait_stop()
  assert(vim.wait(60000, function()
    local s = dap.session()
    return stopped > 0 and s and s.current_frame ~= nil
  end, 50), "debugger did not stop at a breakpoint")
end
local function evaluate()
  local session = assert(dap.session())
  local result, failure
  session:request("evaluate", { expression = "value", frameId = session.current_frame.id, context = "watch" }, function(err, body)
    failure, result = err, body
  end)
  assert(vim.wait(10000, function() return result ~= nil or failure ~= nil end, 20), "variable query timed out")
  assert(not failure and result.result == "42", "wrong variable value: " .. vim.inspect({ failure, result }))
end

press("<F5>")
wait_stop()
evaluate()
local windows = vim.tbl_filter(function(win)
  return vim.bo[vim.api.nvim_win_get_buf(win)].filetype == "dapui_scopes"
end, vim.api.nvim_list_wins())
assert(#windows == 1, "debug variable panel did not open")
-- Resume from the UI, where the current filetype has no launch configuration.
vim.api.nvim_set_current_win(windows[1])
press("<F5>")
assert(vim.wait(15000, function() return exited and not dap.session() end, 50), "Python session did not finish")

-- Traditional terminal input decodes Shift+F5 as F17.
stopped, exited = 0, false
vim.cmd.edit(project .. "/main.py")
press("<F5>")
wait_stop()
local source = vim.api.nvim_get_current_buf()
assert(vim.bo[source].filetype == "python", "Python source window was not focused at the breakpoint")
vim.cmd.quit()
press("<F17>")
assert(vim.wait(15000, function() return not dap.session() end, 50), "Python session did not terminate with F17")
assert(vim.wait(1000, function()
  for _, win in ipairs(vim.api.nvim_list_wins()) do
    local ft = vim.bo[vim.api.nvim_win_get_buf(win)].filetype
    if ft:match("^dapui_") or ft == "dap-repl" then return false end
  end
  return #vim.fn.win_findbuf(source) > 0
end), "closing the Python source before F17 left debug panels or no editor")

stopped, exited = 0, false
vim.cmd.edit(project .. "/main.go")
vim.api.nvim_win_set_cursor(0, { 6, 0 })
press("<F9>")
-- Select the existing default without an interactive picker in headless mode.
local old_select = vim.ui.select
vim.ui.select = function(items, _, callback) callback(items[1], 1) end
press("<F5>")
vim.ui.select = old_select
wait_stop()
evaluate()
local before = stopped
press("<F10>")
assert(vim.wait(15000, function() return stopped > before or exited end, 50), "Go step did not advance")
press("<S-F5>")
assert(vim.wait(15000, function() return not dap.session() end, 50), "Go session did not terminate")
assert(vim.wait(1000, function()
  for _, win in ipairs(vim.api.nvim_list_wins()) do
    if vim.bo[vim.api.nvim_win_get_buf(win)].filetype:match("^dapui_") then return false end
  end
  return true
end), "debug session closed but panels remained open")
print("DAP consumer: OK")
