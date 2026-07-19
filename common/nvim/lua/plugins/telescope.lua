local plugin = require("config.plugin_versions").spec

return {
  plugin("nvim-telescope/telescope.nvim", {
    cmd = "Telescope",
    keys = {
      {
        "<leader>ff",
        "<cmd>Telescope find_files<CR>",
        desc = "Find files",
      },
      {
        "<leader>fF",
        function()
          require("telescope.builtin").find_files({
            hidden = true,
          })
        end,
        desc = "Find all files",
      },
      {
        "<leader>fg",
        "<cmd>Telescope live_grep<CR>",
        desc = "Live grep",
      },
      {
        "<leader>fG",
        function()
          require("telescope.builtin").live_grep({
            additional_args = function()
              return {
                "--hidden",
                "--glob",
                "!**/.git/**",
              }
            end,
          })
        end,
        desc = "Live grep all files",
      },
      {
        "<leader>fb",
        "<cmd>Telescope buffers<CR>",
        desc = "Buffers",
      },
      {
        "<leader>fh",
        "<cmd>Telescope help_tags<CR>",
        desc = "Help tags",
      },
      {
        "<leader>fd",
        "<cmd>Telescope diagnostics<CR>",
        desc = "Diagnostics",
      },
      {
        "<leader>fs",
        "<cmd>Telescope lsp_document_symbols<CR>",
        desc = "Document symbols",
      },
      {
        "<leader>fS",
        "<cmd>Telescope lsp_dynamic_workspace_symbols<CR>",
        desc = "Workspace symbols",
      },
      {
        "<leader>fr",
        "<cmd>Telescope resume<CR>",
        desc = "Resume picker",
      },
    },
    dependencies = {
      plugin("nvim-lua/plenary.nvim"),
      plugin("nvim-telescope/telescope-fzf-native.nvim", {
        build = "make",
        cond = function()
          return vim.fn.executable("make") == 1
        end,
      }),
    },
    config = function()
      local telescope = require("telescope")
      local actions = require("telescope.actions")

      telescope.setup({
        defaults = {
          mappings = {
            i = {
              ["<C-j>"] = actions.move_selection_next,
              ["<C-k>"] = actions.move_selection_previous,
            },
          },
        },
      })

      pcall(telescope.load_extension, "fzf")
    end,
  }),
}
