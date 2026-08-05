local group = vim.api.nvim_create_augroup("UserGeneralAutocmds", { clear = true })

-- Neovim 0.12 uses the built-in Tree-sitter highlighter. The current
-- nvim-treesitter plugin no longer enables it through setup()/opts.
vim.treesitter.language.register("terraform", "tf")

-- nvim-treesitter 1.0+ also dropped ensure_installed/auto_install from
-- setup(), so install a missing parser the first time its filetype is
-- opened rather than maintaining a separate static list to bulk-install
-- ahead of time.
local function ensure_parser_installed(buf, lang)
  local ok, treesitter = pcall(require, "nvim-treesitter")
  if not ok then
    return
  end

  if vim.list_contains(treesitter.get_installed("parsers"), lang) then
    return
  end
  if not vim.list_contains(treesitter.get_available(), lang) then
    return
  end

  treesitter.install(lang):await(function(_, installed)
    if installed and vim.api.nvim_buf_is_valid(buf) then
      pcall(vim.treesitter.start, buf, lang)
    end
  end)
end

vim.api.nvim_create_autocmd("FileType", {
  group = group,
  pattern = "*",
  callback = function(args)
    if pcall(vim.treesitter.start, args.buf) then
      return
    end

    local lang = vim.treesitter.language.get_lang(vim.bo[args.buf].filetype)
    if lang then
      ensure_parser_installed(args.buf, lang)
    end
  end,
})

-- Recompute the file explorer width when the terminal is resized; nvim-tree
-- only sizes on open/toggle otherwise.
vim.api.nvim_create_autocmd("VimResized", {
  group = group,
  callback = function()
    if package.loaded["nvim-tree.api"] then
      require("nvim-tree.api").tree.resize()
    end
  end,
})

-- Restore the last cursor position when reopening a regular file.
vim.api.nvim_create_autocmd("BufReadPost", {
  group = group,
  callback = function(args)
    if vim.bo[args.buf].buftype ~= "" then
      return
    end

    local mark = vim.api.nvim_buf_get_mark(args.buf, '"')
    local line_count = vim.api.nvim_buf_line_count(args.buf)

    if mark[1] > 0 and mark[1] <= line_count then
      for _, winid in ipairs(vim.fn.win_findbuf(args.buf)) do
        if vim.api.nvim_win_is_valid(winid) then
          pcall(vim.api.nvim_win_set_cursor, winid, mark)
        end
      end
    end
  end,
})
