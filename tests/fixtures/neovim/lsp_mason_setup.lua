local languages = require("config.languages")
local target

for _, spec in ipairs(require("plugins.lsp")) do
  if spec[1] == "mason-org/mason-lspconfig.nvim" then
    target = spec
  end
end

assert(target, "mason-lspconfig spec is missing")

-- A user-installed server (:LspInstall) outside Selfishell's default LSP
-- list still needs this plugin's setup() to run and auto-enable it on a
-- fresh Neovim process, so loading must not be limited by filetype.
assert(target.ft == nil, "mason-lspconfig plugin loading must not be limited to default LSP filetypes")
assert(
  vim.deep_equal(target.event, { "BufReadPre", "BufNewFile" }),
  "mason-lspconfig must load on BufReadPre/BufNewFile"
)

local setup_opts
package.preload["mason-lspconfig"] = function()
  return {
    setup = function(opts)
      setup_opts = opts
    end,
  }
end

target.config()

package.preload["mason-lspconfig"] = nil

assert(setup_opts, "mason-lspconfig.setup() was not called")
assert(
  vim.deep_equal(setup_opts.ensure_installed, languages.lsp),
  "mason-lspconfig must install Selfishell's default LSP servers: " .. vim.inspect(setup_opts.ensure_installed)
)
assert(setup_opts.automatic_enable == true, "mason-lspconfig must auto-enable installed servers")

print("LSP Mason setup: OK")
