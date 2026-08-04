local original_executable = vim.fn.executable
vim.fn.executable = function(name)
  if name == "java" then
    return 0
  end
  return original_executable(name)
end

local notifications = {}
local original_notify = vim.notify
vim.notify = function(message, level)
  table.insert(notifications, { message = message, level = level })
end

local languages = require("config.languages")

vim.fn.executable = original_executable
vim.notify = original_notify

assert(vim.tbl_contains(languages.lsp, "jdtls"), "jdtls should still be merged even without java on PATH")

local found_warning = false
for _, notification in ipairs(notifications) do
  if notification.message:match("java") then
    found_warning = true
  end
end
assert(found_warning, "Missing java executable should trigger a notification mentioning java")

print("User extension executable check: OK")
