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
        show_close_icon = false,
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
        globalstatus = true,
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

  -- Git changes, hunk actions, and blame information.
  plugin("lewis6991/gitsigns.nvim", {
    event = { "BufReadPre", "BufNewFile" },
    opts = {
      current_line_blame_opts = {
        delay = 500,
        ignore_whitespace = true,
      },
      preview_config = {
        border = "rounded",
      },
      on_attach = function(bufnr)
        local gitsigns = require("gitsigns")

        local function map(mode, lhs, rhs, desc)
          vim.keymap.set(mode, lhs, rhs, {
            buffer = bufnr,
            silent = true,
            desc = desc,
          })
        end

        -- Navigate Git changes while preserving Vim's diff-mode mappings.
        map("n", "]c", function()
          if vim.wo.diff then
            vim.cmd.normal({ "]c", bang = true })
          else
            gitsigns.nav_hunk("next")
          end
        end, "Next Git change")

        map("n", "[c", function()
          if vim.wo.diff then
            vim.cmd.normal({ "[c", bang = true })
          else
            gitsigns.nav_hunk("prev")
          end
        end, "Previous Git change")

        -- Hunk actions.
        map("n", "<leader>hp", gitsigns.preview_hunk, "Preview Git hunk")
        map("n", "<leader>hi", gitsigns.preview_hunk_inline, "Preview Git hunk inline")
        map("n", "<leader>hs", gitsigns.stage_hunk, "Stage Git hunk")
        map("n", "<leader>hr", gitsigns.reset_hunk, "Reset Git hunk")

        map("x", "<leader>hs", function()
          gitsigns.stage_hunk({
            vim.fn.line("."),
            vim.fn.line("v"),
          })
        end, "Stage selected Git lines")

        map("x", "<leader>hr", function()
          gitsigns.reset_hunk({
            vim.fn.line("."),
            vim.fn.line("v"),
          })
        end, "Reset selected Git lines")

        -- Blame and diff.
        map("n", "<leader>hb", function()
          gitsigns.blame_line({ full = true })
        end, "Show Git blame")

        map("n", "<leader>hd", gitsigns.diffthis, "Diff against Git index")

        map("n", "<leader>hD", function()
          gitsigns.diffthis("~")
        end, "Diff against previous commit")

        -- Optional visual features.
        map("n", "<leader>tb", gitsigns.toggle_current_line_blame, "Toggle Git blame")
        map("n", "<leader>tw", gitsigns.toggle_word_diff, "Toggle Git word diff")

        -- Git hunk text object.
        map({ "o", "x" }, "ih", gitsigns.select_hunk, "Select Git hunk")
      end,
    },
  }),

  -- Scrollbar with the current viewport and diagnostics.
  plugin("petertriho/nvim-scrollbar", {
    main = "scrollbar",
    event = { "BufReadPost", "BufNewFile" },
    opts = {
      show_in_active_only = true,
      hide_if_all_visible = true,
      -- The default handle color (linked to CursorColumn) is nearly
      -- indistinguishable from vscode.nvim's background. Use VS Code's own
      -- scrollbar slider color/opacity instead of a fully opaque gray.
      handle = {
        blend = 60,
        color = "#797979",
      },
      excluded_filetypes = {
        "cmp_docs",
        "cmp_menu",
        "prompt",
        "TelescopePrompt",
        "NvimTree",
        "lazy",
        "mason",
        "help",
      },
      -- gitsigns is deliberately left off: the sign column already shows the
      -- same hunks per-line, and mirroring them here doubled the redraw
      -- triggers (gitsigns update + diagnostic update) for marginal benefit.
      handlers = {
        cursor = false,
        diagnostic = true,
        handle = true,
        search = false,
        ale = false,
      },
    },
  }),
}
