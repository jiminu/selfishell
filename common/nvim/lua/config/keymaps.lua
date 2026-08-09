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

-- Buffer management: deferred to call time since Snacks isn't guaranteed to
-- be loaded yet when this module is evaluated.
map("n", "<leader>bd", function()
  Snacks.bufdelete()
end, {
  silent = true,
  desc = "Delete buffer",
})
map("n", "[b", "<cmd>bprevious<CR>", { silent = true, desc = "Previous buffer" })
map("n", "]b", "<cmd>bnext<CR>", { silent = true, desc = "Next buffer" })

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

    -- autotrigger only fires on the server's own triggerCharacters (mostly
    -- punctuation, e.g. "."), not on ordinary identifier characters -- that
    -- gap is covered separately below, once, for every buffer.
    local client = vim.lsp.get_client_by_id(args.data.client_id)
    if client and client:supports_method("textDocument/completion") then
      vim.lsp.completion.enable(true, client.id, args.buf, {
        autotrigger = true,
      })
    end
  end,
})

-- Completes on ordinary identifier characters, since autotrigger above only
-- covers the server's triggerCharacters. Word characters are never among a
-- server's triggerCharacters in practice (those are punctuation), so this
-- never fires alongside autotrigger for the same keystroke. get() already
-- no-ops when there's no attached client or the popup is already showing a
-- complete list, so no separate buffer/visibility guard is needed here.
vim.api.nvim_create_autocmd("InsertCharPre", {
  group = group,
  callback = function()
    if vim.v.char:match("[%w_]") and #vim.lsp.get_clients({ bufnr = 0, method = "textDocument/completion" }) > 0 then
      vim.lsp.completion.get()
    end
  end,
})

map("i", "<C-Space>", function()
  vim.lsp.completion.get()
end, { desc = "Trigger completion" })

map({ "i", "s" }, "<Tab>", function()
  if vim.fn.pumvisible() == 1 then
    return "<C-n>"
  elseif vim.snippet.active({ direction = 1 }) then
    return "<Cmd>lua vim.snippet.jump(1)<CR>"
  end
  return "<Tab>"
end, { expr = true, silent = true, desc = "Next completion item or snippet tabstop" })

map({ "i", "s" }, "<S-Tab>", function()
  if vim.fn.pumvisible() == 1 then
    return "<C-p>"
  elseif vim.snippet.active({ direction = -1 }) then
    return "<Cmd>lua vim.snippet.jump(-1)<CR>"
  end
  return "<S-Tab>"
end, { expr = true, silent = true, desc = "Previous completion item or snippet tabstop" })

-- Preserves the previous completion setup's behavior: Enter accepts the
-- first item even when it has not been explicitly selected.
map("i", "<CR>", function()
  if vim.fn.pumvisible() == 1 then
    return vim.fn.complete_info({ "selected" }).selected == -1 and "<C-n><C-y>" or "<C-y>"
  end
  return "<CR>"
end, { expr = true, silent = true, desc = "Confirm completion or insert newline" })

return M
