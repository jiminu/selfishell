local languages = require("config.languages")
local target

for _, spec in ipairs(require("plugins.lsp")) do
  if spec[1] == "mason-org/mason-lspconfig.nvim" then
    target = spec
  end
end

assert(target, "mason-lspconfig spec is missing")
assert(vim.deep_equal(target.ft, languages.lsp_filetypes), "LSP filetypes are not centralized")
for _, filetype in ipairs({
  "lua",
  "python",
  "sh",
  "bash",
  "javascript",
  "javascriptreact",
  "typescript",
  "typescriptreact",
}) do
  assert(vim.tbl_contains(target.ft, filetype), "supported LSP filetype is missing: " .. filetype)
end
assert(not vim.tbl_contains(target.ft, "terraform"), "Terraform should not load unsupported LSP plugins")
assert(not vim.tbl_contains(target.ft, "zsh"), "Zsh should not load unsupported LSP plugins")

print("LSP filetype scope: OK")
