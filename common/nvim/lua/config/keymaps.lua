local map = vim.keymap.set

-- Clear search highlighting
map("n", "<Esc>", "<cmd>nohlsearch<CR>", {
  silent = true,
  desc = "Clear search highlight",
})

-- Window navigation
map("n", "<C-h>", "<C-w>h", {
  desc = "Go to left window"
})
map("n", "<C-l>", "<C-w>l", {
  desc = "Go to right window"
})
map("n", "<C-j>", "<C-w>j", {
  desc = "Go to lower window"
})
map("n", "<C-k>", "<C-w>k", {
  desc = "Go to upper window"
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
