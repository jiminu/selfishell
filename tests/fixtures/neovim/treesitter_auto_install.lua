-- Parser presence, not start() success, determines whether installation is needed.

-- Reloads config.autocmds against a mock nvim-treesitter (none when opts.absent).
-- start() without a language fails like a missing parser; each install() is
-- resolved by opts.await, else immediately with opts.install_succeeds.
local function scenario(opts)
  package.loaded["config.autocmds"] = nil
  package.loaded["nvim-treesitter"] = nil
  package.preload["nvim-treesitter"] = nil
  local s = { started = {}, installs = {}, notified = {} }
  local original_start, original_notify = vim.treesitter.start, vim.notify
  vim.treesitter.start = function(buf, lang)
    if lang == nil then
      error("simulated missing parser")
    end
    table.insert(s.started, { buf = buf, lang = lang })
  end
  vim.notify = function(msg, level)
    table.insert(s.notified, { msg = msg, level = level })
  end
  if not opts.absent then
    package.preload["nvim-treesitter"] = function()
      return {
        get_installed = function()
          return opts.installed or {}
        end,
        get_available = function()
          return opts.available or { "widgetlang" }
        end,
        install = function(lang, install_opts)
          table.insert(s.installs, { lang = lang, force = (install_opts and install_opts.force) or false })
          return {
            await = function(_, callback)
              if opts.await then
                opts.await(callback)
              else
                callback(nil, opts.install_succeeds ~= false)
              end
            end,
          }
        end,
      }
    end
  end
  require("config.autocmds")
  function s.open(filetype)
    vim.cmd("enew")
    return pcall(function()
      vim.bo.filetype = filetype
    end)
  end
  function s.restore()
    vim.treesitter.start, vim.notify = original_start, original_notify
    package.preload["nvim-treesitter"] = nil
    package.loaded["nvim-treesitter"] = nil
  end
  return s
end

-- A missing-but-known parser is installed.
do
  local s = scenario({})
  assert(s.open("widgetlang"), "FileType autocmd raised an error when the parser was missing")
  assert(
    #s.installs == 1 and s.installs[1].lang == "widgetlang" and s.installs[1].force == true,
    "A missing-but-available parser was not installed: " .. vim.inspect(s.installs)
  )
  s.restore()
end

-- An already-installed parser must never be reinstalled, even when start()
-- keeps failing on it: repairing that is a manual :TSUpdate/:TSInstall! job.
do
  local s = scenario({ installed = { "widgetlang" } })
  assert(s.open("widgetlang"), "FileType autocmd raised an error for an already-installed parser")
  assert(
    #s.installs == 0,
    "An already-installed parser must not be reinstalled just because vim.treesitter.start() failed: "
      .. vim.inspect(s.installs)
  )
  s.restore()
end

-- A language nvim-treesitter has no parser for at all must never be installed.
do
  local s = scenario({ available = {} })
  assert(s.open("widgetlang"), "FileType autocmd raised an error for an unavailable language")
  assert(#s.installs == 0, "Installed a parser nvim-treesitter does not know about: " .. vim.inspect(s.installs))
  s.restore()
end

-- When nvim-treesitter itself isn't on the runtimepath at all, the autocmd
-- must degrade silently instead of erroring on every unrecognized filetype.
do
  local s = scenario({ absent = true })
  assert(s.open("widgetlang"), "FileType autocmd raised an error when nvim-treesitter was unavailable")
  s.restore()
end

-- FileType fires more than once per buffer at startup, and separate buffers
-- can request the same missing language. Either way install() must run once
-- per in-flight language, and every waiting buffer must still get started.
do
  local pending
  local s = scenario({
    await = function(callback)
      pending = callback
    end,
  })

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
    #s.installs == 1,
    "install() was called more than once for a language already in flight: " .. vim.inspect(s.installs)
  )
  assert(pending, "install() was never invoked for the concurrent-request scenario")
  assert(s.installs[1].force == true, "install() was not called with force = true: " .. vim.inspect(s.installs))

  pending(nil, true)

  assert(
    #s.started == 2,
    "Every waiting buffer must have Tree-sitter started on it once the install resolves: " .. vim.inspect(s.started)
  )
  for _, entry in ipairs(s.started) do
    assert(entry.lang == "widgetlang", "a pending buffer was started with the wrong language: " .. vim.inspect(entry))
  end
  local started_bufs = { s.started[1].buf, s.started[2].buf }
  table.sort(started_bufs)
  local expected = { buf_a, buf_b }
  table.sort(expected)
  assert(
    vim.deep_equal(started_bufs, expected),
    "Every waiting buffer must be started exactly once: " .. vim.inspect(started_bufs)
  )
  s.restore()
