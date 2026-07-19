local M = {}
local revisions = {}
local manifest = vim.fn.stdpath("config") .. "/plugin-versions.conf"

for line in io.lines(manifest) do
  local kind, repository, revision = line:match("^(%S+)%s+(%S+)%s+(%S+)")
  if kind == "nvim-plugin" then
    revisions[repository] = revision
  end
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
