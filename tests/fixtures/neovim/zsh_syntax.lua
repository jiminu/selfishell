-- $VIMRUNTIME/syntax/syntax.vim installs the FileType -> 'syntax' bridge
-- (in a "syntaxset" augroup) that actually loads a filetype's syntax/*.vim
-- colors, and it deliberately skips buffers with b:ts_highlight set so
-- Tree-sitter and legacy syntax highlighting don't double up. A real
-- session gets this from a colorscheme plugin calling `:syntax enable`
-- well before any Zsh buffer is opened; do the same here.
vim.cmd("syntax enable")

require("config.autocmds")
vim.cmd("enew")
vim.api.nvim_buf_set_lines(0, 0, -1, false, { "if [[ -n $value ]]; then", "  print ok", "fi" })
vim.bo.filetype = "zsh"
vim.api.nvim_exec_autocmds("FileType", { buffer = 0 })

assert(
  vim.b.ts_highlight == nil,
  "Tree-sitter should not attach to Zsh buffers: syntax.vim skips loading legacy colors when it is"
)
assert(vim.b.current_syntax == "zsh", "Neovim's built-in Zsh syntax was not loaded")
assert(vim.fn.synID(1, 1, true) ~= 0, "Zsh keyword has no syntax highlight group")
vim.bo.modified = false

print("Zsh syntax fallback: OK")
