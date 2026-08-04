local M = {
  lsp = {
    "lua_ls@3.18.2",
    "pyright@1.1.411",
    "bashls@5.6.0",
    "ts_ls@5.3.0",
  },
  -- Union of the filetypes supported by the configured LSP servers.
  lsp_filetypes = {
    "lua",
    "python",
    "sh",
    "bash",
    "javascript",
    "javascriptreact",
    "typescript",
    "typescriptreact",
  },
}

local function user_extension_path()
  local config_home = vim.env.XDG_CONFIG_HOME
  if not config_home or config_home == "" then
    config_home = vim.fn.expand("~/.config")
  end
  return config_home .. "/selfishell/nvim.user.lua"
end

-- Stats, loads, and executes a Lua file, without raising: returns its result
-- on success, or (nil, an error message) if it doesn't exist, fails to
-- parse, or errors when run.
local function dofile_safe(path)
  if not vim.uv.fs_stat(path) then
    return nil, nil
  end

  local chunk, load_error = loadfile(path)
  if not chunk then
    return nil, load_error
  end

  local ok, result = pcall(chunk)
  if not ok then
    return nil, result
  end

  return result, nil
end

local function load_user_extension()
  local result, err = dofile_safe(user_extension_path())
  if not result then
    if err then
      vim.notify("Selfishell: nvim.user.lua failed to load: " .. tostring(err), vim.log.levels.ERROR)
    end
    return nil
  end

  if type(result) ~= "table" or (result.servers ~= nil and type(result.servers) ~= "table") then
    vim.notify("Selfishell: nvim.user.lua must return a table with an optional `servers` table", vim.log.levels.ERROR)
    return nil
  end

  return result
end

local function lsp_base_name(entry)
  return entry:match("^([^@]+)")
end

local function is_already_managed(name)
  local base = lsp_base_name(name)
  for _, existing in ipairs(M.lsp) do
    if lsp_base_name(existing) == base then
      return true
    end
  end
  return false
end

-- nvim-lspconfig ships a default `filetypes` list for every server it knows
-- about, in a `lsp/<name>.lua` file under its own plugin directory. Reading
-- that file directly needs neither lazy.nvim nor the plugin to be loaded --
-- selfishell install already clones it to a fixed, well-known path.
local function nvim_lspconfig_filetypes(name)
  local path = vim.fn.stdpath("data") .. "/lazy/nvim-lspconfig/lsp/" .. name .. ".lua"
  local config = dofile_safe(path)
  if type(config) ~= "table" or type(config.filetypes) ~= "table" then
    return nil
  end
  return config.filetypes
end

local function filetypes_are_valid(filetypes)
  for _, filetype in ipairs(filetypes) do
    if type(filetype) ~= "string" then
      return false
    end
  end
  return true
end

local function apply_user_server(name, spec)
  if type(name) ~= "string" or type(spec) ~= "table" then
    vim.notify(
      "Selfishell: nvim.user.lua server '" .. tostring(name) .. "' is invalid; skipping",
      vim.log.levels.ERROR
    )
    return
  end

  if spec.filetypes ~= nil and (type(spec.filetypes) ~= "table" or not filetypes_are_valid(spec.filetypes)) then
    vim.notify(
      "Selfishell: nvim.user.lua server '" .. name .. "' has an invalid `filetypes` list; skipping",
      vim.log.levels.ERROR
    )
    return
  end

  if is_already_managed(name) then
    vim.notify(
      "Selfishell: '" .. name .. "' is already managed by Selfishell's defaults; ignoring the nvim.user.lua entry",
      vim.log.levels.WARN
    )
    return
  end

  local filetypes = spec.filetypes or nvim_lspconfig_filetypes(name)
  if not filetypes then
    vim.notify(
      "Selfishell: nvim.user.lua server '"
        .. name
        .. "' has no `filetypes` and none could be found for it in nvim-lspconfig; skipping",
      vim.log.levels.ERROR
    )
    return
  end

  table.insert(M.lsp, name)
  for _, filetype in ipairs(filetypes) do
    table.insert(M.lsp_filetypes, filetype)
  end
end

local function apply_user_extension()
  local extension = load_user_extension()
  if not extension or not extension.servers then
    return
  end

  for name, spec in pairs(extension.servers) do
    apply_user_server(name, spec)
  end
end

apply_user_extension()

return M
