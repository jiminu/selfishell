local notifications = {}
local original_notify = vim.notify
vim.notify = function(message, level)
  table.insert(notifications, { message = message, level = level })
end

local languages = require("config.languages")

vim.notify = original_notify

assert(not vim.tbl_contains(languages.lsp, "unknown_server"), "a server with no filetypes anywhere must not be merged")
assert(#notifications > 0, "missing filetypes with no auto-derivation match must raise a notification")
assert(notifications[1].level == vim.log.levels.ERROR, "missing filetypes must notify at ERROR level")

print("User extension no filetypes found: OK")
