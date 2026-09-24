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
    -- VeryLazy, not `ft`: an :LspInstall server outside the default filetypes
    -- still needs setup() on a fresh process, and vim.lsp.enable() re-fires
    -- FileType for buffers opened before it loads.
    event = "VeryLazy",
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
      vim.lsp.config("*", {
        capabilities = require("cmp_nvim_lsp").default_capabilities(),
      })

      require("mason-lspconfig").setup({
        ensure_installed = languages.lsp,
        automatic_enable = true,
      })
    end,
  }),
}
