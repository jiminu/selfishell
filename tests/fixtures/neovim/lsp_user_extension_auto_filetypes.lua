local languages = require("config.languages")

assert(vim.tbl_contains(languages.lsp, "fake_server"), "server without filetypes should still be merged")
assert(vim.tbl_contains(languages.lsp_filetypes, "fake_ft"), "filetypes should be auto-derived from nvim-lspconfig")

print("User extension auto filetypes: OK")
