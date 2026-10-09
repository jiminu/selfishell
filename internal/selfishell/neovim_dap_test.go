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
