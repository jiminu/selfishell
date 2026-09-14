-- Parser presence, not start() success, determines whether installation is needed.

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

-- An already-installed parser must never be reinstalled, even when start()
-- keeps failing on it: repairing that is a manual :TSUpdate/:TSInstall! job.
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

-- FileType fires more than once per buffer at startup, and separate buffers
-- can request the same missing language. Either way install() must run once
-- per in-flight language, and every waiting buffer must still get started.
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

  -- nvim_create_buf, not :enew: Neovim frees an unmodified :enew buffer and
  -- reuses its number, collapsing buf_a and buf_b into one.
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

-- install() is slow enough that its buffer can move on first: a different
-- file, a changed filetype, or a wholly different buffer reusing the freed
-- number. A valid buffer now on another language must not get the stale one.
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

-- A failing install() must not crash Neovim, must not leave pending_installs
-- stuck in flight (every later open would then do nothing instead of
-- retrying), and must notify once rather than on every failed retry.
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
