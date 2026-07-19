local plugin = require("config.plugin_versions").spec

return {
  -- Theme: must be available during startup.
  plugin("mofiqul/vscode.nvim", {
    lazy = false,
    priority = 1000,
    config = function()
      vim.cmd.colorscheme("vscode")
    end,
  }),

  -- File explorer: loaded only when its command or keymap is used.
  plugin("nvim-tree/nvim-tree.lua", {
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
      {
        "<leader>E",
        "<cmd>NvimTreeFindFile<CR>",
        desc = "Reveal current file",
      },
    },
    dependencies = {
      plugin("nvim-tree/nvim-web-devicons"),
    },
    opts = {},
  }),

  -- Statusline: not required for the critical startup path.
  plugin("nvim-lualine/lualine.nvim", {
    event = "VeryLazy",
    dependencies = {
      plugin("nvim-tree/nvim-web-devicons"),
    },
    opts = {
      options = {
        theme = "vscode",
        icons_enabled = true,
      },
    },
  }),
}
