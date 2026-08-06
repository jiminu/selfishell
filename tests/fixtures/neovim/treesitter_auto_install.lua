-- Exercises config.autocmds' FileType-driven Tree-sitter parser auto-install:
-- nvim-treesitter 1.0+ dropped ensure_installed/auto_install from setup(), so
-- config.autocmds installs a missing parser itself the first time its
-- filetype is opened, instead of relying on a pre-populated static list.

--- @param name string
--- @param opts { start_fails_until: number, available: string[], installed: string[], installed_queries: string[]?, install_succeeds: boolean?, install_produces_queries: boolean?, filetype: string?, mock_treesitter: boolean? }
--- @return boolean exec_ok
--- @return number start_calls
--- @return string[] install_calls
--- @return table<string, number> get_installed_calls count of get_installed() calls, keyed by type
local function run_scenario(name, opts)
  package.loaded["config.autocmds"] = nil
  package.loaded["nvim-treesitter"] = nil
  package.preload["nvim-treesitter"] = nil

  local start_calls = 0
  local original_start = vim.treesitter.start
  vim.treesitter.start = function(...)
    start_calls = start_calls + 1
    if start_calls <= opts.start_fails_until then
      error("simulated missing parser: " .. name)
    end
  end

  -- Real nvim-treesitter installs parsers and their highlight queries as
  -- two separate steps: a parser can be "installed" while its queries are
  -- not (interrupted install, etc.). The mock tracks them as two separate
  -- lists -- via a mutable `queries_installed` set that install() can grow --
  -- rather than folding both into one list, so scenarios can exercise that
  -- gap the same way a real corrupted install would.
  local install_calls = {}
  local get_installed_calls = { parsers = 0, queries = 0 }
  local queries_installed = {}
  for _, lang in ipairs(opts.installed_queries or opts.installed or {}) do
    queries_installed[lang] = true
  end

  if opts.mock_treesitter ~= false then
    package.preload["nvim-treesitter"] = function()
      return {
        get_installed = function(type)
          get_installed_calls[type] = (get_installed_calls[type] or 0) + 1
          if type == "queries" then
            return vim.tbl_keys(queries_installed)
          end
          return opts.installed or {}
        end,
        get_available = function()
          return opts.available or {}
        end,
        install = function(lang, install_opts)
          table.insert(install_calls, { lang = lang, force = (install_opts and install_opts.force) or false })

          -- Real nvim-treesitter's install_lang() silently no-ops (reports
          -- success without doing anything) when the language already
          -- satisfies its own parser-OR-queries "installed" check, unless
          -- force is passed -- that's exactly the trap force = true in the
          -- real code works around, so the mock has to reproduce it for the
          -- repair scenarios below to mean anything.
          local already_installed_by_ts = vim.list_contains(opts.installed or {}, lang) or queries_installed[lang] == true
          local will_run = (install_opts and install_opts.force) or not already_installed_by_ts

          if will_run and opts.install_succeeds ~= false and opts.install_produces_queries ~= false then
            queries_installed[lang] = true
          end

          return {
            await = function(_, callback)
              callback(nil, opts.install_succeeds ~= false)
            end,
          }
        end,
      }
    end
  end

  require("config.autocmds")
  vim.cmd("enew")
  -- Setting 'filetype' already fires the FileType autocmd; triggering it a
  -- second time via nvim_exec_autocmds would double-count start_calls below.
  local exec_ok = pcall(function()
    vim.bo.filetype = opts.filetype or "widgetlang"
  end)

  vim.treesitter.start = original_start
  package.preload["nvim-treesitter"] = nil
  package.loaded["nvim-treesitter"] = nil

  return exec_ok, start_calls, install_calls, get_installed_calls
end

-- A missing-but-known parser is installed, then highlighting is retried.
local exec_ok, start_calls, install_calls = run_scenario("install-and-retry", {
  start_fails_until = 1,
  available = { "widgetlang" },
  installed = {},
})
assert(exec_ok, "FileType autocmd raised an error during install-and-retry")
assert(
  #install_calls == 1 and install_calls[1].lang == "widgetlang",
  "Missing-but-available parser was not installed: " .. vim.inspect(install_calls)
)
-- 3 calls: the initial failing attempt, the direct retry after install, and
-- one more from the FileType autocmd re-firing (so other FileType-based,
-- Tree-sitter-dependent plugins like rainbow-delimiters get a chance too) --
-- which now finds a parser and returns without another install attempt.
assert(start_calls == 3, "Tree-sitter highlighting was not retried after installing the parser: " .. start_calls)

