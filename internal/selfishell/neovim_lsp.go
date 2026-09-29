package selfishell

import (
	"context"
	"fmt"
	"time"
)

// MasonInstall waits for every installer in headless mode and exits nonzero on
// failure. Filter first because the command otherwise reinstalls current tools.
const defaultLSPUpdateLua = `
local ok, message = pcall(function()
  require("lazy").load({ plugins = { "mason.nvim", "mason-lspconfig.nvim" } })
  local registry = require("mason-registry")
  local function pending_servers()
    local mapping = require("mason-lspconfig.mappings").get_mason_map().lspconfig_to_package
    local pending = {}
    for _, specifier in ipairs(require("config.languages").lsp) do
      local server, version = specifier:match("^([^@]+)@([^@]+)$")
      assert(server and version, "Missing approved version for default LSP server: " .. specifier)
      local name = assert(mapping[server], "No Mason package for default LSP server: " .. server)
      local package = registry.get_package(name)
      if not package:is_installed() or package:get_installed_version() ~= version then
        table.insert(pending, name .. "@" .. version)
      end
    end
    return pending
  end
  local readable, pending = pcall(pending_servers)
  if not readable or #pending > 0 then
    local updated, result
    registry.update(function(success, registries) updated, result = success, registries end)
    assert(vim.wait(60000, function() return updated ~= nil end), "Mason registry update timed out")
    assert(updated, "Mason registry update failed: " .. vim.inspect(result))
    pending = pending_servers()
  end
  if #pending > 0 then
    -- Installer stderr (including npm warnings) otherwise becomes an nvim_cmd
    -- error even on success. Quiet still reports failures and exits nonzero.
    table.insert(pending, 1, "--quiet")
    vim.api.nvim_cmd({ cmd = "MasonInstall", args = pending }, {})
  end
end)
if not ok then vim.api.nvim_err_writeln(tostring(message)); vim.cmd("cquit") end
`

func (o *PackageOperation) UpdateDefaultLSP(ctx context.Context, root string, paths Paths, dryRun bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if dryRun {
		o.report(reportPreview, "Would update default Neovim LSP servers to this release's approved versions.")
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	nvim, mise, err := o.nvimCommand(ctx, root, paths)
	if err != nil {
		return err
	}
	if o.Process.progress == nil {
		fmt.Fprintln(o.Process.Out, "Updating default Neovim LSP servers to approved versions...")
	}
	if _, err := o.runNvim(ctx, root, nvim, mise, "--headless", "+lua "+defaultLSPUpdateLua, "+qa"); err != nil {
		return fmt.Errorf("could not update default Neovim LSP servers: %w; retry with selfishell update --tools-only", err)
	}
	o.report(reportSuccess, "Neovim LSP servers: approved versions")
	return nil
}
