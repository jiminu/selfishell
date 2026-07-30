local ok, err = pcall(function()
  require("config.autocmds")

  local current_win = vim.api.nvim_get_current_win()
  local buf_b = vim.api.nvim_get_current_buf()
  vim.api.nvim_buf_set_lines(buf_b, 0, -1, false, { "a", "b", "c" })
  vim.api.nvim_win_set_cursor(current_win, { 1, 0 })

  local buf_a = vim.api.nvim_create_buf(true, false)
  vim.api.nvim_buf_set_lines(buf_a, 0, -1, false, {
    "0123456789",
    "0123456789",
    "0123456789",
    "0123456789",
    "0123456789",
  })
  vim.api.nvim_buf_set_mark(buf_a, '"', 3, 2, {})
  local win_a = vim.api.nvim_open_win(buf_a, false, {
    relative = "editor",
    width = 10,
    height = 5,
    row = 0,
    col = 0,
  })
  vim.api.nvim_win_set_cursor(win_a, { 1, 0 })
  vim.api.nvim_exec_autocmds("BufReadPost", { buffer = buf_a })
  local a_cursor = vim.api.nvim_win_get_cursor(win_a)
  local b_cursor = vim.api.nvim_win_get_cursor(current_win)
  assert(
    a_cursor[1] == 3 and a_cursor[2] == 2,
    "cursor was not restored in the target window: " .. vim.inspect(a_cursor)
  )
  assert(
    b_cursor[1] == 1 and b_cursor[2] == 0,
    "cursor moved in an unrelated window: " .. vim.inspect(b_cursor)
  )

  local original_get_mark = vim.api.nvim_buf_get_mark
  local buf_c = vim.api.nvim_create_buf(true, false)
  vim.api.nvim_buf_set_lines(buf_c, 0, -1, false, { "only one line" })
  vim.api.nvim_buf_get_mark = function(buf, name)
    if buf == buf_c and name == '"' then
      return { 999, 0 }
    end
    return original_get_mark(buf, name)
  end
  local win_c = vim.api.nvim_open_win(buf_c, false, {
    relative = "editor",
    width = 10,
    height = 5,
    row = 0,
    col = 20,
  })
  vim.api.nvim_win_set_cursor(win_c, { 1, 0 })
  vim.api.nvim_exec_autocmds("BufReadPost", { buffer = buf_c })
  local c_cursor = vim.api.nvim_win_get_cursor(win_c)
  assert(
    c_cursor[1] == 1 and c_cursor[2] == 0,
    "an out-of-range mark moved the cursor: " .. vim.inspect(c_cursor)
  )
  vim.api.nvim_buf_get_mark = original_get_mark

  local buf_special = vim.api.nvim_create_buf(false, true)
  vim.api.nvim_buf_set_lines(buf_special, 0, -1, false, { "x", "y", "z" })
  vim.api.nvim_buf_set_mark(buf_special, '"', 2, 0, {})
  local win_special = vim.api.nvim_open_win(buf_special, false, {
    relative = "editor",
    width = 10,
    height = 5,
    row = 0,
    col = 40,
  })
  vim.api.nvim_win_set_cursor(win_special, { 1, 0 })
  vim.api.nvim_exec_autocmds("BufReadPost", { buffer = buf_special })
  local special_cursor = vim.api.nvim_win_get_cursor(win_special)
  assert(
    special_cursor[1] == 1 and special_cursor[2] == 0,
    "a special buffer moved the cursor: " .. vim.inspect(special_cursor)
  )
end)

for _, buf in ipairs(vim.api.nvim_list_bufs()) do
  pcall(function() vim.bo[buf].modified = false end)
end

print(ok and "cursor restore targeting: OK" or ("cursor restore targeting: FAIL " .. tostring(err)))