-- A freshly created LanguageTree isn't parsed yet -- that happens lazily.
-- Other FileType-triggered plugins (rainbow-delimiters, for real) query the
-- tree immediately when re-fired and silently get nothing back from an
-- empty one, with no redraw in a headless instance to ever trigger the real
-- parse afterwards -- reproduced directly against the real plugins, not
-- just inferred. The retry sequence must force a parse between starting the
-- parser and re-firing FileType, not just stop-then-start-then-refire.
do
  package.loaded["config.autocmds"] = nil
  package.loaded["nvim-treesitter"] = nil
  package.preload["nvim-treesitter"] = nil

  local sequence = {}
  local original_start = vim.treesitter.start
  vim.treesitter.start = function(buf, lang)
    if lang == nil then
      error("simulated missing parser")
    end
    table.insert(sequence, "start")
  end

  local original_stop = vim.treesitter.stop
  vim.treesitter.stop = function()
    table.insert(sequence, "stop")
  end

  local original_get_parser = vim.treesitter.get_parser
  vim.treesitter.get_parser = function(...)
    table.insert(sequence, "get_parser")
    return {
      parse = function()
        table.insert(sequence, "parse")
      end,
    }
  end

  local original_exec_autocmds = vim.api.nvim_exec_autocmds
  vim.api.nvim_exec_autocmds = function(event, opts)
    if event == "FileType" then
      table.insert(sequence, "refire")
    end
    return original_exec_autocmds(event, opts)
  end

  package.preload["nvim-treesitter"] = function()
    return {
      get_installed = function()
        return {}
      end,
      get_available = function()
        return { "widgetlang" }
      end,
      install = function()
        return {
          await = function(_, callback)
            callback(nil, true)
          end,
        }
      end,
    }
  end

  require("config.autocmds")

  local buf = vim.api.nvim_create_buf(true, false)
  vim.api.nvim_set_current_buf(buf)
  vim.bo.filetype = "widgetlang"

  assert(
    vim.deep_equal(sequence, { "stop", "start", "get_parser", "parse", "refire" }),
    "the retry sequence must stop, start, force a parse, then re-fire FileType, in that order: "
      .. vim.inspect(sequence)
  )

  vim.treesitter.start = original_start
  vim.treesitter.stop = original_stop
  vim.treesitter.get_parser = original_get_parser
  vim.api.nvim_exec_autocmds = original_exec_autocmds
  package.preload["nvim-treesitter"] = nil
  package.loaded["nvim-treesitter"] = nil
end

-- A parser can be installed while its highlight queries are not (an install
-- interrupted between the two steps -- Neovim killed mid-install, etc.).
-- vim.treesitter.start only needs the parser, so it "succeeds" immediately
-- here (start_fails_until = 0), the way it does for real against a
-- parser-only install -- this must still trigger a real reinstall to pick up
-- the missing queries rather than treating the language as done.
local exec_ok_partial, _, install_calls_partial = run_scenario("parser-installed-queries-missing", {
  start_fails_until = 0,
  available = { "widgetlang" },
  installed = { "widgetlang" },
  installed_queries = {},
})
assert(exec_ok_partial, "FileType autocmd raised an error when queries were missing for an installed parser")
assert(
  #install_calls_partial == 1 and install_calls_partial[1].lang == "widgetlang",
  "A parser installed without its queries was not reinstalled: " .. vim.inspect(install_calls_partial)
)
assert(
  install_calls_partial[1].force == true,
  "install() was not called with force = true, so nvim-treesitter's own "
    .. "installed-parser-OR-queries check would have silently no-op'd it: "
    .. vim.inspect(install_calls_partial)
)