end

-- install() is slow enough that its buffer can move on first: a different
-- file, a changed filetype, or a wholly different buffer reusing the freed
-- number. A valid buffer now on another language must not get the stale one.
do
  local pending
  local s = scenario({
    await = function(callback)
      pending = callback
    end,
  })

  local buf = vim.api.nvim_create_buf(true, false)
  vim.api.nvim_set_current_buf(buf)
  vim.bo.filetype = "widgetlang" -- kicks off the (still in-flight) install
  assert(pending, "install() was never invoked")

  -- The buffer moves on to a different language before the install resolves.
  vim.bo[buf].filetype = "otherlang"

  pending(nil, true)

  assert(
    #s.started == 0,
    "a stale language's parser must not be started on a buffer that changed language: " .. vim.inspect(s.started)
  )
  s.restore()
end

-- A buffer can close entirely before its pending install resolves. That
-- must be ignored safely, not error the callback or start Tree-sitter on a
-- buffer number that may already have been reused for something else.
do
  local pending
  local s = scenario({
    await = function(callback)
      pending = callback
    end,
  })

  local buf = vim.api.nvim_create_buf(true, false)
  vim.api.nvim_set_current_buf(buf)
  vim.bo.filetype = "widgetlang" -- kicks off the (still in-flight) install
  assert(pending, "install() was never invoked")

  -- The buffer closes before the install resolves.
  vim.api.nvim_set_current_buf(vim.api.nvim_create_buf(true, false))
  vim.api.nvim_buf_delete(buf, { force = true })

  local resolve_ok = pcall(pending, nil, true)

  assert(resolve_ok, "install completion must not error when its buffer was already closed")
  assert(#s.started == 0, "a closed buffer must not have Tree-sitter started on it: " .. vim.inspect(s.started))
  s.restore()
end

-- A failing install() must not crash Neovim, must not leave pending_installs
-- stuck in flight (every later open would then do nothing instead of
-- retrying), and must notify once rather than on every failed retry.
do
  local s = scenario({ install_succeeds = false })

  assert(s.open("widgetlang"), "a failed install must not crash the FileType autocmd")
  assert(#s.installs == 1, "a failed install should still have been attempted once")
  assert(#s.notified == 1, "a failed install should notify the user once: " .. vim.inspect(s.notified))
  assert(
    s.notified[1].msg:find("widgetlang", 1, true) ~= nil,
    "the failure notification should name the affected language: " .. vim.inspect(s.notified[1])
  )
  assert(
    s.notified[1].msg:find("highlighting won't be available", 1, true) ~= nil,
    "a language without a bundled parser should report missing highlighting: " .. vim.inspect(s.notified[1])
  )

  s.open("widgetlang")
  assert(
    #s.installs == 2,
    "a failed install must be retried on the next open, not treated as ready or stuck in flight: "
      .. vim.inspect(s.installs)
  )
  assert(
    #s.notified == 1,
    "a repeat failure of the same language must not notify again this session: " .. vim.inspect(s.notified)
  )
  s.restore()
end

-- Neovim bundles the Lua parser, so a failed install must not claim that
-- highlighting is unavailable.
do
  local s = scenario({ available = { "lua" }, install_succeeds = false })
  s.open("lua")
  assert(#s.notified == 1, "a failed bundled-language install should notify once: " .. vim.inspect(s.notified))
  assert(
    s.notified[1].msg:find("bundled parser stays in use", 1, true) ~= nil,
    "a bundled parser failure should say highlighting continues: " .. vim.inspect(s.notified[1])
  )
  s.restore()
end

print("Tree-sitter auto-install: OK")
