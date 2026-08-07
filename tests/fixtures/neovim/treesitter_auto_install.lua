-- Exercises config.autocmds' FileType-driven Tree-sitter parser auto-install:
-- nvim-treesitter 1.0+ dropped ensure_installed/auto_install from setup(), so
-- config.autocmds installs a missing parser itself the first time its
-- filetype is opened, instead of relying on a pre-populated static list.
--
-- Whether a parser is installed is judged solely by nvim-treesitter's own
-- get_installed("parsers"), never by vim.treesitter.start() succeeding or
-- failing -- start() can fail for reasons (e.g. a broken query) that have
-- nothing to do with a missing parser, and repairing that is out of scope.

--- @param name string
--- @param opts { start_fails: boolean?, available: string[], installed: string[], install_succeeds: boolean?, filetype: string?, mock_treesitter: boolean? }
--- @return boolean exec_ok
--- @return number start_calls
--- @return table[] install_calls
local function run_scenario(name, opts)
  package.loaded["config.autocmds"] = nil
  package.loaded["nvim-treesitter"] = nil
  package.preload["nvim-treesitter"] = nil

  local start_calls = 0
  local original_start = vim.treesitter.start
  vim.treesitter.start = function(...)
    start_calls = start_calls + 1
    if opts.start_fails then
      error("simulated missing parser: " .. name)
    end
  end

  local install_calls = {}
  if opts.mock_treesitter ~= false then
    package.preload["nvim-treesitter"] = function()
      return {
        get_installed = function()
          return opts.installed or {}
        end,
        get_available = function()
          return opts.available or {}
        end,
        install = function(lang, install_opts)
          table.insert(install_calls, { lang = lang, force = (install_opts and install_opts.force) or false })
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
  local exec_ok = pcall(function()
    vim.bo.filetype = opts.filetype or "widgetlang"
  end)

  vim.treesitter.start = original_start
  package.preload["nvim-treesitter"] = nil
  package.loaded["nvim-treesitter"] = nil

  return exec_ok, start_calls, install_calls
end

-- A missing-but-known parser is installed.
do
  local exec_ok, _, install_calls = run_scenario("missing-parser-installs", {
    start_fails = true,
    available = { "widgetlang" },
    installed = {},
  })
  assert(exec_ok, "FileType autocmd raised an error when the parser was missing")
  assert(
    #install_calls == 1 and install_calls[1].lang == "widgetlang" and install_calls[1].force == true,
    "A missing-but-available parser was not installed: " .. vim.inspect(install_calls)
  )
end

-- A parser nvim-treesitter already reports installed must never be
-- reinstalled -- not even when vim.treesitter.start() keeps failing on it
-- (e.g. a corrupted local install or a broken query). Repairing that is a
-- manual :TSUpdate/:TSInstall! job, not this autocmd's.
do
  local exec_ok, _, install_calls = run_scenario("already-installed-not-reinstalled", {
    start_fails = true,
    available = { "widgetlang" },
    installed = { "widgetlang" },
  })
  assert(exec_ok, "FileType autocmd raised an error for an already-installed parser")
  assert(
    #install_calls == 0,
    "An already-installed parser must not be reinstalled just because vim.treesitter.start() failed: "
      .. vim.inspect(install_calls)
  )
end

-- A language nvim-treesitter has no parser for at all must never be installed.
do
  local exec_ok, _, install_calls = run_scenario("unavailable-language-never-installed", {
    start_fails = true,
    available = {},
    installed = {},
  })
  assert(exec_ok, "FileType autocmd raised an error for an unavailable language")
  assert(
    #install_calls == 0,
    "Installed a parser nvim-treesitter does not know about: " .. vim.inspect(install_calls)
  )
end

-- When nvim-treesitter itself isn't on the runtimepath at all, the autocmd
-- must degrade silently instead of erroring on every unrecognized filetype.
do
  local exec_ok = run_scenario("plugin-absent-degrades-silently", {
    start_fails = true,
    mock_treesitter = false,
  })
  assert(exec_ok, "FileType autocmd raised an error when nvim-treesitter was unavailable")
end

-- Neovim fires FileType more than once for the same buffer during startup
-- (observed when opening a file from the command line), and separate
-- buffers can independently request the same not-yet-installed language.
-- Either way, install() must only be called once per language while a
-- request is in flight, and every waiting buffer must still get Tree-sitter
-- started on it once the install resolves.
do
  package.loaded["config.autocmds"] = nil
  package.loaded["nvim-treesitter"] = nil
  package.preload["nvim-treesitter"] = nil

  local retried = {}
  local original_start = vim.treesitter.start
  vim.treesitter.start = function(buf, lang)
    if lang == nil then
      error("simulated missing parser")
    end
    table.insert(retried, { buf = buf, lang = lang })
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

  assert(
    #retried == 2,
    "Every waiting buffer must have Tree-sitter started on it once the install resolves: " .. vim.inspect(retried)
  )
  for _, entry in ipairs(retried) do
    assert(entry.lang == "widgetlang", "a pending buffer was started with the wrong language: " .. vim.inspect(entry))
  end
  local retried_bufs = { retried[1].buf, retried[2].buf }
  table.sort(retried_bufs)
  local expected = { buf_a, buf_b }
  table.sort(expected)
  assert(
    vim.deep_equal(retried_bufs, expected),
    "Every waiting buffer must be started exactly once: " .. vim.inspect(retried_bufs)
  )

  vim.treesitter.start = original_start
  package.preload["nvim-treesitter"] = nil
  package.loaded["nvim-treesitter"] = nil
end

-- install() runs asynchronously and can take a while (network + compile).
-- The buffer it was requested for can move on before it resolves -- loaded
-- a different file, had its filetype changed, or (as observed directly:
-- :enew frees an unmodified buffer's number and the very next :enew reuses
-- it) become a completely different buffer that happens to share the same
-- number. A buffer that's valid but now a different language must not get
-- the stale language's parser started on it.
do
  package.loaded["config.autocmds"] = nil
  package.loaded["nvim-treesitter"] = nil
  package.preload["nvim-treesitter"] = nil

  local start_calls = {}
  local original_start = vim.treesitter.start
  vim.treesitter.start = function(buf, lang)
    if lang == nil then
      error("simulated missing parser")
    end
    table.insert(start_calls, { buf = buf, lang = lang })
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

  vim.treesitter.start = original_start
  package.preload["nvim-treesitter"] = nil
  package.loaded["nvim-treesitter"] = nil
end

-- A buffer can close entirely before its pending install resolves. That
-- must be ignored safely, not error the callback or start Tree-sitter on a
-- buffer number that may already have been reused for something else.
do
  package.loaded["config.autocmds"] = nil
  package.loaded["nvim-treesitter"] = nil
  package.preload["nvim-treesitter"] = nil

  local start_calls = {}
  local original_start = vim.treesitter.start
  vim.treesitter.start = function(buf, lang)
    if lang == nil then
      error("simulated missing parser")
    end
    table.insert(start_calls, { buf = buf, lang = lang })
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

  -- The buffer closes before the install resolves.
  vim.api.nvim_set_current_buf(vim.api.nvim_create_buf(true, false))
  vim.api.nvim_buf_delete(buf, { force = true })

  local resolve_ok = pcall(pending_callback, nil, true)

  assert(resolve_ok, "install completion must not error when its buffer was already closed")
  assert(#start_calls == 0, "a closed buffer must not have Tree-sitter started on it: " .. vim.inspect(start_calls))

  vim.treesitter.start = original_start
  package.preload["nvim-treesitter"] = nil
  package.loaded["nvim-treesitter"] = nil
end

-- install() can fail outright (network down, no compiler, disk full --
-- anything nvim-treesitter surfaces as an error). That must not crash
-- Neovim, must not leave pending_installs stuck thinking one is still in
-- flight (or every later open of that filetype would silently do nothing
-- forever instead of retrying), and must tell the user once -- but not spam
-- a notification on every subsequent failed retry of the same language.
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

  local exec_ok = pcall(function()
    vim.cmd("enew")
    vim.bo.filetype = "widgetlang"
  end)
  assert(exec_ok, "a failed install must not crash the FileType autocmd")
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

print("Tree-sitter auto-install: OK")
