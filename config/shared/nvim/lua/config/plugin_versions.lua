local M = {}
local revisions = {}
local manifest = vim.fn.stdpath("config") .. "/plugin-versions.conf"

-- io.lines throws on a missing manifest (a partial install, or Neovim run
-- straight against this config), crashing `require` with a raw traceback.
-- Degrade to empty: callers already report each missing revision clearly.
local file = io.open(manifest, "r")
if file then
  for line in file:lines() do
    local kind, repository, revision = line:match("^(%S+)%s+(%S+)%s+(%S+)")
    if kind == "nvim-plugin" then
      revisions[repository] = revision
    end
  end
  file:close()
end

function M.spec(repository, options)
  local revision = revisions[repository]
  assert(revision, "Missing approved Neovim plugin revision: " .. repository)
  assert(revision:match("^[0-9a-f]+$") and #revision == 40, "Invalid Neovim plugin revision: " .. repository)

  local spec = options or {}
  spec[1] = repository
  spec.commit = revision
  if spec.submodules == nil then
    spec.submodules = false
  end
  return spec
end

function M.revision(repository)
  return revisions[repository]
end

return M
