local languages = require("config.languages")

assert(vim.tbl_contains(languages.lsp, "needs_require"), "a server whose nvim-lspconfig file requires a sibling module should still merge")
assert(vim.tbl_contains(languages.lsp_filetypes, "needs_require_ft"), "filetypes from a require-using nvim-lspconfig file should still be auto-derived")

print("User extension lspconfig require: OK")
