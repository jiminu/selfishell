assert(vim.bo.filetype == "bigfile", "a large file opened through a symlink got filetype " .. vim.bo.filetype); print("Bigfile symlink smoke: OK")
