package selfishell

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPinnedNeovimConsumer(t *testing.T) {
	if os.Getenv("SELFISHELL_PINNED_NEOVIM_E2E") != "1" {
		t.Skip("requires SELFISHELL_PINNED_NEOVIM_E2E=1")
	}
	root, home := testRelease(t), t.TempDir()
	isolateHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", home+"/.config")
	t.Setenv("XDG_DATA_HOME", home+"/.local/share")
	t.Setenv("XDG_STATE_HOME", home+"/.local/state")
	t.Setenv("XDG_CACHE_HOME", home+"/.cache")
	t.Setenv("MISE_DATA_DIR", home+"/.local/share/mise")
	t.Setenv("MISE_STATE_DIR", home+"/.local/state/mise")
	t.Setenv("MISE_CACHE_DIR", home+"/.cache/mise")
	t.Setenv("SHELL", "/bin/zsh")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	var output bytes.Buffer
	p := Process{Out: &output, Err: &output, Env: DeveloperToolEnv(root)}
	if err := ProvisionDeveloper(ctx, root, "neovim-e2e", p); err != nil {
		t.Fatalf("pin provision: %v\n%s", err, output.String())
	}
	if code, _, stderr := nativeCLI(t, root, "install", "--skip-packages", "--yes"); code != 0 {
		t.Fatalf("config install: %s", stderr)
	}
	paths, err := UserPaths()
	if err != nil {
		t.Fatal(err)
	}
	managedConfig := paths.Config + "/mise/selfishell.toml"
	before, err := os.ReadFile(managedConfig)
	if err != nil {
		t.Fatal(err)
	}
	link := home + "/.config/mise/conf.d/selfishell.toml"
	linkTarget, err := os.Readlink(link)
	if err != nil {
		t.Fatal(err)
	}
	mise := home + "/.local/bin/mise"
	settings := withEnvironment(p, map[string]string{"PATH": home + "/.local/bin:" + os.Getenv("PATH")}, "MISE_GLOBAL_CONFIG_FILE", "MISE_DEFAULT_CONFIG_FILENAME", "MISE_OVERRIDE_CONFIG_FILENAMES")
	settings.Dir = home
	if code, err := settings.Run(ctx, mise, "settings", "set", "pin", "true"); err != nil || code != 0 {
		t.Fatalf("user mise settings: %v code %d\n%s", err, code, output.String())
	}
	userConfig, err := os.ReadFile(home + "/.config/mise/config.toml")
	if err != nil || !strings.Contains(string(userConfig), "pin = true") {
		t.Fatalf("user pin: %q %v", userConfig, err)
	}
	after, _ := os.ReadFile(managedConfig)
	if !bytes.Equal(before, after) {
		t.Fatal("user mise settings changed managed config")
	}
	if got, err := os.Readlink(link); err != nil || got != linkTarget {
		t.Fatalf("managed mise link: %s %v", got, err)
	}
	op := &PackageOperation{Process: p}
	output.Reset()
	if err := op.InstallNeovimPlugins(ctx, root, paths, root+"/dependencies.conf", false); err != nil {
		t.Fatalf("Neovim plugin provisioning: %v\n%s", err, output.String())
	}
	deps, err := ReadDependencies(root + "/dependencies.conf")
	if err != nil {
		t.Fatal(err)
	}
	for _, dep := range deps {
		if dep.Kind != "nvim-plugin" {
			continue
		}
		path, err := nvimPluginPath(paths, dep)
		if err != nil {
			t.Fatal(err)
		}
		actual, err := op.gitHead(ctx, path)
		if err != nil || actual != dep.Version {
			t.Fatalf("plugin %s HEAD %s want %s: %v", dep.Name, actual, dep.Version, err)
		}
	}
	if _, err := os.Stat(paths.State + "/nvim/lazy-lock.json"); err != nil {
		t.Fatal("runtime lock missing", err)
	}
	if _, err := os.Lstat(paths.Config + "/nvim/lazy-lock.json"); !os.IsNotExist(err) {
		t.Fatal("runtime lock polluted managed config")
	}
	if err := op.UpdateDefaultLSP(ctx, root, paths, false); err != nil {
		t.Fatalf("default LSP provisioning: %v\n%s", err, output.String())
	}
	receipts, err := filepath.Glob(home + "/.local/share/nvim/mason/packages/*/mason-receipt.json")
	if err != nil || len(receipts) != 7 {
		t.Fatalf("default LSP receipts: %v %v\n%s", receipts, err, output.String())
	}
	installed := make(map[string]time.Time)
	for _, receipt := range receipts {
		info, err := os.Stat(receipt)
		if err != nil {
			t.Fatal(err)
		}
		installed[receipt] = info.ModTime()
	}
	if err := op.UpdateDefaultLSP(ctx, root, paths, false); err != nil {
		t.Fatalf("repeated default LSP update: %v\n%s", err, output.String())
	}
	for receipt, modified := range installed {
		if info, err := os.Stat(receipt); err != nil || !info.ModTime().Equal(modified) {
			t.Fatalf("current LSP was reinstalled: %s %v", receipt, err)
		}
	}
	nvim, _, err := op.nvimCommand(ctx, root, paths)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := op.runNvim(ctx, root, nvim, mise, "--headless", "+lua "+`
local ok, message = pcall(function()
  require("lazy").load({ plugins = { "mason.nvim", "mason-lspconfig.nvim", "nvim-cmp" } })
  local item = require("cmp").get_config().formatting.format({}, { kind = "Function" })
  assert(item.icon and item.icon ~= "", "Missing completion kind icon from the snacks picker config")
  local mapping = require("mason-lspconfig.mappings").get_mason_map().lspconfig_to_package
  for _, specifier in ipairs(require("config.languages").lsp) do
    local server, version = require("mason-core.package").Parse(specifier)
    local package = require("mason-registry").get_package(mapping[server])
    assert(version and package:get_installed_version() == version, "Wrong installed version for " .. specifier)
  end
end)
if not ok then vim.api.nvim_err_writeln(tostring(message)); vim.cmd("cquit") end
`, "+qa"); err != nil {
		t.Fatalf("approved LSP versions: %v\n%s", err, output.String())
	}
	runDefaultLSPSelection(t, root, nvim)
	for _, probe := range []struct{ name, filename, content, marker string }{
		{"terraform", "main.tf", "terraform { required_version = \">= 1.0\" }\n", "Neovim developer smoke: OK"},
		{"python", "main.py", "def nested(value):\n    return {\"items\": [(value,)]}\n", "Python highlighting smoke: OK"},
		{"rainbow", "main.py", "", "Rainbow-delimiters smoke: OK"},
		{"bufnewfile", "brand-new.py", "", "BufNewFile indent smoke: OK"},
	} {
		file := filepath.Join(home, probe.filename)
		if probe.content != "" {
			if err := os.WriteFile(file, []byte(probe.content), 0600); err != nil {
				t.Fatal(err)
			}
		}
		lua := filepath.Join(root, "tests/fixtures/neovim", probe.name+"_smoke.lua")
		var result bytes.Buffer
		command := p
		command.Dir = root + "/config/shared"
		command.Out, command.Err = &result, &result
		code, err := command.Run(ctx, mise, "-C", command.Dir, "exec", "--", "nvim", "--headless", file, "+luafile "+lua, "+qa")
		if err != nil || code != 0 || !strings.Contains(result.String(), probe.marker) {
			t.Fatalf("%s probe: code=%d err=%v output=%s", probe.name, code, err, result.String())
		}
	}
	if _, err := os.Lstat(home + "/brand-new.py"); !os.IsNotExist(err) {
		t.Fatal(fmt.Errorf("BufNewFile probe created its target: %v", err))
	}
	runNeovimConfigFixtures(t, root, p, "nvim", mise)
	runNeovimDAPConsumer(t, ctx, root, home, p, op, paths, nvim, mise)
}
