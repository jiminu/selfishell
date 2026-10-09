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
local actions, notifications = {}, {}
local dap = { adapters = {}, configurations = {}, listeners = { after = { event_initialized = {} }, before = { event_terminated = {}, event_exited = {} } } }
local session
dap.session = function() return session end
for _, name in ipairs({ "continue", "toggle_breakpoint", "step_over", "step_into", "step_out", "terminate" }) do
  dap[name] = function() actions[#actions + 1] = name end
end
package.preload["dap"] = function() return dap end
local panel_open = false
package.preload["dapui"] = function()
  return {
    open = function() panel_open = true end,
    close = function() panel_open = false end,
    toggle = function() panel_open = not panel_open end,
    eval = function() actions[#actions + 1] = "eval" end,
  }
end
local registered = 0
package.preload["mason-nvim-dap"] = function()
  return { default_setup = function(config)
    registered = registered + 1
    dap.adapters[config.name] = config.adapters
    for _, ft in ipairs(config.filetypes) do
      dap.configurations[ft] = vim.list_extend(dap.configurations[ft] or {}, config.configurations)
    end
  end }
end
vim.notify = function(message) notifications[#notifications + 1] = message end
local keys = {}
for _, key in ipairs(bridge.keys) do keys[key[1]] = key[2] end
vim.bo.filetype = "python"
keys["<F5>"]()
assert(#actions == 0 and notifications[1]:find(":DapInstall"), "missing adapter needs actionable guidance")
assert(not panel_open, "missing adapter opened the debug UI")

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

for key, action in pairs({
  ["<F5>"] = "continue", ["<F9>"] = "toggle_breakpoint", ["<F10>"] = "step_over",
  ["<F11>"] = "step_into", ["<S-F11>"] = "step_out", ["<S-F5>"] = "terminate",
  ["<leader>Dc"] = "continue", ["<leader>Db"] = "toggle_breakpoint", ["<leader>Do"] = "step_over",
  ["<leader>Di"] = "step_into", ["<leader>DO"] = "step_out", ["<leader>Dt"] = "terminate",
}) do
  assert(keys[key], "missing debug key: " .. key)
  keys[key]()
  assert(actions[#actions] == action, "incorrect debug action: " .. key)
end
require("config.dap").setup_ui()
dap.listeners.after.event_initialized.selfishell()
assert(panel_open, "initialized session did not open UI")
dap.listeners.before.event_terminated.selfishell()
assert(not panel_open, "terminated session left UI open")
dap.listeners.after.event_initialized.selfishell()
dap.listeners.before.event_exited.selfishell()
assert(not panel_open, "exited session left UI open")
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
print("DAP configuration: OK")
