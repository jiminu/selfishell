local plugin = require("config.plugin_versions").spec

return {
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

  plugin("HiPhish/rainbow-delimiters.nvim", {
    -- Load before the initial buffer's FileType event so the plugin can attach.
    event = { "BufReadPre", "BufNewFile" },
    init = function()
      -- VS Code's three bracket colors appear in no syntax group; vscode.nvim's
      -- map reuses the keyword, string, and comment colors.
      local highlight = { "RainbowDelimiterYellow", "RainbowDelimiterViolet", "RainbowDelimiterBlue" }
      vim.api.nvim_create_autocmd("ColorScheme", {
        pattern = "vscode",
        callback = function()
          local colors = vim.o.background == "dark" and { "#FFD700", "#DA70D6", "#179FFF" }
            or { "#0431FA", "#319331", "#7B3814" }
          for index, group in ipairs(highlight) do
            vim.api.nvim_set_hl(0, group, { fg = colors[index] })
          end
        end,
      })
      vim.g.rainbow_delimiters = {
        highlight = highlight,
        -- The global strategy marks every delimiter: seconds of freeze on a
        -- large file, minutes on a minified one. ~10 ms per 1,000 delimiters.
        condition = function(bufnr)
          local lines = vim.api.nvim_buf_line_count(bufnr)
          return lines <= 5000 and vim.api.nvim_buf_get_offset(bufnr, lines) / lines <= 500
        end,
        strategy = {
          [""] = "rainbow-delimiters.strategy.global",
        },
        query = {
          [""] = "rainbow-delimiters",
        },
      }
    end,
  }),

  plugin("folke/which-key.nvim", {
    event = "VeryLazy",
    keys = {
      {
        "<leader>w",
        function() require("which-key").show({ keys = "<leader>w", loop = true }) end,
        desc = "Resize windows",
      },
    },
    opts = {
      spec = {
        {
          "<leader>w",
          group = "Resize windows",
          -- Virtual keys keep the entry mapping immediate and h/j/k/l local
          -- to the popup. Which-key owns repetition and Escape handling.
          expand = function()
            return {
              { "h", function() vim.cmd("vertical resize -5") end, desc = "Width -5" },
              { "l", function() vim.cmd("vertical resize +5") end, desc = "Width +5" },
              { "j", function() vim.cmd("resize -2") end, desc = "Height -2" },
              { "k", function() vim.cmd("resize +2") end, desc = "Height +2" },
              { "=", function() vim.cmd("wincmd =") end, desc = "Equalize windows" },
            }
          end,
        },
      },
      icons = {
        -- which-key deep-merges keys, so every Nerd Font default needs an override.
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
