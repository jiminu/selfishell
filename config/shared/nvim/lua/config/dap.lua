local M = {}
local editors = {}

-- Shared by nvim-dap frame jumps and dap-ui's Breakpoints/Stacks navigation.
function M.source_window(buf)
  local api = vim.api
  local current = api.nvim_get_current_win()
  local candidates = { current, vim.fn.win_getid(vim.fn.winnr("#")) }
  vim.list_extend(candidates, api.nvim_tabpage_list_wins(0))
  local target
  -- Prefer an existing view of this source, then a normal editor. The previous
  -- window may be a dap-ui panel whose buffer guard rejects source navigation.
  for _, existing in ipairs({ true, false }) do
    for _, win in ipairs(candidates) do
      if api.nvim_win_is_valid(win) and api.nvim_win_get_config(win).relative == "" then
        local shown = api.nvim_win_get_buf(win)
        if (existing and shown == buf) or (not existing and vim.bo[shown].buftype == "" and not vim.wo[win].winfixbuf) then
          target = win
          break
        end
      end
    end
    if target then break end
  end
  if not target then
    vim.cmd("botright vertical " .. (buf and "sbuffer " .. buf or "new"))
    target = api.nvim_get_current_win()
    local editor = editors[api.nvim_get_current_tabpage()]
    for option, value in pairs(editor and editor.options or {}) do vim.wo[target][option] = value end
  end
  return target
end

local function focus_source(buf, line, column)
  local api = vim.api
  local current = api.nvim_get_current_win()
  local keep_repl = vim.bo[api.nvim_win_get_buf(current)].filetype == "dap-repl"
  local target = M.source_window(buf)
  api.nvim_win_set_buf(target, buf)
  local ok, err = pcall(api.nvim_win_set_cursor, target, { line, math.max(column - 1, 0) })
  if not ok then
    vim.notify("Cannot show debug location: " .. tostring(err) .. ". Check that source and executable match.", vim.log.levels.WARN)
  else
    api.nvim_win_call(target, function() vim.cmd("normal! zv") end)
  end
  api.nvim_set_current_win(keep_repl and current or target)
end

function M.continue()
  local dap = require("dap")
  if dap.session() or next(dap.sessions()) then return dap.continue() end
  local buf = vim.api.nvim_get_current_buf()
  local filetype = vim.b[buf]["dap-srcft"] or vim.bo[buf].filetype
  -- Providers can yield and supply configurations without dap.configurations.
  -- Collect once so custom providers do not prompt or perform work twice.
  require("dap.async").run(function()
    local configs = {}
    local names = vim.tbl_keys(dap.providers.configs)
    table.sort(names)
    for _, name in ipairs(names) do
      local supplied = dap.providers.configs[name](buf)
      if not vim.islist(supplied) then
        vim.notify("Debug configuration provider " .. name .. " must return a list.", vim.log.levels.WARN)
      else
        vim.list_extend(configs, supplied)
      end
    end
    if #configs == 0 then
      local adapters = { python = "python", go = "delve", javascript = "js", typescript = "js", c = "codelldb", cpp = "codelldb", rust = "codelldb" }
      local adapter = adapters[filetype]
      local hint = "Add a launch configuration (:help dap-configuration or :help dap-launch.json)."
      local registered = adapter and (dap.adapters[adapter] or (adapter == "js" and (dap.adapters["pwa-node"] or dap.adapters.node)))
      if adapter and not registered then
        local package_name = require("mason-nvim-dap.mappings.source").nvim_dap_to_package[adapter]
        local ok, package = pcall(require("mason-registry").get_package, package_name)
        if not ok or not package:is_installed() then
          hint = "Install the debugger with :DapInstall " .. adapter .. ". Custom setups: :help dap-configuration."
        end
      end
      vim.notify("No debug configuration for " .. (filetype ~= "" and filetype or "this buffer") .. ". " .. hint, vim.log.levels.INFO)
      return
    end
    require("dap.ui").pick_if_many(configs, "Configuration: ", function(config) return config.name end, function(config)
      if config then dap.run(config, { filetype = filetype }) end
    end)
  end)
end

