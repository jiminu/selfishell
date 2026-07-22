local map = vim.keymap.set
local M = {}

-- Clear search highlighting
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

-- Buffer management
function M.delete_buffer(bufnr)
  bufnr = bufnr or vim.api.nvim_get_current_buf()
  if not vim.api.nvim_buf_is_valid(bufnr) then
    return
  end

  local current_win = vim.api.nvim_get_current_win()
  local replacement
  local created_replacement = false

  if vim.api.nvim_win_get_buf(current_win) == bufnr then
    local listed = vim.tbl_filter(function(candidate)
      return vim.api.nvim_buf_is_valid(candidate) and vim.bo[candidate].buflisted
    end, vim.api.nvim_list_bufs())

    for index, candidate in ipairs(listed) do
      if candidate == bufnr then
        replacement = listed[index + 1] or listed[index - 1]
        break
      end
    end
    replacement = replacement or listed[1]
    if replacement == bufnr then
      replacement = nil
    end
    if not replacement then
      replacement = vim.api.nvim_create_buf(true, false)
      created_replacement = true
    end
    vim.api.nvim_win_set_buf(current_win, replacement)
  end

  local ok, err = pcall(vim.cmd, "confirm bdelete " .. bufnr)
  local target_remains = vim.api.nvim_buf_is_valid(bufnr) and vim.api.nvim_buf_is_loaded(bufnr)
  if not ok or target_remains then
    if vim.api.nvim_win_is_valid(current_win) and vim.api.nvim_buf_is_valid(bufnr) then
      vim.api.nvim_win_set_buf(current_win, bufnr)
    end
    if created_replacement and vim.api.nvim_buf_is_valid(replacement) then
      vim.api.nvim_buf_delete(replacement, { force = true })
    end
    if not ok then
      error(err)
    end
  end
end

map("n", "<leader>bd", function()
  M.delete_buffer()
end, {
  silent = true,
  desc = "Delete buffer",
})

-- Keep the selection active while adjusting indentation.
map("x", "<", "<gv", {
  desc = "Indent left and reselect",
})
map("x", ">", ">gv", {
  desc = "Indent right and reselect",
})

-- Diagnostic navigation
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
