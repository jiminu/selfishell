local plugin = require("config.plugin_versions").spec

return {
  -- Automatic bracket/quote pairs.
  plugin("windwp/nvim-autopairs", {
    event = "InsertEnter",
    opts = {},
  }),

  -- nvim-treesitter 1.0+ does not support lazy-loading.
  plugin("nvim-treesitter/nvim-treesitter", {
    lazy = false,
    build = ":TSUpdate",
    config = function()
      require("nvim-treesitter").setup({
        install_dir = vim.fn.stdpath("data") .. "/site",
      })
    end,
  }),

  -- VS Code-style colored delimiters.
  plugin("HiPhish/rainbow-delimiters.nvim", {
    event = "VeryLazy",
    dependencies = {
      plugin("nvim-treesitter/nvim-treesitter"),
    },
    init = function()
      vim.g.rainbow_delimiters = {
        strategy = {
          [""] = "rainbow-delimiters.strategy.global",
        },
        query = {
          [""] = "rainbow-delimiters",
        },
      }
    end,
  }),

  -- Keymap guide: helpful for Space leader mappings.
  plugin("folke/which-key.nvim", {
    event = "VeryLazy",
    opts = {},
  }),
}