-- The buffer's initial (failing-to-highlight) vim.treesitter.start() already
-- asked the real, process-wide vim.treesitter.query.get for "highlights" and
-- it cached the nil it got back -- that memoization has no idea a repair
-- just wrote real query files to disk, so without busting it explicitly,
-- the language would look permanently queryless for the rest of the running
-- Neovim session even though get_installed("queries") now reports it fine.
do
  local original_clear = vim.treesitter.query.get.clear
  local clear_calls = 0
  vim.treesitter.query.get.clear = function(...)
    clear_calls = clear_calls + 1
    return original_clear(...)
  end

  local exec_ok_clears_cache = run_scenario("parser-installed-queries-missing-clears-cache", {
    start_fails_until = 0,
    available = { "widgetlang" },
    installed = { "widgetlang" },
    installed_queries = {},
  })

  vim.treesitter.query.get.clear = original_clear

  assert(exec_ok_clears_cache, "FileType autocmd raised an error while clearing the query cache after repair")
  assert(
    clear_calls > 0,
    "A repaired language's query cache was never cleared -- highlighting would stay broken for the "
      .. "rest of the session even though the queries are now installed on disk"
  )
end

-- A language whose install() never produces queries (nvim-treesitter simply
-- ships none for it) must be attempted once, not retried on every open --
-- otherwise every file of that filetype pays for a redundant install() call.
do
  local exec_ok_queryless, _, install_calls_first, installed_calls_first = run_scenario("queryless-language", {
    start_fails_until = 1,
    available = { "widgetlang" },
    installed = {},
    install_produces_queries = false,
  })
  assert(exec_ok_queryless, "FileType autocmd raised an error for a queryless language")
  assert(
    #install_calls_first == 1,
    "A queryless language was not installed on first open: " .. vim.inspect(install_calls_first)
  )
  assert(installed_calls_first.queries and installed_calls_first.queries > 0, "queries were never checked at all")
end

-- Once a language is confirmed fully ready (parser + queries, or parser +
-- confirmed-queryless), opening more buffers of the same filetype must not
-- re-scan the parser/query install directories -- that scan is the whole
-- cost this caching exists to avoid paying on every file open.
do
  package.loaded["config.autocmds"] = nil
  package.loaded["nvim-treesitter"] = nil
  package.preload["nvim-treesitter"] = nil

  local get_installed_calls = { parsers = 0, queries = 0 }
  package.preload["nvim-treesitter"] = function()
    return {
      get_installed = function(type)
        get_installed_calls[type] = (get_installed_calls[type] or 0) + 1
        return { "widgetlang" }
      end,
      get_available = function()
        return { "widgetlang" }
      end,
      install = function()
        error("install() must not be called for an already fully-installed language")
      end,
    }
  end

  require("config.autocmds")

  vim.cmd("enew")
  vim.bo.filetype = "widgetlang"
  local calls_after_first_open = vim.deepcopy(get_installed_calls)

  vim.cmd("enew")
  vim.bo.filetype = "widgetlang"
  vim.cmd("enew")
  vim.bo.filetype = "widgetlang"

  assert(
    vim.deep_equal(get_installed_calls, calls_after_first_open),
    "Repeat opens of an already-ready language re-scanned install state: "
      .. vim.inspect(calls_after_first_open)
      .. " -> "
      .. vim.inspect(get_installed_calls)
  )

  package.preload["nvim-treesitter"] = nil
  package.loaded["nvim-treesitter"] = nil
end

