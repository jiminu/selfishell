local plugin = require("config.plugin_versions").spec

return {
  plugin("hrsh7th/nvim-cmp", {
    event = "InsertEnter",
    dependencies = {
      plugin("hrsh7th/cmp-nvim-lsp"),
      plugin("hrsh7th/cmp-buffer"),
      plugin("hrsh7th/cmp-path"),
    },
    config = function()
      local cmp = require("cmp")

      cmp.setup({
        -- Neovim 0.12's native snippet engine replaces LuaSnip; it expands
        -- the same LSP snippet syntax cmp already hands it.
        snippet = {
          expand = function(args)
            vim.snippet.expand(args.body)
          end,
        },

        -- Explicit mappings avoid behavior changes from preset updates.
        mapping = {
          ["<C-b>"] = cmp.mapping.scroll_docs(-4),
          ["<C-f>"] = cmp.mapping.scroll_docs(4),
          ["<C-Space>"] = cmp.mapping.complete(),

          -- Preserves the existing behavior: Enter accepts the first item
          -- even when it has not been explicitly selected.
          ["<CR>"] = cmp.mapping.confirm({ select = true }),

          ["<Tab>"] = cmp.mapping(function(fallback)
            if cmp.visible() then
              cmp.select_next_item()
            elseif vim.snippet.active({ direction = 1 }) then
              vim.snippet.jump(1)
            else
              fallback()
            end
          end, { "i", "s" }),

          ["<S-Tab>"] = cmp.mapping(function(fallback)
            if cmp.visible() then
              cmp.select_prev_item()
            elseif vim.snippet.active({ direction = -1 }) then
              vim.snippet.jump(-1)
            else
              fallback()
            end
          end, { "i", "s" }),
        },

        sources = cmp.config.sources({
          { name = "nvim_lsp" },
        }, {
          { name = "buffer" },
          { name = "path" },
        }),
      })
    end,
  }),
}
