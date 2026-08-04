local notifications = {}
local original_notify = vim.notify
vim.notify = function(message, level)
  table.insert(notifications, { message = message, level = level })
end

local languages = require("config.languages")

vim.notify = original_notify

assert(#languages.lsp == 4, "Malformed nvim.user.lua must not change the default lsp list")
assert(
  vim.deep_equal(
    languages.lsp_filetypes,
    { "lua", "python", "sh", "bash", "javascript", "javascriptreact", "typescript", "typescriptreact" }
  ),
  "Malformed nvim.user.lua must not change the default filetypes"
)
assert(#notifications > 0, "Malformed nvim.user.lua must raise a notification")
assert(notifications[1].level == vim.log.levels.ERROR, "Malformed nvim.user.lua must notify at ERROR level")

print("User extension malformed handling: OK")
