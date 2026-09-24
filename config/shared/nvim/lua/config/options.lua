local opt = vim.opt

vim.o.winborder = "rounded"

-- UI
opt.number = true
opt.relativenumber = true
opt.hlsearch = true
opt.ruler = false
-- lualine already displays the mode.
opt.showmode = false
opt.termguicolors = true
opt.signcolumn = "yes"
opt.cursorline = true
opt.wrap = false
opt.list = true
opt.listchars = { tab = "  ", nbsp = "␣", extends = "▸", precedes = "◂" }
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
opt.fileencodings = { "ucs-bom", "utf-8", "euc-kr" }

-- Over SSH, pbcopy or xclip would fill the remote clipboard, and WSL has no
-- provider; OSC 52 reaches the local terminal. Paste stays local: an OSC 52
-- read prompts or waits up to 10s.
if vim.env.SSH_TTY or vim.env.SSH_CONNECTION or vim.fn.has("wsl") == 1 then
  local osc52 = require("vim.ui.clipboard.osc52")
  local copied = { ["+"] = { {}, "v" }, ["*"] = { {}, "v" } }
  local function copy(register)
    local send = osc52.copy(register)
    return function(lines, regtype)
      copied[register] = { lines, regtype }
      send(lines)
    end
  end
  local function paste(register)
    return function()
      return copied[register]
    end
  end
  vim.g.clipboard = {
    name = "OSC 52 copy",
    copy = { ["+"] = copy("+"), ["*"] = copy("*") },
    paste = { ["+"] = paste("+"), ["*"] = paste("*") },
  }
end

-- Completion menu behavior
opt.completeopt = { "menu", "menuone", "noselect" }
-- Also caps nvim-cmp's menu; 0 leaves it unlimited.
opt.pumheight = 10

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
