local plugin = require("config.plugin_versions").spec

local excluded_scope_filetypes = {
  NvimTree = true,
  help = true,
  lazy = true,
  log = true,
  markdown = true,
  mason = true,
  text = true,
}

local function scope_buffer_enabled(buf)
  return vim.bo[buf].buftype == "" and not excluded_scope_filetypes[vim.bo[buf].filetype]
end

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
  plugin("mofiqul/vscode.nvim", {
    lazy = false,
    priority = 1000,
    config = function()
      vim.cmd.colorscheme("vscode")
    end,
  }),

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
        "<cmd>NvimTreeFindFile!<CR>",
        desc = "Reveal current file",
      },
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
      -- nvim-tree hides gitignored entries entirely by default; show them
      -- so they can be dimmed by the git decorator below instead.
      filters = {
        git_ignored = false,
      },
      renderer = {
        add_trailing = true,
        group_empty = true,
        highlight_git = "name",
        indent_markers = {
          enable = true,
          inline_arrows = true,
          icons = {
            edge = " ",
          },
        },
        icons = {
          git_placement = "right_align",
          glyphs = {
            folder = {
              arrow_closed = ">",
              arrow_open = "v",
            },
            git = {
              unstaged = "M",
              staged = "S",
              unmerged = "!",
              renamed = "R",
              untracked = "U",
              deleted = "D",
              ignored = "",
            },
          },
          show = {
            file = false,
            folder = false,
            folder_arrow = true,
            git = true,
          },
        },
      },
    },
  }),

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
    opts = {
      options = {
        always_show_bufferline = false,
        -- Disable icon lookups to avoid loading nvim-web-devicons.
        show_buffer_icons = false,
        close_command = function(bufnr)
          Snacks.bufdelete(bufnr)
        end,
        right_mouse_command = function(bufnr)
          Snacks.bufdelete(bufnr)
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
          bold = false,
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
        duplicate = {
          fg = "#858585",
          bg = { attribute = "bg", highlight = "TabLine" },
          italic = true,
        },
        duplicate_visible = {
          fg = "#858585",
          bg = { attribute = "bg", highlight = "TabLine" },
          italic = true,
        },
        duplicate_selected = {
          fg = "#858585",
          bg = { attribute = "bg", highlight = "TabLineSel" },
          italic = true,
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

  plugin("nvim-lualine/lualine.nvim", {
    event = "VeryLazy",
    opts = {
      options = {
        theme = "vscode",
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
            color = { fg = "#5fd700" },
            icon = "",
            padding = { left = 0, right = 1 },
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
          {
            "filetype",
            icons_enabled = false,
          },
          "location",
          "progress",
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

  -- Load eagerly and enable directly: Snacks' BufReadPost hook misses new and unnamed buffers.
  plugin("folke/snacks.nvim", {
    lazy = false,
    priority = 1000,
    keys = {
      {
        "<leader>ff",
        function()
          Snacks.picker.files()
        end,
        desc = "Find files",
      },
      {
        "<leader>fF",
        function()
          Snacks.picker.files({ hidden = true })
        end,
        desc = "Find all files",
      },
      {
        "<leader>fg",
        function()
          Snacks.picker.grep()
        end,
        desc = "Live grep",
      },
      {
        "<leader>fG",
        function()
          Snacks.picker.grep({ hidden = true })
        end,
        desc = "Live grep all files",
      },
      {
        "<leader>fb",
        function()
          Snacks.picker.buffers()
        end,
        desc = "Buffers",
      },
      {
        "<leader>fh",
        function()
          Snacks.picker.help()
        end,
        desc = "Help tags",
      },
      {
        "<leader>fd",
        function()
          Snacks.picker.diagnostics()
        end,
        desc = "Diagnostics",
      },
      {
        "<leader>fs",
        function()
          Snacks.picker.lsp_symbols()
        end,
        desc = "Document symbols",
      },
      {
        "<leader>fS",
        function()
          Snacks.picker.lsp_workspace_symbols()
        end,
        desc = "Workspace symbols",
      },
      {
        "<leader>fr",
        function()
          Snacks.picker.resume()
        end,
        desc = "Resume picker",
      },
      {
        "<leader>/",
        function()
          Snacks.picker.lines()
        end,
        desc = "Fuzzy search in buffer",
      },
      -- Snacks' own <leader>g prefix: these ask what changed across the
      -- repository, while gitsigns' <leader>h acts on the hunk at the cursor.
      {
        "<leader>gg",
        function()
          Snacks.lazygit()
        end,
        desc = "Open Lazygit",
      },
      {
        "<leader>gs",
        function()
          Snacks.picker.git_status()
        end,
        desc = "Git status",
      },
      {
        "<leader>gd",
        function()
          Snacks.picker.git_diff()
        end,
        desc = "Changed hunks",
      },
      {
        "<leader>gl",
        function()
          Snacks.picker.git_log()
        end,
        desc = "Git log",
      },
      {
        "<leader>gf",
        function()
          Snacks.picker.git_log_file()
        end,
        desc = "Git log for current file",
      },
    },
    opts = {
      -- Selfishell installs ripgrep, but not fd.
      picker = {
        ui_select = false,
        sources = {
          files = {
            cmd = "rg",
            icons = { files = { enabled = false } },
          },
          grep = {
            icons = { files = { enabled = false } },
          },
          buffers = {
            icons = { files = { enabled = false } },
          },
          -- Include diagnostics across the whole workspace.
          diagnostics = {
            filter = { cwd = false },
          },
          git_status = {
            icons = { files = { enabled = false } },
          },
          git_diff = {
            icons = { files = { enabled = false } },
          },
        },
      },
      indent = {
        enabled = true,
        indent = {
          enabled = false,
        },
        animate = {
          enabled = false,
        },
        scope = {
          enabled = true,
          char = "│",
          underline = false,
          hl = "SnacksIndentScope",
          treesitter = {
            enabled = true,
            -- Disables Snacks' built-in block-node whitelist so scope isn't
            -- limited to a fixed set of Tree-sitter node types per language.
            blocks = { enabled = false },
          },
          filter = scope_buffer_enabled,
        },
        chunk = {
          enabled = false,
        },
        filter = scope_buffer_enabled,
      },
    },
    config = function(_, opts)
      require("snacks").setup(opts)
      Snacks.indent.enable()
    end,
  }),

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

        map("n", "<leader>hb", function()
          gitsigns.blame_line({ full = true })
        end, "Show Git blame")

        map("n", "<leader>hd", gitsigns.diffthis, "Diff against Git index")

        map("n", "<leader>hD", function()
          gitsigns.diffthis("~")
        end, "Diff against previous commit")

        map("n", "<leader>ub", gitsigns.toggle_current_line_blame, "Toggle Git blame")
        map("n", "<leader>uw", gitsigns.toggle_word_diff, "Toggle Git word diff")

        map({ "o", "x" }, "ih", gitsigns.select_hunk, "Select Git hunk")
      end,
    },
  }),

  plugin("petertriho/nvim-scrollbar", {
    main = "scrollbar",
    event = { "BufReadPost", "BufNewFile" },
    opts = {
      show_in_active_only = true,
      hide_if_all_visible = true,
      -- The default CursorColumn color is hard to see against this theme.
      handle = {
        blend = 60,
        color = "#797979",
      },
      excluded_filetypes = {
        "cmp_docs",
        "cmp_menu",
        "prompt",
        "NvimTree",
        "lazy",
        "mason",
        "help",
      },
      -- Keep cursor marks disabled to avoid redraws on every cursor move.
      handlers = {
        cursor = false,
        diagnostic = true,
        gitsigns = true,
        handle = true,
        search = false,
        ale = false,
      },
    },
  }),

  plugin("OXY2DEV/markview.nvim", {
    ft = { "markdown", "quarto", "rmd", "typst", "asciidoc" },
    keys = {
      {
        "<leader>um",
        "<cmd>Markview toggle<CR>",
        desc = "Toggle document preview",
      },
    },
    opts = {
      preview = {
        icon_provider = "internal",
      },
    },
  }),
}
