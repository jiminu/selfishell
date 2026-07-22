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
    opts = {
      view = {
        width = 40,
      },
    },
  }),

  -- Buffer tabs: keep the critical startup path clear and hide the bar when a
  -- single buffer is open.
  plugin("akinsho/bufferline.nvim", {
    event = "VeryLazy",
    keys = {
      {
        "[b",
        "<cmd>BufferLineCyclePrev<CR>",
        desc = "Previous buffer",
      },
      {
        "]b",
        "<cmd>BufferLineCycleNext<CR>",
        desc = "Next buffer",
      },
    },
    dependencies = {
      plugin("nvim-tree/nvim-web-devicons"),
    },
    opts = {
      options = {
        always_show_bufferline = false,
        close_command = function(bufnr)
          require("config.keymaps").delete_buffer(bufnr)
        end,
        right_mouse_command = function(bufnr)
          require("config.keymaps").delete_buffer(bufnr)
        end,
        indicator = {
          style = "underline",
        },
        separator_style = "thin",
        show_close_icon = false,
        show_buffer_close_icons = true,
        hover = {
          enabled = true,
          delay = 150,
          reveal = { "close" },
        },
        max_name_length = 24,
        tab_size = 16,
        offsets = {
          {
            filetype = "NvimTree",
            text = "EXPLORER",
            text_align = "left",
            highlight = "TabLineFill",
            separator = true,
          },
        },
      },
      highlights = {
        fill = {
          fg = { attribute = "fg", highlight = "TabLineFill" },
          bg = { attribute = "bg", highlight = "TabLineFill" },
        },
        background = {
          fg = "#858585",
          bg = { attribute = "bg", highlight = "TabLine" },
          italic = false,
        },
        buffer_visible = {
          fg = "#858585",
          bg = { attribute = "bg", highlight = "TabLine" },
          italic = false,
        },
        buffer_selected = {
          fg = "#FFFFFF",
          bg = { attribute = "bg", highlight = "TabLineSel" },
          bold = true,
          italic = false,
          underline = true,
          sp = "#007ACC",
        },
        close_button = {
          fg = "#858585",
          bg = { attribute = "bg", highlight = "TabLine" },
        },
        close_button_visible = {
          fg = "#858585",
          bg = { attribute = "bg", highlight = "TabLine" },
        },
        close_button_selected = {
          fg = "#D4D4D4",
          bg = { attribute = "bg", highlight = "TabLineSel" },
          underline = true,
          sp = "#007ACC",
        },
        modified = {
          fg = "#D7BA7D",
          bg = { attribute = "bg", highlight = "TabLine" },
        },
        modified_visible = {
          fg = "#D7BA7D",
          bg = { attribute = "bg", highlight = "TabLine" },
        },
        modified_selected = {
          fg = "#D7BA7D",
          bg = { attribute = "bg", highlight = "TabLineSel" },
          underline = true,
          sp = "#007ACC",
        },
        separator = {
          fg = { attribute = "bg", highlight = "TabLineFill" },
          bg = { attribute = "bg", highlight = "TabLine" },
        },
        separator_visible = {
          fg = { attribute = "bg", highlight = "TabLineFill" },
          bg = { attribute = "bg", highlight = "TabLine" },
        },
        separator_selected = {
          fg = { attribute = "bg", highlight = "TabLineFill" },
          bg = { attribute = "bg", highlight = "TabLineSel" },
          underline = true,
          sp = "#007ACC",
        },
        indicator_selected = {
          fg = "#007ACC",
          bg = { attribute = "bg", highlight = "TabLineSel" },
          underline = true,
          sp = "#007ACC",
        },
      },
    },
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
