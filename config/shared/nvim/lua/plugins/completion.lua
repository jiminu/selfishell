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
      local kind_icons = Snacks.picker.config.get().icons.kinds

      cmp.setup({
        formatting = {
          format = function(_, item)
            item.icon = vim.trim(kind_icons[item.kind] or "")
            item.icon_hl_group = "CmpItemKind" .. item.kind
            return item
          end,
        },

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

          -- Enter accepts the first item even without an explicit selection.
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
