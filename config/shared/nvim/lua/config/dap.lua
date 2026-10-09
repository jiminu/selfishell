local M = {}

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
    ui.open()
    -- Disconnects and adapter failures do not always send terminated/exited.
    -- on_close may run in a libuv callback; wait until DAP clears its session.
    session.on_close.selfishell = vim.schedule_wrap(function()
      if not dap.session() then
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
