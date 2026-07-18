local languages = require("config.languages")

return {
  -- Automatic bracket/quote pairs.
  {
    "windwp/nvim-autopairs",
    event = "InsertEnter",
    opts = {},
  },

  -- Tree-sitter itself does not support lazy-loading on its current main branch.
  {
    "nvim-treesitter/nvim-treesitter",
    lazy = false,
    build = ":TSUpdate",
    config = function()
      local treesitter = require("nvim-treesitter")

      -- No-op for parsers that are already installed.
      treesitter.install(languages.treesitter)

      local group = vim.api.nvim_create_augroup(
        "UserTreesitterHighlight",
        { clear = true }
      )

      vim.api.nvim_create_autocmd("FileType", {
        group = group,
        pattern = "*",
        callback = function(args)
          -- Starts highlighting when a parser is available. Installation is
          -- asynchronous, so a newly requested parser may not be ready on the
          -- very first buffer open.
          pcall(vim.treesitter.start, args.buf)
        end,
      })
    end,
  },

  -- VS Code-style colored delimiters.
  {
    "HiPhish/rainbow-delimiters.nvim",
    event = { "BufReadPost", "BufNewFile" },
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
