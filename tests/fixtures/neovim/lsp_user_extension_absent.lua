local languages = require("config.languages")

assert(#languages.lsp == 4, "Default LSP list should be unaffected when no nvim.user.lua exists")
assert(
  vim.deep_equal(
    languages.lsp_filetypes,
    { "lua", "python", "sh", "bash", "javascript", "javascriptreact", "typescript", "typescriptreact" }
  ),
  "Default filetypes should be unaffected when no nvim.user.lua exists"
)

print("User extension absent: OK")
