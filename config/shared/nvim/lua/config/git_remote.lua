local M = {}
local repositories = {}
local active_roots = {}
local current_root
local started = false

local function root_for(path)
  return vim.fs.root(path, ".git")
end

local function tree_root()
  local api = package.loaded["nvim-tree.api"]
  if api and api.tree.is_visible() then
    local node = api.tree.get_node_under_cursor()
    while node and node.parent do node = node.parent end
    node = node or api.tree.get_nodes()
    return node and root_for(node.absolute_path)
  end
end

local function redraw()
  local api = package.loaded["nvim-tree.api"]
  if api and api.tree.is_visible() then
    api.tree.reload()
  end
  vim.cmd.redrawstatus()
end

local function git(root, args, callback)
  local command = { "git", "--no-optional-locks", "-C", root }
  vim.list_extend(command, args)
  -- Rename detection can otherwise lazily fetch blobs in a partial clone.
  local options = {
    timeout = 5000,
    env = { GIT_NO_LAZY_FETCH = "1", GIT_ALLOW_PROTOCOL = "", GIT_TERMINAL_PROMPT = "0" },
  }
  vim.system(command, options, vim.schedule_wrap(function(result)
    callback(result.code == 0 and result.stdout or nil)
  end))
end

local function incoming_files(root, output)
  local files = {}
  local function add(path, status)
    path = root .. "/" .. path
    files[path] = "↓" .. status
    local parent = vim.fs.dirname(path)
    while parent and #parent >= #root do
      files[parent] = "↓"
      if parent == root then break end
      parent = vim.fs.dirname(parent)
    end
  end
  -- NUL separators preserve spaces, tabs and newlines in filenames.
  local fields = vim.split(output, "\0", { plain = true, trimempty = true })
  local index = 1
  while index < #fields do
    local status = fields[index]:sub(1, 1)
    add(fields[index + 1], status)
    index = index + 2
    if status == "R" or status == "C" then
      add(fields[index], status)
      index = index + 1
    end
  end
  return files
end

local function refresh(root)
  if not root then return end
  local entry = repositories[root]
  if not entry then
    entry = { status = "", files = {} }
    repositories[root] = entry
  end
  if entry.busy then return end
  entry.busy = true
  local function finish(refs, status, files)
    local changed = entry.status ~= status or not vim.deep_equal(entry.files, files)
    entry.refs, entry.status, entry.files, entry.busy = refs, status, files, false
    if changed then redraw() end
  end
  -- Resolving refs is cheap; walk history and diff only when either tip changes.
  -- Git resolves worktrees, packed refs and upstream configuration itself.
  git(root, { "rev-parse", "HEAD", "@{upstream}" }, function(refs)
    if not refs then
      finish(nil, "", {}) -- Detached/unborn HEAD, missing upstream or Git failure.
      return
    end
    if refs == entry.refs then
      entry.busy = false
      return
    end
    local head, upstream = refs:match("^(%x+)\n(%x+)\n$")
    if not head then
      finish(nil, "", {})
      return
    end
    local range = head .. "..." .. upstream
    git(root, { "rev-list", "--left-right", "--count", range }, function(counts)
      local ahead, behind
      if counts then ahead, behind = counts:match("^(%d+)%s+(%d+)") end
      if not ahead then
        finish(nil, "", {})
        return
      end
      local parts = {}
      if tonumber(ahead) > 0 then parts[#parts + 1] = "↑" .. ahead end
      if tonumber(behind) > 0 then parts[#parts + 1] = "↓" .. behind end
      local status = table.concat(parts, " ")
      if tonumber(behind) == 0 then
        finish(refs, status, {})
        return
      end
      -- Compare the upstream side from the merge base, excluding local commits.
      local args = { "diff", "--no-ext-diff", "--no-textconv", "--name-status", "-z", "--find-renames", range, "--" }
      local function finish_diff(diff)
        -- Counts remain useful for shallow/unrelated histories with no merge base.
        finish(diff and refs or nil, status, diff and incoming_files(root, diff) or {})
      end
      git(root, args, function(diff)
        if diff then
          finish_diff(diff)
        else
          -- Missing blobs still permit tree-only A/D badges, without similarity checks.
          args[6] = "--no-renames"
          git(root, args, finish_diff)
        end
      end)
    end)
  end)
end

function M.refresh()
  -- Match lualine's branch context, even when the tree browses another repo.
  local path = vim.fn.expand("%:p:h")
  if path:match("term://.*") then
    path = vim.fn.expand(path:gsub("term://(.+)//.+", "%1"))
  end
  local explorer_root = tree_root()
  current_root = root_for(path)
  local roots = {}
  if current_root then roots[current_root] = true end
  if explorer_root then roots[explorer_root] = true end
  local changed = not vim.deep_equal(active_roots, roots)
  active_roots = roots
  refresh(current_root)
  if explorer_root ~= current_root then refresh(explorer_root) end
  if changed then redraw() end
end

-- UI callbacks only read cached results; they never spawn Git or access the network.
function M.status()
  local entry = repositories[current_root]
  return entry and entry.status or ""
end

function M.incoming(path)
  for root in pairs(active_roots) do
    local entry = repositories[root]
    if entry and entry.files[path] then return entry.files[path] end
  end
end

function M.decorator()
  local highlights = {
    ["↓M"] = "SelfishellGitModified",
    ["↓A"] = "SelfishellGitAdded",
    ["↓R"] = "SelfishellGitAdded",
    ["↓D"] = "SelfishellGitDeleted",
  }
  local Decorator = require("nvim-tree.api").Decorator:extend()
  function Decorator:new()
    self.enabled = true
    self.highlight_range = "none"
    self.icon_placement = "right_align"
  end
  function Decorator:icons(node)
    local status = M.incoming(node.absolute_path)
    if status then return { { str = status, hl = { highlights[status] or "NvimTreeGitDirtyIcon" } } } end
  end
  return Decorator
end

function M.setup()
  if started then return end
  started = true
  local group = vim.api.nvim_create_augroup("SelfishellGitRemote", { clear = true })
  local function colors()
    local dark = vim.o.background == "dark"
    vim.api.nvim_set_hl(0, "SelfishellGitModified", { fg = dark and "#D7BA7D" or "#895503" })
    vim.api.nvim_set_hl(0, "SelfishellGitAdded", { fg = dark and "#5fd700" or "#008000" })
    vim.api.nvim_set_hl(0, "SelfishellGitDeleted", { fg = dark and "#F44747" or "#FF0000" })
  end
  vim.api.nvim_create_autocmd("ColorScheme", { group = group, callback = colors })
  colors()
  vim.api.nvim_create_autocmd({ "BufEnter", "DirChanged", "FocusGained", "TermLeave", "BufWritePost" }, {
    group = group,
    callback = M.refresh,
  })
  -- Catch fetch/push/checkout in other terminals, including while the tree is idle.
  local timer = vim.uv.new_timer()
  timer:start(5000, 5000, vim.schedule_wrap(M.refresh))
  vim.api.nvim_create_autocmd("VimLeavePre", {
    group = group,
    once = true,
    callback = function() timer:stop(); timer:close() end,
  })
  M.refresh()
end

return M
