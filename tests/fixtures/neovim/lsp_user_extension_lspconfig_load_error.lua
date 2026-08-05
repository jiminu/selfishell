local notifications = {}
local original_notify = vim.notify
vim.notify = function(message, level)
  table.insert(notifications, { message = message, level = level })
end

local languages = require("config.languages")

vim.notify = original_notify

assert(not vim.tbl_contains(languages.lsp, "broken_server"),
  "a server whose nvim-lspconfig file fails to load must not be merged")

local found_load_error = false
for _, notification in ipairs(notifications) do
  if notification.message:find("failed to load", 1, true) then
    found_load_error = true
    assert(notification.level == vim.log.levels.ERROR, "the load-failure notification must be at ERROR level")
    assert(notification.message:find("simulated real breakage", 1, true),
      "the load-failure notification must include the real error, got: " .. notification.message)
  end
end
assert(found_load_error, "a real nvim-lspconfig load failure must be surfaced distinctly from 'not found'")

print("User extension lspconfig load error: OK")
