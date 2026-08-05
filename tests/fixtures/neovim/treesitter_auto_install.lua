-- Exercises config.autocmds' FileType-driven Tree-sitter parser auto-install:
-- nvim-treesitter 1.0+ dropped ensure_installed/auto_install from setup(), so
-- config.autocmds installs a missing parser itself the first time its
-- filetype is opened, instead of relying on a pre-populated static list.

--- @param name string
--- @param opts { start_fails_until: number, available: string[], installed: string[], install_succeeds: boolean?, filetype: string?, mock_treesitter: boolean? }
--- @return boolean exec_ok
--- @return number start_calls
--- @return string[] install_calls
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
        install = function(lang)
          table.insert(install_calls, lang)
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

  return exec_ok, start_calls, install_calls
end

-- A missing-but-known parser is installed, then highlighting is retried.
local exec_ok, start_calls, install_calls = run_scenario("install-and-retry", {
  start_fails_until = 1,
  available = { "widgetlang" },
  installed = {},
})
assert(exec_ok, "FileType autocmd raised an error during install-and-retry")
assert(
  #install_calls == 1 and install_calls[1] == "widgetlang",
  "Missing-but-available parser was not installed: " .. vim.inspect(install_calls)
)
assert(start_calls == 2, "Tree-sitter highlighting was not retried after installing the parser: " .. start_calls)

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

print("Tree-sitter auto-install: OK")
