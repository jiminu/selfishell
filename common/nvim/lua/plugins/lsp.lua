local languages = require("config.languages")

return {
  -- Mason UI can also be opened before any source file is read.
  {
    "mason-org/mason.nvim",
    cmd = {
      "Mason",
      "MasonInstall",
      "MasonUninstall",
      "MasonUpdate",
      "MasonLog",
    },
    opts = {},
  },

  {
    "mason-org/mason-lspconfig.nvim",
    event = { "BufReadPre", "BufNewFile" },
    cmd = {
      "LspInstall",
      "LspUninstall",
    },
    dependencies = {
      "mason-org/mason.nvim",
      "neovim/nvim-lspconfig",
      "hrsh7th/cmp-nvim-lsp",
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
  },
}
