local M = {}
local editors = {}

local function remember_editor()
  local api = vim.api
  local wins = api.nvim_tabpage_list_wins(0)
  table.insert(wins, 1, api.nvim_get_current_win())
  for _, win in ipairs(wins) do
    local buf = api.nvim_win_get_buf(win)
    if vim.bo[buf].buftype == "" and api.nvim_win_get_config(win).relative == "" then
      local editor = { buf = buf, options = {} }
      for _, option in ipairs({ "list", "number", "relativenumber", "winfixwidth", "winfixheight", "wrap", "signcolumn", "spell", "winhighlight" }) do
        editor.options[option] = api.nvim_get_option_value(option, { win = win })
      end
      editors[api.nvim_get_current_tabpage()] = editor
      return
    end
  end
end

local function ensure_editor_windows()
  local api = vim.api
  local panels, tabs = {}, {}
  -- Only dap-ui's own windows count; :DapToggleRepl can open a separate REPL.
  for _, layout in ipairs(require("dapui.windows").layouts) do
    for _, win in pairs(layout.opened_wins) do
      if api.nvim_win_is_valid(win) then
        local tab = api.nvim_win_get_tabpage(win)
        panels[#panels + 1] = { win = win, tab = tab }
        tabs[tab] = win
      end
    end
  end
  for tab in pairs(editors) do
    if not api.nvim_tabpage_is_valid(tab) then editors[tab] = nil end
  end
  for tab, debug_win in pairs(tabs) do
    local has_editor
    for _, win in ipairs(api.nvim_tabpage_list_wins(tab)) do
      if api.nvim_win_get_config(win).relative == "" then
        local buf = api.nvim_win_get_buf(win)
        if vim.bo[buf].buftype == "" then has_editor = true end
      end
    end
    if not has_editor then
      local source = editors[tab]
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
  return panels
end

local function close_ui()
  local panels = ensure_editor_windows()
  -- Removing the bottom tray first preserves an adjacent file explorer's width.
  for layout = #require("dapui.config").layouts, 1, -1 do
    require("dapui").close({ layout = layout })
  end
  for _, panel in ipairs(panels) do
    if not vim.api.nvim_win_is_valid(panel.win) then return panel.tab end
  end
end

function M.toggle_ui()
  if not close_ui() then
    remember_editor()
    require("dapui").open()
  end
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
    -- Disconnects and adapter failures do not always send terminated/exited.
    -- on_close may run in a libuv callback; wait until DAP clears its session.
    session.on_close.selfishell = vim.schedule_wrap(function()
      if not dap.session() then
        close_ui()
      end
    end)
    -- A user can :q individual panels. Rebuild through the public API so stale
    -- window IDs cannot break initialization, keeping the UI in its original tab.
    local tab = close_ui()
    if tab then
      vim.api.nvim_win_call(vim.api.nvim_tabpage_get_win(tab), function() ui.open() end)
    else
      ui.open()
    end
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