-- A manual :TSUninstall (or :TSUpdate/:TSInstall) fires User TSUpdate; the
-- known-good cache must not outlive that, or a language removed out from
-- under it would keep being reported ready forever.
do
  package.loaded["config.autocmds"] = nil
  package.loaded["nvim-treesitter"] = nil
  package.preload["nvim-treesitter"] = nil

  local original_start = vim.treesitter.start
  vim.treesitter.start = function()
    error("simulated missing parser")
  end

  local installed = { "widgetlang" }
  local install_calls = {}
  package.preload["nvim-treesitter"] = function()
    return {
      get_installed = function()
        return installed
      end,
      get_available = function()
        return { "widgetlang" }
      end,
      install = function(lang, install_opts)
        table.insert(install_calls, lang)
        if install_opts and install_opts.force then
          installed = { "widgetlang" }
        end
        return {
          await = function(_, callback)
            callback(nil, true)
          end,
        }
      end,
    }
  end

  require("config.autocmds")

  vim.cmd("enew")
  vim.bo.filetype = "widgetlang" -- caches widgetlang as known-good
  assert(#install_calls == 0, "widgetlang should have started fully installed")

  -- Simulate ":TSUninstall widgetlang" happening out from under the cache.
  installed = {}
  vim.api.nvim_exec_autocmds("User", { pattern = "TSUpdate" })

  vim.cmd("enew")
  vim.bo.filetype = "widgetlang"
  assert(
    #install_calls == 1 and install_calls[1] == "widgetlang",
    "User TSUpdate did not invalidate the known-good cache for an uninstalled language: "
      .. vim.inspect(install_calls)
  )

  vim.treesitter.start = original_start
  package.preload["nvim-treesitter"] = nil
  package.loaded["nvim-treesitter"] = nil
end

-- install() can fail outright (network down, no compiler, disk full --
-- anything try_install_lang() surfaces as an error). That must not get
-- cached as ready (known_good_langs/queryless_langs must stay untouched)
-- and must not leave pending_installs stuck thinking one is still in
-- flight, or every later open of that filetype would silently do nothing
-- forever instead of retrying. It must also tell the user once -- silently
-- broken highlighting with no explanation is its own bug -- but not spam a
-- notification on every subsequent failed retry of the same language.
do
  package.loaded["config.autocmds"] = nil
  package.loaded["nvim-treesitter"] = nil
  package.preload["nvim-treesitter"] = nil

  local original_start = vim.treesitter.start
  vim.treesitter.start = function()
    error("simulated missing parser")
  end

  local notify_calls = {}
  local original_notify = vim.notify
  vim.notify = function(msg, level)
    table.insert(notify_calls, { msg = msg, level = level })
  end

  local install_calls = {}
  package.preload["nvim-treesitter"] = function()
    return {
      get_installed = function()
        return {}
      end,
      get_available = function()
        return { "widgetlang" }
      end,
      install = function(lang)
        table.insert(install_calls, lang)
        return {
          await = function(_, callback)
            callback(nil, false) -- the install genuinely failed
          end,
        }
      end,
    }
  end

  require("config.autocmds")

  vim.cmd("enew")
  vim.bo.filetype = "widgetlang"
  assert(#install_calls == 1, "a failed install should still have been attempted once")
  assert(#notify_calls == 1, "a failed install should notify the user once: " .. vim.inspect(notify_calls))
  assert(
    notify_calls[1].msg:find("widgetlang", 1, true) ~= nil,
    "the failure notification should name the affected language: " .. vim.inspect(notify_calls[1])
  )

  vim.cmd("enew")
  vim.bo.filetype = "widgetlang"
  assert(
    #install_calls == 2,
    "a failed install must be retried on the next open, not treated as ready or stuck in flight: "
      .. vim.inspect(install_calls)
  )
  assert(
    #notify_calls == 1,
    "a repeat failure of the same language must not notify again this session: " .. vim.inspect(notify_calls)
  )

  vim.notify = original_notify
  vim.treesitter.start = original_start
  package.preload["nvim-treesitter"] = nil
  package.loaded["nvim-treesitter"] = nil
end

-- install() runs asynchronously and can take a while (network + compile).
-- The buffer it was requested for can move on before it resolves -- loaded
-- a different file, had its filetype changed, or (as observed directly:
-- :enew frees an unmodified buffer's number and the very next :enew reuses
-- it) become a completely different buffer that happens to share the same
-- number. Only nvim_buf_is_valid() was checked before; a buffer that's
-- valid but now a different language must not get the stale language's
-- parser started on it, and must not have FileType wrongly re-fired either.
do
  package.loaded["config.autocmds"] = nil
  package.loaded["nvim-treesitter"] = nil
  package.preload["nvim-treesitter"] = nil

  local original_start = vim.treesitter.start
  local start_calls = {}
  vim.treesitter.start = function(buf, lang)
    if lang == nil then
      error("simulated missing parser")
    end
    table.insert(start_calls, { buf = buf, lang = lang })
  end

  local refire_count = 0
  local original_exec_autocmds = vim.api.nvim_exec_autocmds
  vim.api.nvim_exec_autocmds = function(event, opts)
    if event == "FileType" then
      refire_count = refire_count + 1
    end
    return original_exec_autocmds(event, opts)
  end

  local pending_callback
  package.preload["nvim-treesitter"] = function()
    return {
      get_installed = function()
        return {}
      end,
      get_available = function()
        return { "widgetlang" }
      end,
      install = function()
        return {
          await = function(_, callback)
            pending_callback = callback
          end,
        }
      end,
    }
  end

  require("config.autocmds")

  local buf = vim.api.nvim_create_buf(true, false)
  vim.api.nvim_set_current_buf(buf)
  vim.bo.filetype = "widgetlang" -- kicks off the (still in-flight) install
  assert(pending_callback, "install() was never invoked")

  -- The buffer moves on to a different language before the install resolves.
  vim.bo[buf].filetype = "otherlang"

  pending_callback(nil, true)

  assert(
    #start_calls == 0,
    "a stale language's parser must not be started on a buffer that changed language: " .. vim.inspect(start_calls)
  )
  assert(
    refire_count == 0,
    "FileType must not be re-fired for a buffer that no longer matches the installed language"
  )

  vim.api.nvim_exec_autocmds = original_exec_autocmds
  vim.treesitter.start = original_start
  package.preload["nvim-treesitter"] = nil
  package.loaded["nvim-treesitter"] = nil
end

-- A parser and its queries can both be "installed" by directory-existence
-- (nvim-treesitter's own bookkeeping) while the query content doesn't parse
-- against the installed parser -- a stale parser paired with a newer query
-- file, observed for real with the `diff` grammar ("Invalid node type
-- change"). vim.treesitter.query.get throws in that case; a directory scan
-- alone can't see it. Use the real "lua" language (bundled with Neovim, so
-- a real parser is always loadable) with an explicit broken query to
-- reproduce the exact throw, rather than mocking query.get itself.
do
  package.loaded["config.autocmds"] = nil
  package.loaded["nvim-treesitter"] = nil
  package.preload["nvim-treesitter"] = nil

  vim.treesitter.query.set("lua", "highlights", "(nonexistent_node_type_xyz) @foo")

  local install_calls = {}
  package.preload["nvim-treesitter"] = function()
    return {
      get_installed = function()
        return { "lua" }
      end,
      get_available = function()
        return { "lua" }
      end,
      install = function(lang, install_opts)
        table.insert(install_calls, lang)
        if install_opts and install_opts.force then
          -- Simulate a real reinstall recompiling a matching parser/query pair.
          vim.treesitter.query.set("lua", "highlights", "(comment) @comment")
        end
        return {
          await = function(_, callback)
            callback(nil, true)
          end,
        }
      end,
    }
  end

  require("config.autocmds")

  vim.cmd("enew")
  vim.bo.filetype = "lua"
  assert(
    #install_calls == 1,
    "a parser/query version mismatch (both 'installed' but the query throws) was not repaired: "
      .. vim.inspect(install_calls)
  )
  assert(pcall(vim.treesitter.query.get, "lua", "highlights"), "the repair should have produced a parseable query")

  vim.cmd("enew")
  vim.bo.filetype = "lua"
  assert(
    #install_calls == 1,
    "a language repaired once should not be reinstalled again on the next open: " .. vim.inspect(install_calls)
  )

  vim.treesitter.query.set("lua", "highlights", nil) -- restore the real bundled query
  package.preload["nvim-treesitter"] = nil
  package.loaded["nvim-treesitter"] = nil
end

-- Same version-mismatch setup, but this time reinstalling doesn't fix it
-- (a persistent, not transient, grammar/query mismatch). One real attempt
-- is all it should get -- retrying on every future open of a language
-- whose queries will never validate would hammer the network/compiler for
-- nothing, forever.
do
  package.loaded["config.autocmds"] = nil
  package.loaded["nvim-treesitter"] = nil
  package.preload["nvim-treesitter"] = nil

  vim.treesitter.query.set("lua", "highlights", "(nonexistent_node_type_xyz) @foo")

  local install_calls = {}
  package.preload["nvim-treesitter"] = function()
    return {
      get_installed = function()
        return { "lua" }
      end,
      get_available = function()
        return { "lua" }
      end,
      install = function(lang)
        table.insert(install_calls, lang) -- reinstalling does NOT fix the mismatch this time
        return {
          await = function(_, callback)
            callback(nil, true)
          end,
        }
      end,
    }
  end

  require("config.autocmds")

  vim.cmd("enew")
  vim.bo.filetype = "lua"
  assert(#install_calls == 1, "an unrepairable version mismatch should still get one real attempt")

  vim.cmd("enew")
  vim.bo.filetype = "lua"
  assert(
    #install_calls == 1,
    "an unrepairable version mismatch must not be retried on every future open: " .. vim.inspect(install_calls)
  )

  vim.treesitter.query.set("lua", "highlights", nil) -- restore the real bundled query
  package.preload["nvim-treesitter"] = nil
  package.loaded["nvim-treesitter"] = nil
end

-- A language nvim-treesitter has no parser for at all must never be installed.
local exec_ok_unavailable, _, install_calls_unavailable = run_scenario("unavailable-language", {
  start_fails_until = math.huge,
  available = {},
  installed = {},
})
assert(exec_ok_unavailable, "FileType autocmd raised an error for an unavailable language")
assert(
  #install_calls_unavailable == 0,
  "Installed a parser nvim-treesitter does not know about: " .. vim.inspect(install_calls_unavailable)
)

-- A parser already marked installed that still fails to start (e.g. a
-- corrupted local install) must not be re-installed on every buffer open.
local exec_ok_installed, _, install_calls_already_installed = run_scenario("already-installed-but-broken", {
  start_fails_until = math.huge,
  available = { "widgetlang" },
  installed = { "widgetlang" },
})
assert(exec_ok_installed, "FileType autocmd raised an error for an already-installed parser")
assert(
  #install_calls_already_installed == 0,
  "Re-attempted installing an already-installed parser: " .. vim.inspect(install_calls_already_installed)
)

-- When nvim-treesitter itself isn't on the runtimepath at all, the autocmd
-- must degrade silently instead of erroring on every unrecognized filetype.
local exec_ok_absent = run_scenario("plugin-absent", {
  start_fails_until = math.huge,
  mock_treesitter = false,
})
assert(exec_ok_absent, "FileType autocmd raised an error when nvim-treesitter was unavailable")

-- Neovim fires FileType more than once for the same buffer during startup
-- (observed when opening a file from the command line), and separate
-- buffers can independently request the same not-yet-installed language.
-- Either way, install() must only be called once per language while a
-- request is in flight, and every waiting buffer must still get retried
-- once it resolves.
do
  package.loaded["config.autocmds"] = nil
  package.loaded["nvim-treesitter"] = nil
  package.preload["nvim-treesitter"] = nil

  local retried_bufs = {}
  local original_start = vim.treesitter.start
  vim.treesitter.start = function(buf, lang)
    if lang == nil then
      error("simulated missing parser")
    end
    table.insert(retried_bufs, buf)
  end

  local install_calls = {}
  local pending_callback
  package.preload["nvim-treesitter"] = function()
    return {
      get_installed = function()
        return {}
      end,
      get_available = function()
        return { "widgetlang" }
      end,
      install = function(lang, install_opts)
        table.insert(install_calls, { lang = lang, force = (install_opts and install_opts.force) or false })
        return {
          await = function(_, callback)
            pending_callback = callback
          end,
        }
      end,
    }
  end

  require("config.autocmds")

  -- vim.api.nvim_create_buf, not :enew: leaving an unmodified :enew buffer
  -- for another lets Neovim free and immediately reuse its number, which
  -- would silently collapse buf_a and buf_b into the same buffer and defeat
  -- the point of this scenario (two genuinely distinct buffers).
  local buf_a = vim.api.nvim_create_buf(true, false)
  vim.api.nvim_set_current_buf(buf_a)
  vim.bo.filetype = "widgetlang" -- first FileType fire for buf_a
  vim.api.nvim_exec_autocmds("FileType", { buffer = buf_a }) -- simulates Neovim's observed second fire

  local buf_b = vim.api.nvim_create_buf(true, false)
  vim.api.nvim_set_current_buf(buf_b)
  vim.bo.filetype = "widgetlang" -- a second buffer requesting the same in-flight language

  assert(
    #install_calls == 1,
    "install() was called more than once for a language already in flight: " .. vim.inspect(install_calls)
  )
  assert(pending_callback, "install() was never invoked for the concurrent-request scenario")
  assert(install_calls[1].force == true, "install() was not called with force = true: " .. vim.inspect(install_calls))

  pending_callback(nil, true)

  table.sort(retried_bufs)
  -- buf_a was queued twice (once per FileType fire) but must be retried
  -- exactly once: pending_installs tracks buffers as a set, not a list, so
  -- the same buffer firing FileType twice while an install is in flight
  -- doesn't cause a redundant double stop/start/FileType-refire on it.
  local expected = { buf_a, buf_b }
  table.sort(expected)
  assert(
    vim.deep_equal(retried_bufs, expected),
    "Every waiting buffer must be retried exactly once: " .. vim.inspect(retried_bufs)
  )

  vim.treesitter.start = original_start
  package.preload["nvim-treesitter"] = nil
  package.loaded["nvim-treesitter"] = nil
end

print("Tree-sitter auto-install: OK")
