local languages = require("config.languages")

return {
  -- Automatic bracket/quote pairs.
  {
    "windwp/nvim-autopairs",
    event = "InsertEnter",
    opts = {},
  },

  -- Tree-sitter parsing is enabled when a buffer is opened; parser installation
  -- happens during `selfishell install`, not at editor startup.
  {
    "nvim-treesitter/nvim-treesitter",
    event = { "BufReadPost", "BufNewFile" },
    build = ":TSUpdate",
    opts = {
      highlight = {
        enable = true,
      },
    },
  },

  -- VS Code-style colored delimiters.
  {
    "HiPhish/rainbow-delimiters.nvim",
    event = "VeryLazy",
    dependencies = {
      "nvim-treesitter/nvim-treesitter",
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
  },

  -- Keymap guide: helpful for Space leader mappings.
  {
    "folke/which-key.nvim",
    event = "VeryLazy",
    opts = {},
  },
}
