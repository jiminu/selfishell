require("lazy").load({ plugins = { "nvim-tree.lua", "nvim-dap-ui" } })
vim.o.columns, vim.o.lines = 160, 35
local api = vim.api
local dap, ui = require("dap"), require("dapui")
local editor, source = api.nvim_get_current_win(), api.nvim_get_current_buf()
local lines = {}
for i = 1, 120 do lines[i] = "source line " .. i end
api.nvim_buf_set_lines(source, 0, -1, false, lines)
vim.bo[source].modified = false
require("nvim-tree.api").tree.open()
api.nvim_set_current_win(editor)
ui.open()
local panels = {}
for _, win in ipairs(api.nvim_list_wins()) do
  local ft = vim.bo[api.nvim_win_get_buf(win)].filetype
  if ft:match("^dapui_") or ft == "dap-repl" then panels[ft] = win end
end

-- Exercise nvim-dap's real frame navigation with dap-ui's buffer guards.
-- Only the adapter's scope request is omitted; this needs no external debugger.
local session = setmetatable({ config = { type = "focus-test" }, filetype = "text",
  sign_group = "focus-test", _request_scopes = function() end,
}, { __index = require("dap.session") })
local function jump(buf, line)
  session:_frame_set({ id = 1, name = "main", source = { path = api.nvim_buf_get_name(buf) }, line = line, column = 1 })
  vim.cmd.redraw()
end
local function visit_panels()
  api.nvim_set_current_win(panels.dapui_breakpoints)
  api.nvim_set_current_win(panels.dapui_scopes)
end
visit_panels()
jump(source, 90)
assert(api.nvim_win_get_buf(editor) == source and api.nvim_win_get_cursor(editor)[1] == 90,
  "stepping after visiting debug panels did not move the source cursor")
assert(vim.fn.line("w0", editor) <= 90 and vim.fn.line("w$", editor) >= 90,
  "stepping left the execution line outside the viewport")
assert(api.nvim_get_current_win() == editor, "stepping from Scopes did not focus the editor")
for ft, win in pairs(panels) do
  assert(vim.bo[api.nvim_win_get_buf(win)].filetype == ft, "stepping replaced a debug panel: " .. ft)
end

-- A newly encountered source must use an editor too, not the previous panel.
local other = api.nvim_create_buf(true, false)
api.nvim_buf_set_name(other, vim.env.HOME .. "/other-source.txt")
api.nvim_buf_set_lines(other, 0, -1, false, lines)
vim.bo[other].modified = false
visit_panels()
jump(other, 100)
assert(api.nvim_win_get_buf(editor) == other and api.nvim_win_get_cursor(editor)[1] == 100,
  "a new source file did not reuse the editor")

-- Preserve the REPL input focus while following the source in its own window.
api.nvim_set_current_win(panels.dapui_breakpoints)
api.nvim_set_current_win(panels["dap-repl"])
jump(other, 25)
assert(api.nvim_get_current_win() == panels["dap-repl"], "stepping stole REPL input focus")
assert(api.nvim_win_get_cursor(editor)[1] == 25 and vim.fn.line("w0", editor) <= 25,
  "stepping from the REPL did not scroll the source")

-- If the user closed the source window, create an editor without consuming UI.
api.nvim_win_close(editor, true)
visit_panels()
jump(source, 75)
local restored = vim.fn.win_findbuf(source)[1]
assert(restored and api.nvim_win_get_cursor(restored)[1] == 75, "no editor restored when stepping after :q")
for ft, win in pairs(panels) do
  assert(api.nvim_win_is_valid(win) and vim.bo[api.nvim_win_get_buf(win)].filetype == ft,
    "source recovery consumed a debug panel: " .. ft)
end
print("DAP focus smoke: OK")