function M.setup_start()
  local dap = require("dap")
  dap.defaults.fallback.switchbuf = dap.defaults.fallback.switchbuf or focus_source
  dap.listeners.on_config.selfishell = function(config)
    if config.request ~= "launch" or config.__pendingTargetId then return config end
    local root = vim.fn.getcwd():gsub("/$", "") .. "/"
    local modified = {}
    for _, buf in ipairs(vim.api.nvim_list_bufs()) do
      local name = vim.api.nvim_buf_get_name(buf)
      if vim.bo[buf].buflisted and vim.bo[buf].buftype == "" and vim.bo[buf].modified
        and (vim.startswith(name, root) or buf == vim.api.nvim_get_current_buf()) then
        modified[#modified + 1] = name == "" and "[No Name]" or vim.fn.fnamemodify(name, ":.")
      end
    end
    if #modified > 0 then
      vim.notify("Unsaved changes in " .. table.concat(modified, ", ") .. ". Debugging uses files on disk; save with :wa and restart.", vim.log.levels.WARN)
    end
    return config
  end
end

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

local function setup_javascript(config)
  local dap = require("dap")
  if not dap.adapters["pwa-node"] then
    -- Mason installs js-debug, but the bridge has no adapter/launch mapping.
    config.name = "pwa-node"
    config.adapters = {
      type = "server",
      host = "127.0.0.1",
      port = "${port}",
      executable = {
        command = vim.fn.exepath("js-debug-adapter"),
        args = { "${port}", "127.0.0.1" },
      },
      enrich_config = function(launch, on_config)
        local resolved = vim.deepcopy(launch)
        if resolved.type == "node" then resolved.type = "pwa-node" end
        -- Standalone js-debug needs the workspace to discover source maps
        -- before the program starts, including for project launch.json files.
        resolved.__workspaceFolder = resolved.__workspaceFolder or vim.fn.getcwd()
        on_config(resolved)
      end,
    }
    config.filetypes = { "javascript", "typescript" }
    config.configurations = {
      { name = "Node: Launch current file", request = "launch", program = "${file}" },
      {
        name = "Node: Launch JavaScript file", request = "launch",
        program = function()
          local path = vim.fn.input("Path to JavaScript entrypoint: ", vim.fn.getcwd() .. "/", "file")
          return path ~= "" and vim.fn.fnamemodify(path, ":p") or dap.ABORT
        end,
      },
      {
        name = "Node: Attach (port)", request = "attach", address = "127.0.0.1",
        port = function()
          local input = vim.fn.input("Node inspector port: ", "9229")
          if input == "" then return dap.ABORT end
          local port = tonumber(input)
          if port and port % 1 == 0 and port >= 1 and port <= 65535 then return port end
          vim.notify("Node inspector port must be an integer from 1 to 65535.", vim.log.levels.WARN)
          return dap.ABORT
        end,
      },
    }
    for _, launch in ipairs(config.configurations) do
      launch.type = "pwa-node"
      launch.cwd = "${workspaceFolder}"
      launch.sourceMaps = true
      if launch.request == "launch" then launch.console = "integratedTerminal" end
      launch.skipFiles = { "<node_internals>/**" }
      launch.outFiles = { "${workspaceFolder}/**/*.js", "${workspaceFolder}/**/*.mjs", "${workspaceFolder}/**/*.cjs", "!**/node_modules/**" }
    end
    require("mason-nvim-dap").default_setup(config)
  end
  -- VS Code launch.json files normally use the shorter alias.
  dap.adapters.node = dap.adapters.node or dap.adapters["pwa-node"]
end

function M.setup_adapter(config)
  if config.name == "js" then return setup_javascript(config) end
  -- Mason emits install success on reinstalls too. Keep existing registrations,
  -- including user overrides, rather than append duplicate launch configurations.
  if require("dap").adapters[config.name] then return end
  if config.name == "python" then
    for _, launch in ipairs(config.configurations or {}) do
      launch.pythonPath = python_path
    end
  elseif config.name == "delve" or config.name == "codelldb" then
    for _, launch in ipairs(config.configurations or {}) do
      if type(launch.args) == "function" then
        launch.args = function()
          return require("dap.utils").splitstr(vim.fn.input("Args: "))
        end
      end
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
