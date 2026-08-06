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
-- the user this session. Deliberately NOT cleared on TSUpdate: nvim-
-- treesitter's own install() fires User TSUpdate synchronously at its own
-- start (reload_parsers()), before the async work even begins -- including
-- on the retry's own install() call. Clearing this here would wipe the
-- just-set record before that same retry's failure is even known, and the
-- next failure would notify again, defeating "once per session" entirely.
local notified_failures = {}

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

-- Languages where a real install() completed (parser and queries both
-- exist on disk) but the queries still fail to parse against the installed
-- parser -- a persistent grammar/query mismatch a reinstall didn't fix.
-- Kept separate from queryless_langs/known_good_langs so this broken state
-- is never silently reported as fine (it must not be treated as "ready" or
-- as "genuinely has no queries"), and separate from notified_failures
-- (install() itself succeeded here, unlike that case) so each gets its own
-- once-per-session notification. Unlike notified_failures, this one *is*
-- cleared on TSUpdate: ensure_parser_installed short-circuits on
-- repair_failed_langs before ever calling install() again, so -- unlike
-- the install-failure case -- clearing it here can't be wiped out by a
-- retry's own install() call; it only opens the door to a recheck the next
-- time that language's file is opened, e.g. after a manual :TSUpdate.
local repair_failed_langs = {}
local notified_repair_failures = {}

-- nvim-treesitter fires User TSUpdate at the start of install(), update(),
-- and uninstall() alike. A manual :TSUninstall on a language we'd already
-- cached as good would otherwise never be re-checked -- reset those caches
-- on that event so they can't outlive what's actually on disk.
vim.api.nvim_create_autocmd("User", {
  group = group,
  pattern = "TSUpdate",
  callback = function()
    known_good_langs = {}
    queryless_langs = {}
    repair_failed_langs = {}
    notified_repair_failures = {}
  end,
})

-- pcall's success/failure, not its return value, is what matters here:
-- vim.treesitter.query.get returns nil (no error) for a language that
-- genuinely ships no highlights query, which is fine -- but it *throws* if
-- a highlights.scm exists and references a node type the installed parser
-- doesn't have. That happens for real: a parser binary can be stale
-- relative to the query file symlinked straight from nvim-treesitter's own
-- checkout (observed with the `diff` grammar). A directory-existence check
-- alone can't see that; only actually asking for the query can.
local function has_parseable_queries(lang)
  return pcall(vim.treesitter.query.get, lang, "highlights")
end

local function is_ready(treesitter, lang)
  if known_good_langs[lang] then
    return true
  end
  if not vim.list_contains(treesitter.get_installed("parsers"), lang) then
    return false
  end
  if queryless_langs[lang] then
    known_good_langs[lang] = true
    return true
  end
  if not vim.list_contains(treesitter.get_installed("queries"), lang) then
    return false
  end
  if not has_parseable_queries(lang) then
    return false
  end
  known_good_langs[lang] = true
  return true
end

local function ensure_parser_installed(buf, lang)
  local ok, treesitter = pcall(require, "nvim-treesitter")
  if not ok then
    return
  end

  -- Give up retrying a language whose queries are known not to validate
  -- even after a real reinstall -- only TSUpdate (see above) reopens this,
  -- not another failed vim.treesitter.start() on the next file open.
  if repair_failed_langs[lang] then
    return
  end
  if is_ready(treesitter, lang) then
    return
  end
  if not vim.list_contains(treesitter.get_available(), lang) then
    return
  end

  -- A set keyed by buffer, not a list: Neovim fires FileType for the same
  -- buffer more than once (see above), and without deduping here that
  -- buffer would get queued twice and be retried twice below -- a harmless
  -- but wasteful double stop/start/FileType-refire on install completion.
  if pending_installs[lang] then
    pending_installs[lang][buf] = true
    return
  end
  pending_installs[lang] = { [buf] = true }

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

    -- The buffer's earlier vim.treesitter.start() (parser present, queries
    -- missing or unparseable) already had a Highlighter query the *global*
    -- query cache for "highlights" and got a result cached; re-reading the
    -- query files we just (re)installed would otherwise keep returning that
    -- same stale result for the rest of this Neovim session even though
    -- they've changed on disk. Clear the whole cache (cheap, and this path
    -- only runs once per broken language) before checking, rather than
    -- guessing which query types were queried and cached before the repair.
    pcall(function()
      vim.treesitter.query.get:clear()
    end)

    local queries_installed = vim.list_contains(treesitter.get_installed("queries"), lang)

    if not queries_installed then
      -- nvim-treesitter genuinely ships no highlights query for this
      -- language. Nothing more to try, and nothing wrong to report.
      queryless_langs[lang] = true
      known_good_langs[lang] = true
    elseif has_parseable_queries(lang) then
      -- A real success: install() completed and the queries it produced
      -- actually parse against the parser it just (re)installed.
      known_good_langs[lang] = true
    else
      -- Queries exist but still don't parse against the parser we just
      -- (re)installed -- a persistent grammar/query mismatch, not the
      -- "ships no queries at all" case, and not fixed by reinstalling.
      -- Leaving this queryless_langs/known_good_langs, as before, silently
      -- reported the language as fine forever: no highlighting, no retry,
      -- no warning, treated internally like nothing was ever wrong.
      -- Instead: it stays unable to start (vim.treesitter.start keeps
      -- throwing, honestly reflecting the broken state), isn't retried on
      -- every future open (repair_failed_langs short-circuits that
      -- above), and is reported once so the user knows and can act.
      repair_failed_langs[lang] = true
      if not notified_repair_failures[lang] then
        notified_repair_failures[lang] = true
        vim.notify(
          "Selfishell: Tree-sitter queries for '"
            .. lang
            .. "' remain incompatible after reinstall. Run :TSUpdate "
            .. lang
            .. " and check :messages.",
          vim.log.levels.WARN
        )
      end
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
        -- A buffer whose parser was already present (only queries were
        -- missing) got a working vim.treesitter.start() earlier in the
        -- FileType callback, before this repair ran. start() never tears
        -- down a prior highlighter for the same buffer -- it just points
        -- the buffer at a second one -- so stop() first or the old instance
        -- leaks, still attached to the parse tree's callbacks forever.
        pcall(vim.treesitter.stop, pending_buf)
        pcall(vim.treesitter.start, pending_buf, lang)
        -- A freshly created LanguageTree isn't parsed yet -- that happens
        -- lazily. Other plugins that attach on FileType and immediately
        -- query the tree (rainbow-delimiters does) get an empty one and
        -- silently produce nothing, with no redraw in a headless instance
        -- to ever trigger the real parse afterwards. Force it up front so
        -- whoever attaches next, below, sees a fully parsed tree.
        pcall(function()
          vim.treesitter.get_parser(pending_buf, lang):parse()
        end)
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
