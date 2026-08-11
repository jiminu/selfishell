vim.treesitter.start = function() end

require("config.autocmds")
vim.cmd("enew")
vim.api.nvim_buf_set_lines(0, 0, -1, false, { "if [[ -n $value ]]; then", "  print ok", "fi" })
vim.bo.filetype = "zsh"
vim.api.nvim_exec_autocmds("FileType", { buffer = 0 })
-- The real callback defers its runtime! call with vim.schedule (see
-- config/autocmds.lua for why), so give the event loop a turn before
-- asserting on its effect.
vim.wait(100, function()
  return vim.b.current_syntax == "zsh"
end)

assert(vim.b.current_syntax == "zsh", "Neovim's built-in Zsh syntax was not loaded")
assert(vim.fn.synID(1, 1, true) ~= 0, "Zsh keyword has no syntax highlight group")
vim.bo.modified = false

print("Zsh syntax fallback: OK")
