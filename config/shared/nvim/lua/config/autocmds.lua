local group = vim.api.nvim_create_augroup("UserGeneralAutocmds", { clear = true })

-- nvim-treesitter installs parsers; Neovim enables highlighting separately.
vim.treesitter.language.register("terraform", "tf")

-- Repeated FileType events can nest install() waits and prevent highlighting.
-- Track each in-flight language until installation finishes.
local pending_installs = {}

-- Suppresses a repeat notification only, never the retry: pending_installs
-- gates retries and is cleared regardless of outcome.
local notified_failures = {}

local function ensure_parser_installed(buf, lang)
  local ok, treesitter = pcall(require, "nvim-treesitter")
  if not ok then
    return
  end

  -- start() can fail for reasons unrelated to a missing parser (a broken
  -- query, say), so only the parser's presence on disk is the signal.
  if vim.list_contains(treesitter.get_installed("parsers"), lang) then
    return
  end
  if not vim.list_contains(treesitter.get_available(), lang) then
    return
  end

  -- Keyed by buffer, not a list: FileType repeats, and a list would queue
  -- and retry the same buffer twice.
  if pending_installs[lang] then
    pending_installs[lang][buf] = true
    return
  end
  pending_installs[lang] = { [buf] = true }

  -- A query-only partial install makes install() no-op; force is the only
  -- way past it. Safe here: the check above confirmed the parser is missing.
  treesitter.install(lang, { force = true }):await(function(_, installed)
    local buffers = pending_installs[lang]
    pending_installs[lang] = nil
    if not installed then
      if not notified_failures[lang] then
        notified_failures[lang] = true
        vim.notify(
          "Selfishell: Tree-sitter failed to install '"
            .. lang
            .. "'; highlighting won't be available until it succeeds. Will retry the next time a "
            .. lang
            .. " file is opened.",
          vim.log.levels.WARN
        )
      end
      return
    end

    for pending_buf in pairs(buffers) do
      -- The install is slow enough that the buffer may hold a different
      -- filetype by now; re-check, or a parser attaches to foreign content.
      local current_lang = vim.api.nvim_buf_is_valid(pending_buf)
        and vim.treesitter.language.get_lang(vim.bo[pending_buf].filetype)
      if current_lang == lang then
        pcall(vim.treesitter.start, pending_buf, lang)
      end
    end
  end)
end

vim.api.nvim_create_autocmd("FileType", {
  group = group,
  pattern = "*",
  callback = function(args)
    pcall(vim.treesitter.start, args.buf)

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

-- Highlight yanks that Vim's normal change reporting does not show.
vim.api.nvim_create_autocmd("TextYankPost", {
  group = group,
  callback = function()
    vim.hl.on_yank()
  end,
})

-- Restore the last cursor position when reopening a regular file, but not in
-- commit or rebase messages, xxd dumps, or diff mode (:h restore-cursor).
-- Filetype detection runs after init.lua's autocmds, so match it here.
vim.api.nvim_create_autocmd("BufReadPost", {
  group = group,
  callback = function(args)
    if vim.bo[args.buf].buftype ~= "" or vim.wo.diff then
      return
    end
    local filetype = vim.bo[args.buf].filetype
    if filetype == "" then
      filetype = vim.filetype.match({ buf = args.buf }) or ""
    end
    if filetype:find("commit", 1, true) or filetype == "gitrebase" or filetype == "xxd" then
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
