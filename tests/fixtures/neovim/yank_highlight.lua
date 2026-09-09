local ok, err = pcall(function()
  require("config.autocmds")

  local buf = vim.api.nvim_get_current_buf()
  vim.api.nvim_buf_set_lines(buf, 0, -1, false, { "alpha", "bravo", "charlie" })

  local function yank_extmarks()
    local ns = vim.api.nvim_get_namespaces()["nvim.hlyank"]
    if not ns then
      return {}
    end
    return vim.api.nvim_buf_get_extmarks(buf, ns, 0, -1, { details = true })
  end

  local function clear_yank_extmarks()
    local ns = vim.api.nvim_get_namespaces()["nvim.hlyank"]
    if ns then
      vim.api.nvim_buf_clear_namespace(buf, ns, 0, -1)
    end
  end

  vim.api.nvim_win_set_cursor(0, { 1, 0 })
  vim.cmd("normal! y1j")
  local marks = yank_extmarks()
  assert(#marks == 1, "a yank did not produce exactly one highlight: " .. vim.inspect(marks))
  local details = marks[1][4]
  assert(
    marks[1][2] == 0 and details.end_row == 2,
    "the highlight does not span the yanked lines: " .. vim.inspect(marks)
  )
  clear_yank_extmarks()

  -- on_yank is documented to act on `y` only; delete and change redraw the
  -- screen on their own and must not flash.
  vim.api.nvim_win_set_cursor(0, { 1, 0 })
  vim.cmd("normal! dd")
  assert(#yank_extmarks() == 0, "a delete produced a yank highlight")
end)

for _, buf in ipairs(vim.api.nvim_list_bufs()) do
  pcall(function() vim.bo[buf].modified = false end)
end

print(ok and "yank highlight: OK" or ("yank highlight: FAIL " .. tostring(err)))
