local languages = require("config.languages")

local pyright_count = 0
for _, entry in ipairs(languages.lsp) do
  if entry == "pyright@1.1.411" then
    pyright_count = pyright_count + 1
  end
  assert(entry ~= "pyright", "redeclaring pyright must not add a second, unpinned entry")
  assert(entry ~= "pyright@2.0.0", "a versioned redeclaration must not add a second, differently-pinned entry")
end
assert(pyright_count == 1, "pyright@1.1.411 must remain present exactly once")

print("User extension redeclare handling: OK")
