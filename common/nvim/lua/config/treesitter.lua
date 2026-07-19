local M = {}

function M.install(languages)
  local treesitter = require("nvim-treesitter")
  local installed_ok = treesitter.install(languages, { summary = true }):wait(300000)

  assert(installed_ok, "One or more Tree-sitter parsers failed to install")

  local installed = {}
  for _, language in ipairs(treesitter.get_installed("parsers")) do
    installed[language] = true
  end

  local missing = {}
  for _, language in ipairs(languages) do
    if not installed[language] then
      table.insert(missing, language)
    end
  end

  assert(#missing == 0, "Tree-sitter parsers are missing after installation: " .. table.concat(missing, ", "))
end

return M
