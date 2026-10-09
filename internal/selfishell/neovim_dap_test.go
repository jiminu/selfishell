package selfishell

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jiminu/selfishell/internal/testutil"
)

func TestNeovimDAPConfiguration(t *testing.T) {
	t.Parallel()
	root := testRelease(t)
	nvim, ok := standaloneNeovimExecutable(t, root)
	if !ok {
		t.Skip("requires the pinned physical Neovim executable")
	}
	p := standaloneNeovimProcess(t, nvim)
	home := envValue(p.Env, "HOME")
	config := home + "/.config/nvim"
	if err := os.MkdirAll(config, 0700); err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile(root + "/dependencies.conf")
	if err != nil {
		t.Fatal(err)
	}
	if err := testutil.WriteFile(config+"/plugin-versions.conf", manifest, 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"project/.venv/bin/python", "active/bin/python", "conda/bin/python", "other/.venv/bin/python", "bin/python3"} {
		path := home + "/" + name
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := testutil.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	p = withEnvironment(p, map[string]string{
		"PATH":                         home + "/bin:" + envValue(p.Env, "PATH"),
		"SELFISHELL_NVIM_TEST_FIXTURE": root + "/tests/fixtures/neovim/dap_setup.lua",
	}, "WSL_DISTRO_NAME", "VIRTUAL_ENV", "CONDA_PREFIX")
	p.Dir = home + "/project"
	var output bytes.Buffer
	p.Out, p.Err = &output, &output
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	code, err := p.Run(ctx, nvim, "--headless", "-u", "NONE", "-i", "NONE", "--cmd", "set runtimepath^="+root+"/config/shared/nvim", "+lua dofile(vim.env.SELFISHELL_NVIM_TEST_FIXTURE)", "+qa!")
	if err != nil || code != 0 || !strings.Contains(output.String(), "DAP configuration: OK") {
		t.Fatalf("DAP configuration: code=%d err=%v output=%s", code, err, output.String())
	}
}

func TestNeovimDAPUserConfigurationSurvivesLifecycle(t *testing.T) {
	root, home, paths := blockHome(t, "ubuntu")
	blockOK(t, root, "install", "--skip-packages", "--yes")
	user := paths.Config + "/nvim/lua/plugins/dap_user.lua"
	contents := "return { { 'jay-babu/mason-nvim-dap.nvim', opts = { handlers = { python = function() end } } } }\n"
	if err := testutil.WriteFile(user, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	receipt := home + "/.local/share/nvim/mason/packages/debugpy/mason-receipt.json"
	if err := os.MkdirAll(filepath.Dir(receipt), 0700); err != nil {
		t.Fatal(err)
	}
	if err := testutil.WriteFile(receipt, []byte("user-owned adapter\n"), 0600); err != nil {
		t.Fatal(err)
	}
	blockOK(t, root, "update", "--tools-only", "--skip-packages", "--yes")
	blockEqual(t, user, []byte(contents))
	blockEqual(t, receipt, []byte("user-owned adapter\n"))
	blockOK(t, root, "uninstall", "--restore", "--yes")
	blockEqual(t, user, []byte(contents))
	blockEqual(t, receipt, []byte("user-owned adapter\n"))
}

// The opt-in pinned consumer owns network provisioning. These probes use real
// plugin commits and real adapters; all language projects and installs are private.
func runNeovimDAPConsumer(t *testing.T, ctx context.Context, root, home string, p Process, op *PackageOperation, paths Paths, nvim, mise string) {
	t.Helper()
	pins, err := approvedMisePins(root+"/config/shared/mise.toml", []string{"python"})
	if err != nil {
		t.Fatal(err)
	}
	if err := op.InstallMise(ctx, root, paths, "required", false, pins...); err != nil {
		t.Fatal("DAP Python provisioning", err)
	}
	project := home + "/debug-project"
	if err := os.MkdirAll(project, 0700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"main.py": "value = 41\nvalue += 1\nprint(value)\n",
		"go.mod":  "module debugfixture\n\ngo 1.20\n",
		"main.go": "package main\nimport \"fmt\"\nfunc main() {\n value := 41\n value++\n fmt.Println(value)\n}\n",
	} {
		if err := testutil.WriteFile(project+"/"+name, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	run := func(fixture string) {
		t.Helper()
		var output bytes.Buffer
		command := withEnvironment(p, map[string]string{
			"SELFISHELL_DAP_PROJECT":       project,
			"SELFISHELL_NVIM_TEST_FIXTURE": root + "/tests/fixtures/neovim/" + fixture,
			// Delve builds with Go. Keep its caches private and removable by TempDir.
			"GOPATH": home + "/go", "GOMODCACHE": home + "/go/pkg/mod",
			"GOCACHE": home + "/.cache/go-build", "GOFLAGS": "-modcacherw",
			"GOENV": "off", "GOWORK": "off", "GOTOOLCHAIN": "local",
		}, "VIRTUAL_ENV", "CONDA_PREFIX", "WSL_DISTRO_NAME")
		command.Dir, command.Out, command.Err = root+"/config/shared", &output, &output
		code, err := command.Run(ctx, mise, "-C", command.Dir, "exec", "--", nvim, "--headless", "+lua dofile(vim.env.SELFISHELL_NVIM_TEST_FIXTURE)", "+qa!")
		if err != nil || code != 0 || !strings.Contains(output.String(), "DAP consumer: OK") {
			t.Fatalf("%s: code=%d err=%v output=%s", fixture, code, err, output.String())
		}
		t.Logf("%s: DAP consumer passed", fixture)
	}
	run("dap_install_smoke.lua")
	before := map[string][]byte{}
	for _, name := range []string{"debugpy", "delve"} {
		path := home + "/.local/share/nvim/mason/packages/" + name + "/mason-receipt.json"
		before[path], err = os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := op.UpdateDefaultLSP(ctx, root, paths, false); err != nil {
		t.Fatal(err)
	}
	for path, want := range before {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("LSP sync changed user adapter %s: %v", path, err)
		}
	}
	run("dap_debug_smoke.lua")
}
