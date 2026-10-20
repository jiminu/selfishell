local plugin = require("config.plugin_versions").spec

return {
  plugin("saghen/blink.cmp", {
    event = { "InsertEnter", "CmdlineEnter" },
    opts = function()
      local kind_icons = {}
      for kind, icon in pairs(Snacks.picker.config.get().icons.kinds) do
        kind_icons[kind] = vim.trim(icon)
      end

      return {
        -- Explicit mappings avoid behavior changes from preset updates.
        keymap = {
          preset = "none",
          ["<C-b>"] = { "scroll_documentation_up", "fallback" },
          ["<C-f>"] = { "scroll_documentation_down", "fallback" },
          ["<C-Space>"] = { "show", "show_documentation", "hide_documentation" },
          ["<C-e>"] = { "hide", "fallback" },
          -- Enter accepts the first item even without an explicit selection.
          ["<CR>"] = { "select_and_accept", "fallback" },
          ["<Tab>"] = { "select_next", "snippet_forward", "fallback" },
          ["<S-Tab>"] = { "select_prev", "snippet_backward", "fallback" },
        },
        appearance = { kind_icons = kind_icons },
        completion = {
          list = { selection = { preselect = false, auto_insert = true } },
          menu = { border = "rounded", max_height = 10 },
          documentation = { auto_show = true, auto_show_delay_ms = 200, window = { border = "rounded" } },
        },
        signature = { enabled = true, window = { border = "rounded" } },
        sources = { default = { "lsp", "path", "buffer" } },
        -- The Rust matcher adds typo tolerance and frecency but needs a
        -- per-platform prebuilt binary; the Lua matcher needs nothing extra.
        fuzzy = { implementation = "lua" },
      }
    end,
  }),
}
