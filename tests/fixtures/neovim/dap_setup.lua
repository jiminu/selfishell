vim.g.mapleader = " "
require("config.keymaps")
local ok, specs = pcall(require, "plugins.dap")
assert(ok, "Selfishell must provide the DAP plugin configuration")
local by_name = {}
for _, spec in ipairs(specs) do
  by_name[spec[1]] = spec
end
local bridge = assert(by_name["jay-babu/mason-nvim-dap.nvim"])
assert(#bridge.opts.ensure_installed == 0 and bridge.opts.automatic_installation == false,
  "opening Neovim must not install debug adapters")

-- Offline boundary: real Selfishell callbacks, a debugger that records requests.
local actions = {}
local dap = { ABORT = {}, adapters = {}, configurations = {}, listeners = { on_config = {}, after = { event_initialized = {} }, before = { event_terminated = {}, event_exited = {} } } }
dap.defaults = { fallback = {} }
local session
dap.session = function() return session end
dap.sessions = function() return session and { session } or {} end
local launched
dap.run = function(config) launched = config; actions[#actions + 1] = "run" end
for _, name in ipairs({ "continue", "toggle_breakpoint", "step_over", "step_into", "step_out", "terminate" }) do
  dap[name] = function() actions[#actions + 1] = name end
end
package.preload["dap"] = function() return dap end
package.preload["dap.async"] = function() return { run = function(fn) coroutine.wrap(fn)() end } end
package.preload["dap.ui"] = function()
  return { pick_if_many = function(items, _, _, callback)
    if #items == 1 then callback(items[1]) else vim.ui.select(items, {}, callback) end
  end }
end
local installed = false
package.preload["mason-registry"] = function()
  return { get_package = function() return { is_installed = function() return installed end } end }
end
package.preload["mason-nvim-dap.mappings.source"] = function()
  return { nvim_dap_to_package = { python = "debugpy", delve = "delve", js = "js-debug-adapter", codelldb = "codelldb" } }
end
local panel_open = false
package.preload["dapui"] = function()
  return {
    open = function() panel_open = true end,
    close = function() panel_open = false end,
    toggle = function() panel_open = not panel_open end,
    eval = function() actions[#actions + 1] = "eval" end,
  }
end
package.preload["dapui.config"] = function()
  return { layouts = { { position = "left" }, { position = "bottom" } } }
end
package.preload["dapui.windows"] = function() return { layouts = {} } end
local registered = 0
package.preload["mason-nvim-dap"] = function()
  return { default_setup = function(config)
    if not config.adapters then return end
    registered = registered + 1
    dap.adapters[config.name] = config.adapters
    for _, ft in ipairs(config.filetypes) do
      dap.configurations[ft] = vim.list_extend(dap.configurations[ft] or {}, config.configurations)
    end
  end }
end
local keys = {}
for _, key in ipairs(bridge.keys) do keys[key[1]] = key[2] end
vim.bo.filetype = "python"
-- A user may supply launch configurations through native providers alone.
-- Read each provider once, including providers that yield before returning.
local provider_calls = 0
local custom_launch = { name = "Custom launch", type = "custom", request = "launch" }
dap.providers = { configs = { user = function()
  provider_calls = provider_calls + 1
  local co = coroutine.running()
  vim.schedule(function() coroutine.resume(co) end)
  coroutine.yield()
  return { custom_launch }
end } }
keys["<F5>"]()
assert(vim.wait(1000, function() return launched ~= nil end), "F5 bypassed native DAP configuration providers")
assert(launched == custom_launch and provider_calls == 1, "F5 must use the provider result once")
assert(not panel_open, "starting a session opened the UI before initialization")

local notify, messages = vim.notify, {}
vim.notify = function(message) messages[#messages + 1] = message end
local providers = dap.providers.configs
dap.providers.configs = {}
for ft, adapter in pairs({ python = "python", go = "delve", javascript = "js", typescript = "js", cpp = "codelldb" }) do
  vim.bo.filetype = ft
  messages = {}
  keys["<F5>"]()
  assert(#messages == 1 and messages[1]:find(":DapInstall " .. adapter, 1, true), "missing installation hint for " .. ft)
end
installed = true
messages = {}
keys["<F5>"]()
assert(#messages == 1 and not messages[1]:find(":DapInstall", 1, true), "installed adapter was reported as missing")
installed = false
vim.bo.filetype = "python"
dap.adapters.python = { type = "executable", command = "external-debugpy" }
messages = {}
keys["<F5>"]()
assert(#messages == 1 and not messages[1]:find(":DapInstall", 1, true), "external adapter was reported as missing")
dap.adapters.python = nil
vim.bo.filetype = "java"
messages = {}
keys["<F5>"]()
assert(#messages == 1 and not messages[1]:find(":DapInstall", 1, true), "unsupported language received incomplete setup instructions")
dap.providers.configs = {
  invalid = function() return nil end,
  valid = function() return { custom_launch } end,
}
launched = nil
keys["<F5>"]()
assert(launched == custom_launch, "invalid provider prevented a valid configuration from launching")
dap.providers.configs = providers

-- Warn on launch without changing the source or showing warnings on resume/attach.
by_name["mfussenegger/nvim-dap"].config()
local prelaunch = assert(dap.listeners.on_config.selfishell, "launch warning hook missing")
vim.api.nvim_buf_set_name(0, vim.env.HOME .. "/project/unsaved.py")
vim.api.nvim_buf_set_lines(0, 0, -1, false, { "value = 42" })
messages = {}
assert(prelaunch(custom_launch) == custom_launch)
assert(#messages == 1 and messages[1]:find(":wa", 1, true), "unsaved source warning missing")
assert(vim.bo.modified and vim.fn.filereadable(vim.api.nvim_buf_get_name(0)) == 0, "debug launch saved the user's edits")
messages = {}
prelaunch({ request = "attach" })
prelaunch({ request = "launch", __pendingTargetId = "child" })
assert(#messages == 0, "attach or child session repeated the save warning")
session = {}
keys["<F5>"]()
assert(actions[#actions] == "continue" and #messages == 0, "resume repeated startup checks")
session = nil
vim.bo.modified = false
prelaunch(custom_launch)
assert(#messages == 0, "saved source produced a warning")
vim.api.nvim_buf_set_name(0, vim.env.HOME .. "/outside-project.py")
vim.bo.modified = true
prelaunch(custom_launch)
assert(#messages == 1, "unsaved current file outside the working directory was missed")
vim.bo.modified = false
vim.notify = notify

local config = {
  name = "python", adapters = { type = "executable", command = "debugpy-adapter" },
  filetypes = { "python" }, configurations = { { name = "Python: Launch file", type = "python", request = "launch", program = "${file}" } },
}
local handler = bridge.opts.handlers[1]
handler(config)
handler(vim.deepcopy(config))
assert(registered == 1 and #dap.configurations.python == 1, "adapter reinstall duplicated configurations")
local path = dap.configurations.python[1].pythonPath
local home = vim.env.HOME
assert(path() == home .. "/project/.venv/bin/python", "project .venv not selected")
vim.env.CONDA_PREFIX = home .. "/conda"
assert(path() == home .. "/conda/bin/python", "active Conda environment not selected")
vim.env.VIRTUAL_ENV = home .. "/active"
assert(path() == home .. "/active/bin/python", "active virtualenv not selected")
vim.env.VIRTUAL_ENV, vim.env.CONDA_PREFIX = nil, nil
vim.cmd.cd(home .. "/other")
assert(path() == home .. "/other/.venv/bin/python", "Python selection was cached across projects")
vim.cmd.cd(home)
assert(path() == home .. "/bin/python3", "PATH interpreter fallback not selected")
vim.env.VIRTUAL_ENV = home .. "/missing"
assert(path() == home .. "/bin/python3", "stale virtualenv path was selected")

-- Mason knows how to install js-debug but supplies no adapter or launches.
local personal = { name = "Personal TypeScript launch", type = "node" }
dap.configurations.typescript = { personal }
handler({ name = "js" })
assert(dap.adapters["pwa-node"], "installing js must register the Node debug adapter")
assert(dap.adapters.node == dap.adapters["pwa-node"], "launch.json's node type must resolve to the JS adapter")
assert(dap.adapters.node.executable.command == home .. "/bin/js-debug-adapter", "JS adapter must use Mason's executable on PATH")
assert(dap.configurations.typescript[1] == personal, "JS setup replaced a personal launch")
local project_launch = { name = "Project launch", type = "node" }
local resolved
local function resolve(launch)
  dap.adapters.node.enrich_config(launch, function(value) resolved = value end)
end
resolve(project_launch)
assert(resolved.type == "pwa-node" and project_launch.type == "node", "JS adapter must resolve the node alias without changing the user's launch")
assert(resolved.__workspaceFolder == home, "JS source map discovery needs the current workspace")
assert(not project_launch.__workspaceFolder, "JS setup mutated the user's launch configuration")
vim.cmd.cd(home .. "/project")
resolve(project_launch)
assert(resolved.__workspaceFolder == home .. "/project", "JS workspace was cached across projects")
project_launch.__workspaceFolder = home .. "/custom"
resolve(project_launch)
assert(resolved.__workspaceFolder == home .. "/custom", "JS setup replaced an explicit workspace")
vim.cmd.cd(home)
for _, ft in ipairs({ "javascript", "typescript" }) do
  local launches = {}
  for _, launch in ipairs(dap.configurations[ft]) do launches[launch.name] = launch end
  assert(launches["Node: Launch current file"].program == "${file}", "current-file debugging is missing for " .. ft)
  local compiled = assert(launches["Node: Launch JavaScript file"])
  local attach = assert(launches["Node: Attach (port)"])
  assert(launches["Node: Launch current file"].console == "integratedTerminal" and compiled.console == "integratedTerminal",
    "JS/TS launches must allow standard input")
  assert(attach.console == nil, "attach must keep the existing process's terminal")
  local input = vim.fn.input
  vim.fn.input = function() return home .. "/project/dist/main.js" end
  assert(compiled.program() == home .. "/project/dist/main.js", "chosen JavaScript entrypoint was lost")
  vim.fn.input = function() return "" end
  assert(compiled.program() == dap.ABORT, "cancelled file selection must abort launch")
  assert(attach.port() == dap.ABORT, "cancelled port selection must abort attach")
  vim.fn.input = function() return "9230" end
  assert(attach.port() == 9230, "attach must use the chosen inspector port")
  for _, invalid in ipairs({ "0", "65536", "9229.5", "invalid" }) do
    vim.fn.input = function() return invalid end
    assert(attach.port() == dap.ABORT, "invalid inspector port was accepted: " .. invalid)
  end
  vim.fn.input = input
end
local custom_node = { type = "server", port = 8765 }
dap.adapters.node = custom_node
local js_count, ts_count = #dap.configurations.javascript, #dap.configurations.typescript
handler({ name = "js" })
assert(#dap.configurations.javascript == js_count and #dap.configurations.typescript == ts_count,
  "JS reinstall duplicated launch configurations")
assert(dap.adapters.node == custom_node, "JS reinstall replaced a personal node adapter")
dap.adapters["pwa-node"] = custom_node
handler({ name = "js" })
assert(dap.adapters["pwa-node"] == custom_node, "JS setup replaced a personal pwa-node adapter")

session = {}
for key, action in pairs({
  ["<F5>"] = "continue", ["<F9>"] = "toggle_breakpoint", ["<F10>"] = "step_over",
  ["<F11>"] = "step_into", ["<S-F11>"] = "step_out", ["<S-F5>"] = "terminate",
  ["<F23>"] = "step_out", ["<F17>"] = "terminate",
  ["<leader>Dc"] = "continue", ["<leader>Db"] = "toggle_breakpoint", ["<leader>Do"] = "step_over",
  ["<leader>Di"] = "step_into", ["<leader>DO"] = "step_out", ["<leader>Dt"] = "terminate",
}) do
  assert(keys[key], "missing debug key: " .. key)
  keys[key]()
  assert(actions[#actions] == action, "incorrect debug action: " .. key)
end
require("config.dap").setup_ui()
session = { on_close = {} }
dap.listeners.after.event_initialized.selfishell(session)
assert(panel_open, "initialized session did not open UI")
assert(session.on_close.selfishell, "debug UI must also close on disconnect or adapter failure")
local closed = session
session = nil
closed.on_close.selfishell(closed)
assert(vim.wait(1000, function() return not panel_open end), "closed session left UI open")
keys["<leader>Du"]()
assert(panel_open, "manual UI toggle did not open UI")
vim.bo.filetype = "dapui_scopes"
assert(vim.fn.maparg("<C-j>", "n") == "<C-W>j", "debug UI broke window navigation")
session = {}
keys["<F5>"]()
assert(actions[#actions] == "continue", "F5 in a debug panel did not resume")

local custom = { type = "server", port = 9876 }
dap.adapters.python = custom
handler(config)
assert(dap.adapters.python == custom, "automatic setup replaced a user adapter")
local custom_switchbuf = function() end
dap.defaults.fallback.switchbuf = custom_switchbuf
require("config.dap").setup_start()
assert(dap.defaults.fallback.switchbuf == custom_switchbuf, "setup replaced personal frame navigation")
print("DAP configuration: OK")
