local group = vim.api.nvim_create_augroup("UserGeneralAutocmds", { clear = true })

-- Neovim 0.12 uses the built-in Tree-sitter highlighter. The current
-- nvim-treesitter plugin no longer enables it through setup()/opts.
vim.treesitter.language.register("terraform", "tf")

-- nvim-treesitter 1.0+ also dropped ensure_installed/auto_install from
-- setup(), so install a missing parser the first time its filetype is
-- opened rather than maintaining a separate static list to bulk-install
-- ahead of time.
--
-- Neovim fires FileType for a buffer more than once during startup (e.g.
-- opening a file from the command line), so track in-flight installs per
-- language ourselves: calling nvim-treesitter's install() a second time
-- before the first finishes routes through its own concurrent-install
-- guard, which blocks on a *nested* vim.wait() -- timing-sensitive enough
-- that it was observed to leave a buffer's highlighter never started.
local pending_installs = {}

local function ensure_parser_installed(buf, lang)
  local ok, treesitter = pcall(require, "nvim-treesitter")
  if not ok then
    return
  end

  if vim.list_contains(treesitter.get_installed("parsers"), lang) then
    return
  end
  if not vim.list_contains(treesitter.get_available(), lang) then
    return
  end

  if pending_installs[lang] then
    table.insert(pending_installs[lang], buf)
    return
  end
  pending_installs[lang] = { buf }

  treesitter.install(lang):await(function(_, installed)
    local buffers = pending_installs[lang]
    pending_installs[lang] = nil
    if not installed then
      return
    end

    for _, pending_buf in ipairs(buffers) do
      if vim.api.nvim_buf_is_valid(pending_buf) then
        pcall(vim.treesitter.start, pending_buf, lang)
        -- Other plugins (e.g. rainbow-delimiters) attach on their own
        -- FileType autocmd and assume a parser is already available there;
        -- re-fire it now that one exists so they get a chance to attach too.
        pcall(vim.api.nvim_exec_autocmds, "FileType", { buffer = pending_buf })
      end
    end
  end)
end

vim.api.nvim_create_autocmd("FileType", {
  group = group,
  pattern = "*",
  callback = function(args)
    if pcall(vim.treesitter.start, args.buf) then
      return
    end

    local lang = vim.treesitter.language.get_lang(vim.bo[args.buf].filetype)
    if lang then
      ensure_parser_installed(args.buf, lang)
    end
  end,
})

-- Recompute the file explorer width when the terminal is resized; nvim-tree
-- only sizes on open/toggle otherwise.
vim.api.nvim_create_autocmd("VimResized", {
  group = group,
  callback = function()
    if package.loaded["nvim-tree.api"] then
      require("nvim-tree.api").tree.resize()
    end
  end,
})

-- Restore the last cursor position when reopening a regular file.
vim.api.nvim_create_autocmd("BufReadPost", {
  group = group,
  callback = function(args)
    if vim.bo[args.buf].buftype ~= "" then
      return
    end

    local mark = vim.api.nvim_buf_get_mark(args.buf, '"')
    local line_count = vim.api.nvim_buf_line_count(args.buf)

    if mark[1] > 0 and mark[1] <= line_count then
      for _, winid in ipairs(vim.fn.win_findbuf(args.buf)) do
        if vim.api.nvim_win_is_valid(winid) then
          pcall(vim.api.nvim_win_set_cursor, winid, mark)
        end
      end
    end
  end,
})
