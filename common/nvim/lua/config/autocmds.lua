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

-- nvim-treesitter installs a parser and its highlight queries as two
-- separate steps of one install() call. `vim.treesitter.start` only needs
-- the parser, so it happily "succeeds" with the queries step never having
-- run (interrupted install, killed Neovim, etc.), silently leaving a buffer
-- with a working parser but no highlighting at all. So "installed" has to
-- mean parser *and* queries, not just the parser nvim-treesitter's own
-- get_installed("parsers") reports.
--
-- Checking both is two directory scans instead of one, which only matters
-- if paid on every FileType. It isn't: once a language is confirmed ready
-- it's cached in known_good_langs for the rest of the session, so repeat
-- opens of an already-working filetype (the overwhelmingly common case) cost
-- a table lookup, same as before this change. Languages that ship no
-- queries at all are cached in queryless_langs after one real install
-- attempt confirms that, so they don't get reinstalled on every open either.
local known_good_langs = {}
local queryless_langs = {}

-- nvim-treesitter fires User TSUpdate at the start of install(), update(),
-- and uninstall() alike. A manual :TSUninstall on a language we'd already
-- cached as good would otherwise never be re-checked -- reset both caches
-- on that event so they can't outlive what's actually on disk.
vim.api.nvim_create_autocmd("User", {
  group = group,
  pattern = "TSUpdate",
  callback = function()
    known_good_langs = {}
    queryless_langs = {}
  end,
})

local function is_ready(treesitter, lang)
  if known_good_langs[lang] then
    return true
  end
  if not vim.list_contains(treesitter.get_installed("parsers"), lang) then
    return false
  end
  if queryless_langs[lang] or vim.list_contains(treesitter.get_installed("queries"), lang) then
    known_good_langs[lang] = true
    return true
  end
  return false
end

local function ensure_parser_installed(buf, lang)
  local ok, treesitter = pcall(require, "nvim-treesitter")
  if not ok then
    return
  end

  if is_ready(treesitter, lang) then
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

  -- nvim-treesitter's own install() treats a language as done and no-ops
  -- if *either* its parser or its queries are already present (it doesn't
  -- know we specifically need queries too) -- so a plain install() call here
  -- would silently do nothing for exactly the parser-without-queries case
  -- this function exists to repair. force = true is the only way through
  -- that public API to make it actually redo the work.
  treesitter.install(lang, { force = true }):await(function(_, installed)
    local buffers = pending_installs[lang]
    pending_installs[lang] = nil
    if not installed then
      return
    end

    if vim.list_contains(treesitter.get_installed("queries"), lang) then
      known_good_langs[lang] = true
    else
      queryless_langs[lang] = true
      known_good_langs[lang] = true
    end

    -- The buffer's earlier vim.treesitter.start() (parser present, queries
    -- missing) already had a Highlighter query the *global* query cache for
    -- "highlights" and got nil back; that nil is memoized, so re-reading the
    -- query files we just installed would otherwise keep returning the same
    -- stale nil for the rest of this Neovim session even though they now
    -- exist on disk. Clear the whole cache (cheap, and this path only runs
    -- once per broken language) rather than guessing which query types were
    -- queried and cached before the repair.
    pcall(function()
      vim.treesitter.query.get:clear()
    end)

    for _, pending_buf in ipairs(buffers) do
      if vim.api.nvim_buf_is_valid(pending_buf) then
        -- A buffer whose parser was already present (only queries were
        -- missing) got a working vim.treesitter.start() earlier in the
        -- FileType callback, before this repair ran. start() never tears
        -- down a prior highlighter for the same buffer -- it just points
        -- the buffer at a second one -- so stop() first or the old instance
        -- leaks, still attached to the parse tree's callbacks forever.
        pcall(vim.treesitter.stop, pending_buf)
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
    local start_ok = pcall(vim.treesitter.start, args.buf)
    local lang = vim.treesitter.language.get_lang(vim.bo[args.buf].filetype)

    if start_ok then
      if not lang then
        return
      end
      local ok, treesitter = pcall(require, "nvim-treesitter")
      if not ok or is_ready(treesitter, lang) then
        return
      end
    end

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
