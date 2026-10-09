local lazy = require("lazy.core.config").plugins
assert(not lazy["nvim-dap"]._.loaded, "debugger loaded before a debug action")
vim.cmd("DapInstall python delve")
local registry = require("mason-registry")
local dap = require("dap")
assert(vim.wait(180000, function()
  return registry.get_package("debugpy"):is_installed()
    and registry.get_package("delve"):is_installed()
    and dap.adapters.python ~= nil and dap.adapters.delve ~= nil
end, 100), "adapters did not install and register in the current process; see MasonLog")
assert(#dap.configurations.python == 1, "Python default launch missing or duplicated")
assert(#dap.configurations.go > 0, "Go default launches missing")
print("DAP consumer: OK")
