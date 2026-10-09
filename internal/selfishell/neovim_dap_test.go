package selfishell

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/url"
	"os"
	"os/exec"
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
	for _, name := range []string{"project/.venv/bin/python", "active/bin/python", "conda/bin/python", "other/.venv/bin/python", "bin/python3", "bin/js-debug-adapter"} {
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
		"main.py":         "value = 41\nvalue += 1\nprint(value)\n",
		"go.mod":          "module debugfixture\n\ngo 1.20\n",
		"main.go":         "package main\nimport \"fmt\"\nfunc main() {\n value := 41\n value++\n fmt.Println(value)\n}\n",
		"main.cjs":        "let value = 41;\nvalue += 1;\nconsole.log(value);\nvalue += 1;\nconsole.log(value);\n",
		"main.ts":         "type Count = number;\nlet value: Count = 41;\nvalue += 1;\nconsole.log(value);\nvalue += 1;\nconsole.log(value);\n",
		"mapped.ts":       "type Count = number;\nlet value: Count = 41;\nvalue += 1;\nconsole.log(value);\nvalue += 1;\nconsole.log(value);\n",
		"dist/mapped.cjs": "let value = 41;\nvalue += 1;\nconsole.log(value);\nvalue += 1;\nconsole.log(value);\n//# sourceMappingURL=mapped.cjs.map\n",
		// Each generated line maps to the next TS line, after the erased type alias.
		"dist/mapped.cjs.map": `{"version":3,"file":"mapped.cjs","sources":["../mapped.ts"],"names":[],"mappings":"AACA;AACA;AACA;AACA;AACA"}`,
		"attach.cjs":          "let value = 41;\nvalue += 1;\nconsole.log(value);\nsetInterval(() => {}, 1000);\n",
		"input.cjs":           "const rl = require('node:readline').createInterface({ input: process.stdin, output: process.stdout });\nrl.question('Value? ', (answer) => {\n  const value = Number(answer) + 1;\n  console.log(value);\n  rl.close();\n});\n",
		"input.ts":            "const rl = require('node:readline').createInterface({ input: process.stdin, output: process.stdout });\nrl.question('Value? ', (answer: string) => {\n  const value: number = Number(answer) + 1;\n  console.log(value);\n  rl.close();\n});\n",
	} {
		if err := os.MkdirAll(filepath.Dir(project+"/"+name), 0700); err != nil {
			t.Fatal(err)
		}
		if err := testutil.WriteFile(project+"/"+name, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	inspectorPort := ""
	run := func(fixture string) {
		t.Helper()
		var output bytes.Buffer
		command := withEnvironment(p, map[string]string{
			"SELFISHELL_DAP_PROJECT":         project,
			"SELFISHELL_NVIM_TEST_FIXTURE":   root + "/tests/fixtures/neovim/" + fixture,
			"SELFISHELL_NODE_INSPECTOR_PORT": inspectorPort,
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
	for _, name := range []string{"debugpy", "delve", "js-debug-adapter"} {
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
	if err := os.MkdirAll(project+"/.vscode", 0700); err != nil {
		t.Fatal(err)
	}
	launch := `{"version":"0.2.0","configurations":[{"name":"TS project launch","type":"node","request":"launch","program":"${workspaceFolder}/dist/mapped.cjs","cwd":"${workspaceFolder}","outFiles":["${workspaceFolder}/dist/**/*.cjs"]}]}`
	if err := testutil.WriteFile(project+"/.vscode/launch.json", []byte(launch), 0600); err != nil {
		t.Fatal(err)
	}
	// Go owns the attach target and cleanup; the Lua probe exercises Neovim.
	node, err := op.miseOutput(ctx, p, mise, "-C", root+"/config/shared", "which", "node")
	if err != nil {
		t.Fatal(err)
	}
	attached := exec.CommandContext(ctx, node, "--inspect-brk=127.0.0.1:0", project+"/attach.cjs")
	attached.Env, attached.Dir = p.environment(), project
	stderr, err := attached.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := attached.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = attached.Process.Kill(); _ = attached.Wait() }()
	ready := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			if endpoint, found := strings.CutPrefix(scanner.Text(), "Debugger listening on "); found {
				if parsed, err := url.Parse(endpoint); err == nil {
					ready <- parsed.Port()
					_, _ = io.Copy(io.Discard, stderr)
					return
				}
			}
		}
		ready <- ""
	}()
	select {
	case inspectorPort = <-ready:
		if inspectorPort == "" {
			t.Fatal("Node inspector did not report a port")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Node inspector startup timed out")
	}
	run("dap_javascript_smoke.lua")
}
