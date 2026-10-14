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

func TestNeovimDAPLayout(t *testing.T) {
	t.Parallel()
	plugins := os.Getenv("SELFISHELL_TEST_NVIM_PLUGIN_ROOT")
	if plugins == "" {
		t.Skip("requires SELFISHELL_TEST_NVIM_PLUGIN_ROOT with approved plugin checkouts")
	}
	root := testRelease(t)
	nvim, ok := standaloneNeovimExecutable(t, root)
	if !ok {
		t.Skip("requires the pinned physical Neovim executable")
	}
	p := standaloneNeovimProcess(t, nvim)
	home := envValue(p.Env, "HOME")
	manifest, err := os.ReadFile(root + "/dependencies.conf")
	if err != nil {
		t.Fatal(err)
	}
	config := home + "/.config/nvim"
	if err := os.MkdirAll(config, 0700); err != nil {
		t.Fatal(err)
	}
	if err := testutil.WriteFile(config+"/plugin-versions.conf", manifest, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	deps, err := ReadDependencies(root + "/dependencies.conf")
	if err != nil {
		t.Fatal(err)
	}
	for _, dep := range deps {
		name := filepath.Base(dep.Name)
		if name == "nvim-tree.lua" || name == "nvim-dap" || name == "nvim-dap-ui" || name == "nvim-nio" {
			actual, err := (&PackageOperation{Process: p}).gitHead(ctx, filepath.Join(plugins, name))
			if err != nil || actual != dep.Version {
				t.Fatalf("plugin %s HEAD %s want %s: %v", dep.Name, actual, dep.Version, err)
			}
		}
	}
	p = withEnvironment(p, map[string]string{"SELFISHELL_TEST_NVIM_PLUGIN_ROOT": plugins}, "WSL_DISTRO_NAME")
	var output bytes.Buffer
	p.Out, p.Err, p.Dir = &output, &output, home
	code, err := p.Run(ctx, nvim, "--headless", "-u", "NONE", "-i", "NONE", "--cmd",
		"set runtimepath^="+root+"/config/shared/nvim", "+luafile "+root+"/tests/fixtures/neovim/dap_layout_smoke.lua", "+qa!")
	if err != nil || code != 0 || !strings.Contains(output.String(), "DAP layout smoke: OK") {
		t.Fatalf("DAP layout: code=%d err=%v output=%s", code, err, output.String())
	}
}
