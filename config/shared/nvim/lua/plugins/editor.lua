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
    -- Load before the initial buffer's FileType event so the plugin can attach.
    event = { "BufReadPre", "BufNewFile" },
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
    opts = {
      icons = {
        -- Disable per-mapping filetype/devicons icon lookups; which-key
        -- deep-merges `keys` with its Nerd Font defaults, so every key
        -- must be listed here explicitly or it keeps its default glyph.
        mappings = false,
        keys = {
          Up = "Up ",
          Down = "Down ",
          Left = "Left ",
          Right = "Right ",
          C = "C-",
          M = "M-",
          D = "D-",
          S = "S-",
          CR = "Enter ",
          Esc = "Esc ",
          ScrollWheelDown = "ScrollDown ",
          ScrollWheelUp = "ScrollUp ",
          NL = "Enter ",
          BS = "Backspace ",
          Space = "Space ",
          Tab = "Tab ",
          F1 = "F1 ",
          F2 = "F2 ",
          F3 = "F3 ",
          F4 = "F4 ",
          F5 = "F5 ",
          F6 = "F6 ",
          F7 = "F7 ",
          F8 = "F8 ",
          F9 = "F9 ",
          F10 = "F10 ",
          F11 = "F11 ",
          F12 = "F12 ",
        },
      },
    },
  }),
}
