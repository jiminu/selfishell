local project = vim.env.SELFISHELL_DAP_PROJECT .. "/cpp"
vim.cmd.cd(project)
vim.cmd.edit(project .. "/main.cpp")
local function press(key) vim.api.nvim_feedkeys(vim.keycode(key), "xt", false) end
vim.api.nvim_win_set_cursor(0, { 5, 0 })
press("<F9>")
local dap = require("dap")
assert(dap.adapters.codelldb, "CodeLLDB did not register after restart")
local exit_code
dap.listeners.after.event_exited.consumer = function(_, body) exit_code = body.exitCode end
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
  return session and session.current_frame and session.current_frame.line == 5
end, 20), "C++ did not stop at the source breakpoint")
vim.ui.select, vim.fn.input = select, input
local session = assert(dap.session())
local result, failure
session:request("evaluate", { expression = "value", context = "watch", frameId = session.current_frame.id }, function(err, body)
  failure, result = err, body
end)
assert(vim.wait(10000, function() return failure ~= nil or result ~= nil end, 20), "C++ variable query timed out")
assert(not failure and result.result == "42", "C++ quoted/empty arguments or variable evaluation failed")

press("<F5>")
assert(vim.wait(10000, function()
  return exit_code ~= nil and not dap.session()
end, 20), "C++ session did not finish")
assert(exit_code == 0, "C++ argument fixture exited unsuccessfully")
print("DAP consumer: OK")
