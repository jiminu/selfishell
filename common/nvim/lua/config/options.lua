local opt = vim.opt

-- UI
opt.number = true
opt.relativenumber = true
opt.hlsearch = true
opt.laststatus = 2
opt.ruler = false
opt.termguicolors = true
opt.signcolumn = "yes"

-- Editing
opt.autoindent = true
opt.smartindent = true
opt.tabstop = 2
opt.softtabstop = 2
opt.shiftwidth = 2
opt.expandtab = true
opt.undofile = true

-- Search
opt.ignorecase = true
opt.smartcase = true

-- Integration
opt.mouse = "a"
opt.clipboard = "unnamedplus"
opt.fileencodings = { "utf-8", "euc-kr" }

-- Completion menu behavior
opt.completeopt = { "menu", "menuone", "noselect" }

-- Diagnostic display
vim.diagnostic.config({
  virtual_text = {
    prefix = "●",
    spacing = 4,
  },
  signs = true,
  underline = true,
  update_in_insert = false,
  severity_sort = true,
})
