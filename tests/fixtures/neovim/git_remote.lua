vim.opt.rtp:prepend(vim.env.SELFISHELL_NVIM_CONFIG)
local remote = require("config.git_remote")
remote.setup()
local tree, lualine
if vim.env.SELFISHELL_TEST_NVIM_PLUGIN_ROOT and vim.env.SELFISHELL_TEST_NVIM_PLUGIN_ROOT ~= "" then
  for _, plugin in ipairs({ "nvim-tree.lua", "lualine.nvim", "vscode.nvim" }) do
    vim.opt.rtp:append(vim.env.SELFISHELL_TEST_NVIM_PLUGIN_ROOT .. "/" .. plugin)
  end
  for _, spec in ipairs(require("plugins.ui")) do
    if spec[1] == "nvim-tree/nvim-tree.lua" or spec[1] == "nvim-lualine/lualine.nvim" then
      spec.config(spec, spec.opts)
    end
  end
  tree, lualine = require("nvim-tree.api").tree, require("lualine")
  vim.api.nvim_exec_autocmds("VimEnter", {})
end

local function rendered()
  local buf = vim.api.nvim_win_get_buf(tree.winid())
  local text = vim.api.nvim_buf_get_lines(buf, 0, -1, false)
  -- Right-aligned badges are virtual text, not buffer contents.
  for _, mark in ipairs(vim.api.nvim_buf_get_extmarks(buf, -1, 0, -1, { details = true })) do
    for _, chunk in ipairs(mark[4].virt_text or {}) do text[#text + 1] = chunk[1] end
  end
  return table.concat(text, "\n"), vim.api.nvim_eval_statusline(lualine.statusline(true), {}).str
end

-- Go owns the Git repositories and mutations; this probe only drives Neovim.
for line in io.lines() do
  local request = vim.json.decode(line)
  if not request.poll then
    vim.cmd.cd(vim.fn.fnameescape(request.path))
    vim.cmd.enew()
    if tree then
      tree.close()
      tree.open({ path = request.tree_path or request.path })
      tree.expand_all()
    end
    vim.api.nvim_exec_autocmds("FocusGained", {})
  end
  local function snapshot()
    local files = {}
    for _, name in ipairs(request.files) do
      files[name] = remote.incoming(request.path .. "/" .. name) or ""
    end
    return { status = remote.status(), files = files }
  end
  vim.wait(8000, function()
    if not vim.deep_equal(snapshot(), request.want) then return false end
    if tree then
      local badges, line_text = rendered()
      local counts = request.want.status:gsub(" ", "")
      if counts ~= "" and not line_text:find(counts, 1, true) then return false end
      if request.want.status == "" and (line_text:find("↑", 1, true) or line_text:find("↓", 1, true)) then return false end
      for name, status in pairs(request.want.files) do
        if status ~= "" and vim.uv.fs_stat(request.path .. "/" .. name) and not badges:find(status, 1, true) then return false end
      end
    end
    return true
  end, 20)
  local result = snapshot()
  if tree then
    result.tree, result.line = rendered()
  end
  io.stdout:write(vim.json.encode(result) .. "\n")
  io.stdout:flush()
end
