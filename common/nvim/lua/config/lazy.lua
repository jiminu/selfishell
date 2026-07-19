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

local expected_revision = versions.revision("folke/lazy.nvim")
local installed_revision = vim.fn.system({ "git", "-C", lazypath, "rev-parse", "HEAD" }):gsub("%s+$", "")
if vim.v.shell_error ~= 0 or installed_revision ~= expected_revision then
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
