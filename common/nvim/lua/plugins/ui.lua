return {
  -- Theme: must be available during startup.
  {
    "mofiqul/vscode.nvim",
    lazy = false,
    priority = 1000,
    config = function()
      vim.cmd.colorscheme("vscode")
    end,
  },

  -- File explorer: loaded only when its command or keymap is used.
  {
    "nvim-tree/nvim-tree.lua",
    main = "nvim-tree",
    cmd = {
      "NvimTreeToggle",
      "NvimTreeOpen",
      "NvimTreeFindFile",
      "NvimTreeFocus",
    },
    keys = {
      {
        "<leader>e",
        "<cmd>NvimTreeToggle<CR>",
        desc = "Toggle file explorer",
      },
    },
    dependencies = {
      "nvim-tree/nvim-web-devicons",
    },
    opts = {},
  },

  -- Statusline: not required for the critical startup path.
  {
    "nvim-lualine/lualine.nvim",
    event = "VeryLazy",
    dependencies = {
      "nvim-tree/nvim-web-devicons",
    },
    opts = {
      options = {
        theme = "vscode",
        icons_enabled = true,
      },
    },
  },
}
