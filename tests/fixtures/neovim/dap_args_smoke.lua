require("lazy").load({ plugins = { "mason-nvim-dap.nvim" } })
local dap = require("dap")
local defaults = require("mason-nvim-dap.mappings.configurations")
local input = vim.fn.input
for _, adapter in ipairs({ "delve", "codelldb" }) do
  local config = {
    name = adapter, adapters = {}, filetypes = { "args-test-" .. adapter },
    configurations = vim.deepcopy(defaults[adapter]),
  }
  require("config.dap").setup_adapter(config)
  local with_args = dap.configurations[config.filetypes[1]][2]
  for _, case in ipairs({
    { [[--label "hello world" 'another value']], { "--label", "hello world", "another value" } },
    { [[--label=hello\ world]], { "--label=hello world" } },
    { [["" plain]], { "", "plain" } },
    { "   ", {} },
  }) do
    vim.fn.input = function() return case[1] end
    assert(vim.deep_equal(with_args.args(), case[2]), adapter .. " split quoted/empty arguments incorrectly")
  end
  local personal = { name = "Personal", type = adapter, args = { "hello world" } }
  dap.configurations[config.filetypes[1]][#dap.configurations[config.filetypes[1]] + 1] = personal
  require("config.dap").setup_adapter(config)
  assert(vim.deep_equal(personal.args, { "hello world" }), "setup changed personal arguments")
end
vim.fn.input = input
print("DAP args smoke: OK")
