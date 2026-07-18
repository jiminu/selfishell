return {
  {
    "nvim-telescope/telescope.nvim",
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
    },
    dependencies = {
      "nvim-lua/plenary.nvim",
      {
        "nvim-telescope/telescope-fzf-native.nvim",
        build = "make",
        cond = function()
          return vim.fn.executable("make") == 1
        end,
      },
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
  },
}
