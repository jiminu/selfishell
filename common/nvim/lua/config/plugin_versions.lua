local M = {}
local revisions = {}
local manifest = vim.fn.stdpath("config") .. "/plugin-versions.conf"

-- io.lines(manifest) throws immediately if the manifest is missing (a
-- partial/interrupted install, or Neovim pointed straight at this config
-- without running the installer), which would crash `require` for every
-- caller with a raw traceback. Degrade to an empty manifest instead; callers
-- already report a clear error for an individual missing/mismatched
-- revision (see config/lazy.lua's "lazy.nvim revision does not match").
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
  return spec
end

function M.revision(repository)
  return revisions[repository]
end

return M
