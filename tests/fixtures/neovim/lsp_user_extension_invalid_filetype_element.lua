local notifications = {}
local original_notify = vim.notify
vim.notify = function(message, level)
  table.insert(notifications, { message = message, level = level })
end

local languages = require("config.languages")

vim.notify = original_notify

assert(not vim.tbl_contains(languages.lsp, "clangd"), "a server with a non-string filetype element must not be merged at all")
assert(#notifications > 0, "a non-string filetype element must raise a notification")
assert(notifications[1].level == vim.log.levels.ERROR, "a non-string filetype element must notify at ERROR level")

print("User extension invalid filetype element: OK")
