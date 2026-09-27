package selfishell

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDefaultLSPSelection(t *testing.T) {
	root := testRelease(t)
	nvim, ok := standaloneNeovimExecutable(t, root)
	if !ok {
		t.Skip("requires the pinned physical Neovim executable")
	}
	runDefaultLSPSelection(t, root, nvim)
}

func runDefaultLSPSelection(t *testing.T, root, nvim string) {
	t.Helper()
	for _, tc := range []struct {
		mode, want string
		failed     bool
	}{
		{"mixed", "install:lua_ls-package,pyright-package,bashls-package", false},
		{"current", "registry:updated", false},
		{"install-warning", "install:lua_ls-package,pyright-package,bashls-package", false},
		{"registry-failure", "registry unavailable", true},
		{"missing-mapping", "lua_ls", true},
		{"install-failure", "installer failed", true},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			p := standaloneNeovimProcess(t, nvim)
			p = withEnvironment(p, map[string]string{"SELFISHELL_LSP_TEST_MODE": tc.mode})
			fixture := t.TempDir() + "/mason.lua"
			if err := os.WriteFile(fixture, []byte(defaultLSPFixtureLua), 0600); err != nil {
				t.Fatal(err)
			}
			p = withEnvironment(p, map[string]string{"SELFISHELL_LSP_TEST_FIXTURE": fixture})
			var output bytes.Buffer
			p.Out, p.Err = &output, &output
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			code, err := p.Run(ctx, nvim, "--headless", "-u", "NONE", "-i", "NONE",
				"--cmd", "set runtimepath^="+root+"/config/shared/nvim",
				"+lua dofile(vim.env.SELFISHELL_LSP_TEST_FIXTURE)", "+lua "+defaultLSPUpdateLua, "+qa")
			if err != nil || (code != 0) != tc.failed || !strings.Contains(output.String(), tc.want) {
				t.Fatalf("code=%d err=%v output=%s", code, err, output.String())
			}
			if tc.mode == "current" && strings.Contains(output.String(), "install:") {
				t.Fatal("reinstalled current servers")
			}
		})
	}
}

// Replace only Mason's external registry and installer; run the production Lua
// with the real default server list and Neovim command/error handling.
const defaultLSPFixtureLua = `
local mode = vim.env.SELFISHELL_LSP_TEST_MODE
local refreshed = false
local packages, mapping = {}, {}
for index, server in ipairs(require("config.languages").lsp) do
  local name = server .. "-package"
  mapping[server] = name
  packages[name] = {
    is_installed = function() return mode == "current" or index ~= 1 end,
    get_installed_version = function()
      if mode ~= "current" and index == 2 then return "old" end
      if mode ~= "current" and index == 3 then return nil end
      return "latest"
    end,
    get_latest_version = function() return "latest" end,
  }
end
if mode == "missing-mapping" then mapping.lua_ls = nil end
package.preload["lazy"] = function() return { load = function() end } end
package.preload["mason-registry"] = function() return {
  update = function(callback)
    vim.schedule(function()
      refreshed = true
      print("registry:updated")
      callback(mode ~= "registry-failure", "registry unavailable")
    end)
  end,
  get_package = function(name)
    assert(refreshed, "used stale registry")
    return assert(packages[name], "touched a user-added package")
  end,
} end
package.preload["mason-lspconfig.mappings"] = function() return {
  get_mason_map = function()
    assert(refreshed, "mapped before registry refresh")
    return { lspconfig_to_package = mapping }
  end,
} end
vim.api.nvim_create_user_command("MasonInstall", function(command)
  local quiet = command.fargs[1] == "--quiet"
  if quiet then table.remove(command.fargs, 1) end
  print("install:" .. table.concat(command.fargs, ","))
  if mode == "install-warning" and not quiet then
    vim.api.nvim_err_write("npm warn: successful install with a warning\n")
  end
  if mode == "install-failure" then
    print("installer failed")
    vim.cmd("cquit")
  end
end, { nargs = "+" })
`
