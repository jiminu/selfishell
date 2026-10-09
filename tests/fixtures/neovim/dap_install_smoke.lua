local lazy = require("lazy.core.config").plugins
assert(not lazy["nvim-dap"]._.loaded, "debugger loaded before a debug action")
vim.cmd.edit(vim.env.SELFISHELL_DAP_PROJECT .. "/main.ts")
local notify, notices = vim.notify, {}
vim.notify = function(message) notices[#notices + 1] = message end
vim.api.nvim_feedkeys(vim.keycode("<F5>"), "xt", false)
assert(vim.wait(1000, function()
  for _, message in ipairs(notices) do
    if message:find(":DapInstall js", 1, true) then return true end
  end
end), "F5 without an installed adapter did not show installation guidance")
local dap = require("dap")
assert(not dap.session() and not dap.adapters["pwa-node"], "installation hint started or installed a debugger")
vim.notify = notify
vim.cmd("DapInstall python delve js")
local registry = require("mason-registry")
assert(vim.wait(180000, function()
  return registry.get_package("debugpy"):is_installed()
    and registry.get_package("delve"):is_installed()
    and registry.get_package("js-debug-adapter"):is_installed()
    and dap.adapters.python ~= nil and dap.adapters.delve ~= nil
    and dap.adapters["pwa-node"] ~= nil and dap.adapters.node ~= nil
end, 100), "adapters did not install and register in the current process; see MasonLog")
assert(#dap.configurations.python == 1, "Python default launch missing or duplicated")
assert(#dap.configurations.go > 0, "Go default launches missing")
assert(#dap.configurations.javascript > 0 and #dap.configurations.typescript > 0, "JS/TS default launches missing")
print("DAP consumer: OK")
