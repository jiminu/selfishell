local notifications = {}
local original_notify = vim.notify
vim.notify = function(message, level)
  table.insert(notifications, { message = message, level = level })
end

local languages = require("config.languages")

vim.notify = original_notify

assert(#languages.lsp == 4, "a nvim.user.lua with no usable return value must not change the default lsp list")
assert(#notifications > 0, "a nvim.user.lua with no usable return value must raise a notification")
assert(notifications[1].level == vim.log.levels.ERROR, "a nvim.user.lua with no usable return value must notify at ERROR level")

print("User extension no return value: OK")
