vim.treesitter.start = function(buf)
  vim.g.selfishell_test_filetype = vim.bo[buf].filetype
end

require("config.autocmds")
vim.cmd("enew")
vim.bo.filetype = "sh"
vim.api.nvim_exec_autocmds("FileType", { buffer = 0 })

print(vim.g.selfishell_test_filetype)
print(vim.treesitter.language.get_lang("tf"))
