local notifications = {}
local original_notify = vim.notify
vim.notify = function(message, level)
  table.insert(notifications, { message = message, level = level })
end

local languages = require("config.languages")

vim.notify = original_notify

assert(#languages.lsp == 4, "servers with an invalid name must not be merged")
assert(#notifications >= 2, "each invalid server name must raise its own notification")
for _, notification in ipairs(notifications) do
  assert(notification.level == vim.log.levels.ERROR, "an invalid server name must notify at ERROR level")
end

print("User extension invalid name: OK")
