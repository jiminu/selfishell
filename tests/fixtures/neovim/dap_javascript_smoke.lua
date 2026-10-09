local project = vim.env.SELFISHELL_DAP_PROJECT
vim.cmd.cd(project)
local function press(key)
  vim.api.nvim_feedkeys(vim.keycode(key), "xt", false)
end
vim.cmd.edit(project .. "/main.cjs")
press("<F9>")
local dap = require("dap")
dap.set_log_level("TRACE")
assert(dap.adapters["pwa-node"] and dap.adapters.node, "JS adapter was not registered after restart")
local old_select, old_input = vim.ui.select, vim.fn.input
local selected
vim.ui.select = function(items, _, callback)
  for index, item in ipairs(items) do
    if item.name == selected then callback(item, index); return end
  end
  error("Missing configuration: " .. selected)
end
vim.fn.input = function(prompt)
  if prompt == "Node inspector port: " then return vim.env.SELFISHELL_NODE_INSPECTOR_PORT end
  return project .. "/dist/mapped.cjs"
end

local function stopped_at(file, line)
  local session = dap.session()
  local frame = session and session.current_frame
  return frame and frame.source.path == project .. "/" .. file and frame.line == line
end
local function wait_stop(file, line)
  local stopped = vim.wait(15000, function() return stopped_at(file, line) end, 20)
  if not stopped then
    local log = vim.fn.readfile(vim.fn.stdpath("log") .. "/dap.log")
    print(table.concat(vim.list_slice(log, math.max(1, #log - 350)), "\n"))
  end
  assert(stopped,
    "debugger did not stop at " .. file .. ":" .. line .. " (" .. selected .. ")")
end
local function evaluate()
  local session = assert(dap.session())
  local result, failure
  session:request("evaluate", { expression = "value", frameId = session.current_frame.id, context = "watch" }, function(err, body)
    failure, result = err, body
  end)
  assert(vim.wait(10000, function() return result ~= nil or failure ~= nil end, 20), "variable query timed out")
  assert(not failure and result.result == "42", "wrong JS/TS variable: " .. vim.inspect({ failure, result }))
end
local function closed()
  return not dap.session() and next(dap.sessions()) == nil
end

for _, case in ipairs({
  { file = "main.cjs", line = 3, config = "Node: Launch current file" },
  { file = "main.ts", line = 4, config = "Node: Launch current file" },
  { file = "mapped.ts", line = 4, config = "Node: Launch JavaScript file" },
  { file = "mapped.ts", line = 4, config = "TS project launch" },
  { file = "attach.cjs", line = 3, config = "Node: Attach (port)" },
}) do
  selected = case.config
  dap.clear_breakpoints()
  vim.cmd.edit(project .. "/" .. case.file)
  vim.api.nvim_win_set_cursor(0, { case.line, 0 })
  press("<F9>")
  press("<F5>")
  if selected == "Node: Attach (port)" then
    -- --inspect-brk pauses at entry before the requested breakpoint.
    assert(vim.wait(15000, function()
      local s = dap.session()
      return s and s.current_frame ~= nil
    end, 20), "attach did not pause")
    if not stopped_at(case.file, case.line) then press("<F5>") end
  end
  wait_stop(case.file, case.line)
  evaluate()
  press("<F10>")
  wait_stop(case.file, case.line + 1)
  if selected == "Node: Launch JavaScript file" then
    press("<F5>")
    assert(vim.wait(15000, closed, 20), "JS session hierarchy remained after normal exit")
  else
    press("<F17>")
    assert(vim.wait(15000, closed, 20), "JS session hierarchy remained after Shift+F5")
  end
  assert(vim.wait(1000, function()
    for _, win in ipairs(vim.api.nvim_list_wins()) do
      local ft = vim.bo[vim.api.nvim_win_get_buf(win)].filetype
      if ft:match("^dapui_") or ft == "dap-repl" then return false end
    end
    return true
  end), "JS/TS debug panels remained after shutdown")
  print("JS/TS consumer: " .. case.file .. " / " .. selected .. " OK")
end
vim.ui.select, vim.fn.input = old_select, old_input
print("DAP consumer: OK")
