local plugin = require("config.plugin_versions").spec

local function action(name)
  return function() require("dap")[name]() end
end

local function start_or_continue()
  require("config.dap").continue()
end

return {
  plugin("mfussenegger/nvim-dap", {
    lazy = true,
    config = function()
      vim.fn.sign_define("DapBreakpoint", { text = "", texthl = "DiagnosticError" })
      require("config.dap").setup_start()
    end,
  }),
  plugin("rcarriga/nvim-dap-ui", {
    lazy = true,
    dependencies = {
      plugin("mfussenegger/nvim-dap"),
      plugin("nvim-neotest/nvim-nio"),
    },
    opts = {},
    config = function(_, opts)
      require("dapui").setup(opts)
      require("config.dap").setup_ui()
    end,
  }),
  -- One entrypoint loads Mason, DAP and its UI before registering adapters.
  -- Both :DapInstall and the first debug key work in a fresh Neovim process.
  plugin("jay-babu/mason-nvim-dap.nvim", {
    dependencies = {
      plugin("mason-org/mason.nvim"),
      plugin("mfussenegger/nvim-dap"),
      plugin("rcarriga/nvim-dap-ui"),
    },
    cmd = {
      "DapInstall", "DapUninstall", "DapContinue", "DapNew", "DapToggleBreakpoint",
      "DapStepOver", "DapStepInto", "DapStepOut", "DapTerminate", "DapDisconnect",
      "DapToggleRepl", "DapEval", "DapPause", "DapRestartFrame",
    },
    keys = {
      { "<F5>", start_or_continue, desc = "Debug: start / continue" },
      { "<F9>", action("toggle_breakpoint"), desc = "Debug: toggle breakpoint" },
      { "<F10>", action("step_over"), desc = "Debug: step over" },
      { "<F11>", action("step_into"), desc = "Debug: step into" },
      { "<S-F11>", action("step_out"), desc = "Debug: step out" },
      { "<S-F5>", action("terminate"), desc = "Debug: terminate" },
      -- Traditional terminals decode Shift+F5/F11 as F17/F23.
      { "<F23>", action("step_out"), desc = "Debug: step out" },
      { "<F17>", action("terminate"), desc = "Debug: terminate" },
      { "<leader>Dc", start_or_continue, desc = "Debug: start / continue (F5)" },
      { "<leader>Db", action("toggle_breakpoint"), desc = "Debug: toggle breakpoint (F9)" },
      { "<leader>Do", action("step_over"), desc = "Debug: step over (F10)" },
      { "<leader>Di", action("step_into"), desc = "Debug: step into (F11)" },
      { "<leader>DO", action("step_out"), desc = "Debug: step out (Shift+F11)" },
      { "<leader>Dt", action("terminate"), desc = "Debug: terminate (Shift+F5)" },
      { "<leader>Du", function() require("config.dap").toggle_ui() end, desc = "Debug: toggle UI" },
      { "<leader>De", function() require("dapui").eval() end, mode = { "n", "x" }, desc = "Debug: evaluate expression" },
    },
    opts = {
      ensure_installed = {},
      automatic_installation = false,
      handlers = {
        function(config) require("config.dap").setup_adapter(config) end,
      },
    },
  }),
}
