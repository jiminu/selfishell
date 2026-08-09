local languages = require("config.languages")
local plugin = require("config.plugin_versions").spec

return {
  -- Mason UI can also be opened before any source file is read.
  plugin("mason-org/mason.nvim", {
    cmd = {
      "Mason",
      "MasonInstall",
      "MasonUninstall",
      "MasonUpdate",
      "MasonLog",
    },
    opts = {},
  }),

  plugin("mason-org/mason-lspconfig.nvim", {
    -- Not `ft`-gated: a user-installed server (`:LspInstall`) outside
    -- Selfishell's default filetype list still needs this plugin's setup()
    -- to run and auto-enable it on a fresh Neovim process.
    event = { "BufReadPre", "BufNewFile" },
    cmd = {
      "LspInstall",
      "LspUninstall",
    },
    dependencies = {
      plugin("mason-org/mason.nvim"),
      plugin("neovim/nvim-lspconfig"),
    },
    config = function()
      require("mason-lspconfig").setup({
        ensure_installed = languages.lsp,
        automatic_enable = true,
      })
    end,
  }),
}
