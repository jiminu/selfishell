local M = {
  lsp = {
    "lua_ls@3.18.2",
    "pyright@1.1.411",
    "bashls@5.6.0",
  },
  -- Union of the filetypes supported by the configured LSP servers.
  lsp_filetypes = {
    "lua",
    "python",
    "sh",
    "bash",
  },
}

-- Servers that need an external prerequisite Selfishell does not manage.
-- Checked only when the user opts into that server via nvim.user.lua.
local EXECUTABLE_REQUIREMENTS = {
  jdtls = "java",
}

local function user_extension_path()
  local config_home = vim.env.XDG_CONFIG_HOME
  if not config_home or config_home == "" then
    config_home = vim.fn.expand("~/.config")
  end
  return config_home .. "/selfishell/nvim.user.lua"
end

local function load_user_extension()
  local path = user_extension_path()
  if not vim.uv.fs_stat(path) then
    return nil
  end

  local chunk, load_error = loadfile(path)
  if not chunk then
    vim.notify("Selfishell: nvim.user.lua failed to load: " .. tostring(load_error), vim.log.levels.ERROR)
    return nil
  end

  local ok, result = pcall(chunk)
  if not ok then
    vim.notify("Selfishell: nvim.user.lua raised an error: " .. tostring(result), vim.log.levels.ERROR)
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

local function apply_user_extension()
  local extension = load_user_extension()
  if not extension or not extension.servers then
    return
  end

  for name, spec in pairs(extension.servers) do
    if type(name) ~= "string" or type(spec) ~= "table" or type(spec.filetypes) ~= "table" then
      vim.notify(
        "Selfishell: nvim.user.lua server '" .. tostring(name) .. "' must set a `filetypes` list; skipping",
        vim.log.levels.ERROR
      )
    elseif is_already_managed(name) then
      vim.notify(
        "Selfishell: '" .. name .. "' is already managed by Selfishell's defaults; ignoring the nvim.user.lua entry",
        vim.log.levels.WARN
      )
    else
      table.insert(M.lsp, name)
      for _, filetype in ipairs(spec.filetypes) do
        if type(filetype) == "string" then
          table.insert(M.lsp_filetypes, filetype)
        end
      end

      local required_executable = EXECUTABLE_REQUIREMENTS[name]
      if required_executable and vim.fn.executable(required_executable) == 0 then
        vim.notify(
          "Selfishell: '"
            .. name
            .. "' requires '"
            .. required_executable
            .. "' on PATH. Install it (for example: `mise use -g java@21`) before opening a matching buffer.",
          vim.log.levels.WARN
        )
      end
    end
  end
end

apply_user_extension()

return M
