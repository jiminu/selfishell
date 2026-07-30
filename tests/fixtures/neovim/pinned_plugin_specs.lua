local modules = {
  "plugins.completion",
  "plugins.editor",
  "plugins.lsp",
  "plugins.telescope",
  "plugins.ui",
}

for _, module in ipairs(modules) do
  assert(type(require(module)) == "table", "Invalid plugin spec: " .. module)
end

print("pinned plugin specs: OK")
