local M = {}
local editor

local function remember_editor()
  local api = vim.api
  local wins = api.nvim_tabpage_list_wins(0)
  table.insert(wins, 1, api.nvim_get_current_win())
  for _, win in ipairs(wins) do
    local buf = api.nvim_win_get_buf(win)
    if vim.bo[buf].buftype == "" and api.nvim_win_get_config(win).relative == "" then
      editor = { buf = buf, options = {} }
      for _, option in ipairs({ "list", "number", "relativenumber", "winfixwidth", "winfixheight", "wrap", "signcolumn", "spell", "winhighlight" }) do
        editor.options[option] = api.nvim_get_option_value(option, { win = win })
      end
      return
    end
  end
end

local function ensure_editor_windows(source)
  local api = vim.api
  local has_debug_ui = false
  for _, tab in ipairs(api.nvim_list_tabpages()) do
    local debug_win, has_editor
    for _, win in ipairs(api.nvim_tabpage_list_wins(tab)) do
      if api.nvim_win_get_config(win).relative == "" then
        local buf = api.nvim_win_get_buf(win)
        local ft = vim.bo[buf].filetype
        if vim.bo[buf].buftype == "" then has_editor = true end
        if ft:match("^dapui_") or ft == "dap-repl" then debug_win = win end
      end
    end
    has_debug_ui = has_debug_ui or debug_win ~= nil
    if debug_win and not has_editor then
      -- :q can leave only debug panels. Give dap-ui a normal window to keep
      -- before it hits E444 closing the last one. A fresh split also avoids
      -- dap-ui's buffer guard forcing Watches back into a reused panel.
      api.nvim_win_call(debug_win, function()
        if source and api.nvim_buf_is_valid(source.buf) and vim.bo[source.buf].buflisted and vim.bo[source.buf].buftype == "" then
          vim.cmd("botright vertical sbuffer " .. source.buf)
        else
          vim.cmd("botright vnew")
          -- New buffers otherwise inherit the panel's hidden line numbers and
          -- highlight overrides. Keep the editor's options even if it was deleted.
          for option, value in pairs(source and source.options or { winhighlight = "" }) do
            vim.wo[option] = value
          end
        end
      end)
    end
  end
  return has_debug_ui
end

function M.toggle_ui()
  if not ensure_editor_windows(editor) then remember_editor() end
  require("dapui").toggle()
end

local function python_path()
  -- Resolve when launching, since :cd and active environments can change.
  for _, directory in ipairs({ vim.env.VIRTUAL_ENV or "", vim.env.CONDA_PREFIX or "", vim.fn.getcwd() .. "/.venv" }) do
    if directory ~= "" and vim.fn.executable(directory .. "/bin/python") == 1 then
      return directory .. "/bin/python"
    end
  end
  for _, name in ipairs({ "python3", "python" }) do
    local path = vim.fn.exepath(name)
    if path ~= "" then return path end
  end
  vim.notify("Python is missing. Activate the project's Python environment before debugging.", vim.log.levels.WARN)
  return require("dap").ABORT
end

function M.setup_adapter(config)
  -- Mason emits install success on reinstalls too. Keep existing registrations,
  -- including user overrides, rather than append duplicate launch configurations.
  if require("dap").adapters[config.name] then return end
  if config.name == "python" then
    for _, launch in ipairs(config.configurations or {}) do
      launch.pythonPath = python_path
    end
  end
  require("mason-nvim-dap").default_setup(config)
end

function M.setup_ui()
  local dap, ui = require("dap"), require("dapui")
  dap.listeners.after.event_initialized.selfishell = function(session)
    remember_editor()
    local source = editor
    ui.open()
    -- Disconnects and adapter failures do not always send terminated/exited.
    -- on_close may run in a libuv callback; wait until DAP clears its session.
    session.on_close.selfishell = vim.schedule_wrap(function()
      if not dap.session() then
        ensure_editor_windows(source)
        -- Match toggle's close order: removing the left panel before the
        -- bottom tray makes Neovim add its width to an adjacent file explorer.
        for layout = #require("dapui.config").layouts, 1, -1 do
          ui.close({ layout = layout })
        end
      end
    end)
  end
  vim.api.nvim_create_autocmd("FileType", {
    group = vim.api.nvim_create_augroup("SelfishellDebugWindows", { clear = true }),
    pattern = { "dapui_*", "dap-repl" },
    callback = function(event)
      require("config.keymaps").set_window_navigation({ buffer = event.buf })
    end,
  })
end

return M
