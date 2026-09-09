local opt = vim.opt

-- Floating windows (diagnostics, hover, signature help, ...)
vim.o.winborder = "rounded"

-- UI
opt.number = true
opt.relativenumber = true
opt.hlsearch = true
opt.laststatus = 2
opt.ruler = false
-- lualine renders the mode in its first section; leaving showmode on
-- prints "-- INSERT --" on the command line right below it as well.
opt.showmode = false
opt.termguicolors = true
opt.signcolumn = "yes"
opt.cursorline = true
opt.wrap = false
opt.list = true
opt.listchars = { extends = "▸", precedes = "◂" }
opt.scrolloff = 4
opt.splitbelow = true
opt.splitright = true

-- Editing
opt.autoindent = true
opt.smartindent = true
opt.tabstop = 2
opt.softtabstop = 2
opt.shiftwidth = 2
opt.expandtab = true
opt.undofile = true
opt.confirm = true

-- Search
opt.ignorecase = true
opt.smartcase = true
opt.inccommand = "split"

-- Integration
opt.mouse = "a"
opt.clipboard = "unnamedplus"
opt.fileencodings = { "utf-8", "euc-kr" }

-- Completion menu behavior
opt.completeopt = { "menu", "menuone", "noselect" }
-- nvim-cmp reads this for its own menu and treats 0 as "no limit", which
-- makes the menu as tall as the number of candidates -- easily most of the
-- screen for an LSP source.
opt.pumheight = 10

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
