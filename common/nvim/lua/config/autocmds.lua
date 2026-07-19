local group = vim.api.nvim_create_augroup("UserGeneralAutocmds", { clear = true })

-- Neovim 0.12 uses the built-in Tree-sitter highlighter. The current
-- nvim-treesitter plugin no longer enables it through setup()/opts.
vim.treesitter.language.register("terraform", "tf")

vim.api.nvim_create_autocmd("FileType", {
  group = group,
  pattern = "*",
  callback = function(args)
    pcall(vim.treesitter.start, args.buf)
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
      pcall(vim.api.nvim_win_set_cursor, 0, mark)
    end
  end,
})
