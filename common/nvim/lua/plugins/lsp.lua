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
    ft = languages.lsp_filetypes,
    cmd = {
      "LspInstall",
      "LspUninstall",
    },
    dependencies = {
      plugin("mason-org/mason.nvim"),
      plugin("neovim/nvim-lspconfig"),
      plugin("hrsh7th/cmp-nvim-lsp"),
    },
    config = function()
      -- Apply completion capabilities to every LSP config.
      vim.lsp.config("*", {
        capabilities = require("cmp_nvim_lsp").default_capabilities(),
      })

      -- mason-lspconfig installs and automatically enables these servers.
      require("mason-lspconfig").setup({
        ensure_installed = languages.lsp,
      })
    end,
  }),
}
