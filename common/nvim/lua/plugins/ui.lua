local plugin = require("config.plugin_versions").spec

local function lualine_mode_color()
  local dark = vim.o.background == "dark"
  local colors = dark and {
    background = "#262626",
    normal = "#0A7ACA",
    insert = "#4EC9B0",
    visual = "#FFAF00",
    replace = "#F44747",
    command = "#DDB6F2",
  } or {
    background = "#F5F5F5",
    normal = "#AF00DB",
    insert = "#008000",
    visual = "#C08000",
    replace = "#FF0000",
    command = "#FFA3A3",
  }
  local modes = {
    i = "insert",
    R = "replace",
    s = "visual",
    t = "insert",
    v = "visual",
    V = "visual",
    ["\22"] = "visual",
    c = "command",
  }
  return {
    fg = colors[modes[vim.fn.mode(1):sub(1, 1)] or "normal"],
    bg = colors.background,
    gui = "bold",
  }
end

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
      on_attach = function(bufnr)
        require("nvim-tree.api").config.mappings.default_on_attach(bufnr)
        require("config.keymaps").set_window_navigation({ buffer = bufnr })
      end,
      view = {
        -- Scale with the terminal width instead of nvim-tree's default
        -- content-based adaptive sizing, which ignores terminal size.
        width = function()
          local computed = math.floor(vim.o.columns * 0.25)
          return math.max(20, math.min(30, computed))
        end,
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
        globalstatus = true,
        component_separators = "",
        section_separators = "",
      },
      sections = {
        lualine_a = {
          {
            "mode",
            color = lualine_mode_color,
          },
        },
        lualine_b = {},
        lualine_c = {
          {
            "branch",
            color = lualine_mode_color,
          },
          {
            "filename",
            path = 0,
            symbols = {
              modified = " ●",
              readonly = " 󰌾",
              unnamed = "[No Name]",
              newfile = "[New]",
            },
          },
          {
            "diagnostics",
            sources = { "nvim_diagnostic" },
            sections = { "error", "warn" },
            symbols = {
              error = " ",
              warn = " ",
            },
          },
        },
        lualine_x = {
          "filetype",
          "location",
        },
        lualine_y = {},
        lualine_z = {},
      },
      inactive_sections = {
        lualine_a = {},
        lualine_b = {},
        lualine_c = {},
        lualine_x = {},
        lualine_y = {},
        lualine_z = {},
      },
    },
  }),
}
