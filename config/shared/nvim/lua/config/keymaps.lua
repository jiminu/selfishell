local map = vim.keymap.set
local M = {}

map("n", "<Esc>", "<cmd>nohlsearch<CR>", {
  silent = true,
  desc = "Clear search highlight",
})

-- Window navigation. Plugins with buffer-local mappings can call this after
-- their defaults to preserve the same navigation keys.
function M.set_window_navigation(options)
  options = options or {}
  local mappings = {
    ["<C-h>"] = { "<C-W>h", "Go to left window" },
    ["<C-j>"] = { "<C-W>j", "Go to lower window" },
    ["<C-k>"] = { "<C-W>k", "Go to upper window" },
    ["<C-l>"] = { "<C-W>l", "Go to right window" },
  }

  for lhs, mapping in pairs(mappings) do
    map("n", lhs, mapping[1], vim.tbl_extend("force", {
      desc = mapping[2],
      silent = true,
    }, options))
  end
end

M.set_window_navigation()

map("n", "<leader>w", function()
  local function move_border(horizontal, offset)
    local win = vim.fn.winnr()
    -- Prefer the right/bottom border. At the screen edge, move the shared
    -- left/top border instead, keeping the same on-screen direction.
    if vim.fn.winnr(horizontal and "l" or "j") == win then
      local adjacent = vim.fn.winnr(horizontal and "h" or "k")
      if adjacent == win then return end
      win = adjacent
    end
    if horizontal then
      vim.fn.win_move_separator(win, offset)
    else
      vim.fn.win_move_statusline(win, offset)
    end
  end
  local commands = {
    h = function() move_border(true, -5) end,
    l = function() move_border(true, 5) end,
    k = function() move_border(false, -2) end,
    j = function() move_border(false, 2) end,
    ["="] = function() vim.cmd("wincmd =") end,
  }
  while true do
    vim.api.nvim_echo({ { "Resize: h/l left/right 5 | k/j up/down 2 | = equalize | Esc done", "ModeMsg" } }, false, {})
    vim.cmd.redraw()
    local ok, key = pcall(vim.fn.getcharstr)
    if not ok or not commands[key] then
      vim.api.nvim_echo({ { "" } }, false, {})
      if ok and key ~= vim.keycode("<Esc>") then
        -- Replay only the exit key, ahead of any remaining input. A ':' or
        -- window navigation key should perform its ordinary mapped action.
        vim.api.nvim_feedkeys(key, "mi", false)
      end
      return
    end
    commands[key]()
  end
end, { desc = "Resize windows", silent = true })

-- Buffer management: deferred to call time since Snacks isn't guaranteed to
-- be loaded yet when this module is evaluated.
map("n", "<leader>bd", function()
  Snacks.bufdelete()
end, {
  silent = true,
  desc = "Delete buffer",
})

map("x", "<", "<gv", {
  desc = "Indent left and reselect",
})
map("x", ">", ">gv", {
  desc = "Indent right and reselect",
})

map("n", "[d", function()
  vim.diagnostic.jump({ count = -1, float = true })
end, { desc = "Previous diagnostic" })

map("n", "]d", function()
  vim.diagnostic.jump({ count = 1, float = true })
end, { desc = "Next diagnostic" })

-- LSP mappings are created only for buffers with an attached LSP client.
local group = vim.api.nvim_create_augroup("UserLspKeymaps", { clear = true })

vim.api.nvim_create_autocmd("LspAttach", {
  group = group,
  callback = function(args)
    local function lsp_map(lhs, rhs, desc)
      map("n", lhs, rhs, {
        buffer = args.buf,
        silent = true,
        desc = desc,
      })
    end

    lsp_map("gd", vim.lsp.buf.definition, "Go to definition")
    lsp_map("K", vim.lsp.buf.hover, "Hover documentation")
    lsp_map("<leader>rn", vim.lsp.buf.rename, "Rename symbol")
    lsp_map("<leader>ca", vim.lsp.buf.code_action, "Code action")
    lsp_map("<leader>d", vim.diagnostic.open_float, "Show line diagnostics")
  end,
})

return M
