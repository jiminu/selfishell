package.preload["config.plugin_versions"] = function()
  return { revision = function() return vim.env.SELFISHELL_TEST_REVISION end }
end
package.preload["lazy"] = function()
  return { setup = function() vim.g.selfishell_lazy_setup = true end }
end
vim.fn.system = function()
  error("Git fallback should not run for a detached HEAD")
end

require("config.lazy")
print(vim.g.selfishell_lazy_setup)
