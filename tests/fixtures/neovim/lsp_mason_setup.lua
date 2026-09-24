local languages = require("config.languages")
local target

for _, spec in ipairs(require("plugins.lsp")) do
  if spec[1] == "mason-org/mason-lspconfig.nvim" then
    target = spec
  end
end

assert(target, "mason-lspconfig spec is missing")

-- Not filetype-gated: an :LspInstall server outside the defaults still needs
-- setup() on a fresh process. VeryLazy keeps the registry work off the first screen.
assert(target.ft == nil, "mason-lspconfig plugin loading must not be limited to default LSP filetypes")
assert(target.event == "VeryLazy", "mason-lspconfig must load on VeryLazy: " .. vim.inspect(target.event))

package.preload["cmp_nvim_lsp"] = function()
  return {
    default_capabilities = function()
      return {}
    end,
  }
end

local setup_opts
package.preload["mason-lspconfig"] = function()
  return {
    setup = function(opts)
      setup_opts = opts
    end,
  }
end

target.config()

package.preload["cmp_nvim_lsp"] = nil
package.preload["mason-lspconfig"] = nil

assert(setup_opts, "mason-lspconfig.setup() was not called")
assert(
  vim.deep_equal(setup_opts.ensure_installed, languages.lsp),
  "mason-lspconfig must install Selfishell's default LSP servers: " .. vim.inspect(setup_opts.ensure_installed)
)
assert(setup_opts.automatic_enable == true, "mason-lspconfig must auto-enable installed servers")

print("LSP Mason setup: OK")
