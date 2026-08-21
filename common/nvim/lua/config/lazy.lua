local versions = require("config.plugin_versions")
local data_home = vim.env.XDG_DATA_HOME
if not data_home or data_home == "" then
  data_home = vim.fn.expand("~/.local/share")
end
local state_home = vim.env.XDG_STATE_HOME
if not state_home or state_home == "" then
  state_home = vim.fn.expand("~/.local/state")
end
local lazypath = data_home .. "/selfishell/nvim/lazy/lazy.nvim"

if not vim.uv.fs_stat(lazypath) then
  vim.notify(
    "lazy.nvim is missing. Run `selfishell install` to prepare Neovim plugins.",
    vim.log.levels.WARN
  )
  return
end

vim.opt.rtp:prepend(lazypath)

local function lazy_revision(path)
  -- Selfishell installs lazy.nvim at a detached commit, so the common path
  -- can verify HEAD without spawning Git. Keep Git as a compatibility fallback.
  local head_file = io.open(path .. "/.git/HEAD", "r")
  if head_file then
    local head = head_file:read("*l")
    head_file:close()
    if head and #head == 40 and head:match("^[0-9a-f]+$") then
      return head
    end
  end

  local revision = vim.fn.system({ "git", "-C", path, "rev-parse", "HEAD" }):gsub("%s+$", "")
  if vim.v.shell_error ~= 0 then
    return nil
  end
  return revision
end

local expected_revision = versions.revision("folke/lazy.nvim")
local installed_revision = lazy_revision(lazypath)
if installed_revision ~= expected_revision then
  vim.notify(
    "lazy.nvim revision does not match this Selfishell release. Run `selfishell update --tools-only`.",
    vim.log.levels.ERROR
  )
  return
end

require("lazy").setup({
  lockfile = state_home .. "/selfishell/nvim/lazy-lock.json",
  spec = {
    { import = "plugins" },
  },
  install = {
    colorscheme = { "vscode", "habamax" },
  },
  checker = {
    enabled = false,
  },
  change_detection = {
    notify = false,
  },
})
