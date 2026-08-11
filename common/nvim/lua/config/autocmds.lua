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

-- Languages whose install() has already failed once and been reported to
-- the user this session. Only suppresses the notification, not the retry:
-- pending_installs is what actually gates a retry, and it's always cleared
-- regardless of outcome, so the next file open of the same language tries
-- again even though this stays set.
local notified_failures = {}

local function ensure_parser_installed(buf, lang)
  local ok, treesitter = pcall(require, "nvim-treesitter")
  if not ok then
    return
  end

  -- vim.treesitter.start() succeeding or failing is not the signal here --
  -- it can fail for reasons unrelated to a missing parser (e.g. a broken
  -- query), and it's not this function's job to repair that. Whether the
  -- parser itself is actually on disk is the only thing that matters.
  if vim.list_contains(treesitter.get_installed("parsers"), lang) then
    return
  end
  if not vim.list_contains(treesitter.get_available(), lang) then
    return
  end

  -- A set keyed by buffer, not a list: Neovim fires FileType for the same
  -- buffer more than once (see above), and without deduping here that
  -- buffer would get queued twice and be retried twice below.
  if pending_installs[lang] then
    pending_installs[lang][buf] = true
    return
  end
  pending_installs[lang] = { [buf] = true }

  -- A query-only partial install (query dir present, parser file missing)
  -- would make nvim-treesitter's own install() treat the language as
  -- already done and no-op. force = true is the only way through that
  -- public API to make sure the parser actually gets installed here; this
  -- is safe because this call is only reached when get_installed("parsers")
  -- above already confirmed the parser itself is missing.
  treesitter.install(lang, { force = true }):await(function(_, installed)
    local buffers = pending_installs[lang]
    pending_installs[lang] = nil
    if not installed then
      -- Network down, no compiler, disk full, an nvim-treesitter internal
      -- error -- whatever it was, tell the user once per language per
      -- session instead of leaving highlighting silently missing with no
      -- explanation. pending_installs is already cleared above, so the
      -- next time any buffer of this language opens, a fresh attempt runs.
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
      -- The install ran asynchronously and could take a while (network +
      -- compile); the buffer may have loaded a different file (or a
      -- different filetype in the same buffer) by the time it resolves.
      -- Confirm it's still the language this install was for before
      -- touching it, or a stale parser could get attached to content it
      -- doesn't belong to.
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
    -- The Zsh parser currently ships without highlight queries, so attaching
    -- it here would still leave the buffer uncolored -- but attaching still
    -- has a side effect: the highlighter constructor resets 'syntax' to "" on
    -- every buffer it attaches to, which blocks Neovim's own built-in
    -- synload autocmd from loading syntax/zsh.vim's colors on this and any
    -- later FileType event. Skipping the attach for zsh avoids that, and
    -- lets Neovim's built-in mechanism color it normally.
    if vim.bo[args.buf].filetype == "zsh" then
      return
    end

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
