local function run(install_result, installed)
  package.loaded["nvim-treesitter"] = nil
  package.preload["nvim-treesitter"] = function()
    return {
      install = function()
        return { wait = function() return install_result end }
      end,
      get_installed = function() return installed end,
    }
  end
  package.loaded["config.treesitter"] = nil
  return pcall(require("config.treesitter").install, { "lua", "python" })
end

local ok, message = run(false, {})
assert(
  not ok and tostring(message):find("failed to install", 1, true),
  "false install result was accepted: " .. tostring(message)
)

ok, message = run(true, { "lua" })
assert(
  not ok and tostring(message):find("python", 1, true),
  "missing parser was accepted: " .. tostring(message)
)

ok, message = run(true, { "lua", "python" })
assert(ok, tostring(message))
print("Tree-sitter install verification: OK")
