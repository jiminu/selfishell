local languages = require("config.languages")

assert(vim.tbl_contains(languages.lsp, "clangd"), "clangd from nvim.user.lua should be merged into lsp")
assert(vim.tbl_contains(languages.lsp, "lua_ls@3.18.2"), "default servers must remain present")
assert(vim.tbl_contains(languages.lsp_filetypes, "c"), "c filetype from nvim.user.lua should be merged")
assert(vim.tbl_contains(languages.lsp_filetypes, "cpp"), "cpp filetype from nvim.user.lua should be merged")
assert(vim.tbl_contains(languages.lsp_filetypes, "lua"), "default filetypes must remain present")

print("User extension merge: OK")
