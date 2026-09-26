package selfishell

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestNeovimConfigFixtures(t *testing.T) {
	root := testRelease(t)
	nvim, err := (Process{}).lookPath("nvim")
	if err != nil {
		t.Skip("Neovim executable unavailable for offline native Lua fixtures")
	}
	runNeovimConfigFixtures(t, root, Process{Env: os.Environ()}, nvim, "")
}

// These remain native Lua behavior probes; Go owns isolation, process execution,
// fixture setup and assertions for every former neovim_config_test.bash case.
func runNeovimConfigFixtures(t *testing.T, root string, base Process, nvim, mise string) {
	t.Helper()
	home := t.TempDir()
	config := home + "/.config"
	data := home + "/.local/share"
	for _, dir := range []string{config + "/nvim", data, home + "/.local/state", home + "/.cache", home + "/tmp"} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	manifest := config + "/nvim/plugin-versions.conf"
	contents, err := os.ReadFile(root + "/dependencies.conf")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, contents, 0600); err != nil {
		t.Fatal(err)
	}
	base = withEnvironment(base, map[string]string{
		"HOME": home, "XDG_CONFIG_HOME": config, "XDG_DATA_HOME": data,
		"XDG_STATE_HOME": home + "/.local/state", "XDG_CACHE_HOME": home + "/.cache",
		"TMPDIR": home + "/tmp", "NVIM_LOG_FILE": home + "/nvim.log",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	run := func(fixture, revision string) string {
		t.Helper()
		file := root + "/tests/fixtures/neovim/" + fixture
		command := withEnvironment(base, map[string]string{"SELFISHELL_NVIM_TEST_FIXTURE": file, "SELFISHELL_TEST_REVISION": revision})
		command.Dir = root + "/config/shared"
		var result bytes.Buffer
		command.Out, command.Err = &result, &result
		args := []string{"--headless", "-u", "NONE", "-i", "NONE", "--cmd", "set runtimepath^=" + root + "/config/shared/nvim", "+lua dofile(vim.env.SELFISHELL_NVIM_TEST_FIXTURE)", "+qa"}
		name := nvim
		if mise != "" {
			name = mise
			args = append([]string{"-C", command.Dir, "exec", "--", nvim}, args...)
		}
		code, err := command.Run(ctx, name, args...)
		if err != nil || code != 0 {
			t.Fatalf("%s: code=%d err=%v output=%s", fixture, code, err, result.String())
		}
		return strings.ReplaceAll(result.String(), "\r", "")
	}
	for _, item := range []struct{ fixture, marker string }{
		{"treesitter_autocmd.lua", "sh\nterraform"},
		{"treesitter_auto_install.lua", "Tree-sitter auto-install: OK"},
		{"pinned_plugin_specs.lua", "pinned plugin specs: OK"},
		{"editor_workflow.lua", "editor workflows: OK"},
		{"lsp_mason_setup.lua", "LSP Mason setup: OK"},
		{"cursor_restore.lua", "cursor restore targeting: OK"},
		{"yank_highlight.lua", "yank highlight: OK"},
		{"ssh_clipboard.lua", "SSH clipboard: OK"},
	} {
		if output := run(item.fixture, ""); !strings.Contains(output, item.marker) {
			t.Fatalf("%s marker missing: %s", item.fixture, output)
		}
	}
	if err := os.Remove(manifest); err != nil {
		t.Fatal(err)
	}
	if output := run("plugin_versions_missing_manifest.lua", ""); !strings.Contains(output, "plugin_versions missing manifest: OK") {
		t.Fatalf("missing manifest case: %s", output)
	}
	if err := os.WriteFile(manifest, contents, 0600); err != nil {
		t.Fatal(err)
	}
	deps, err := ReadDependencies(root + "/dependencies.conf")
	if err != nil {
		t.Fatal(err)
	}
	var lazyRevision string
	configured := map[string]string{}
	seen := map[string]bool{}
	for _, dep := range deps {
		if dep.Kind != "nvim-plugin" {
			continue
		}
		if seen[dep.Name] {
			t.Fatalf("duplicate Neovim pin: %s", dep.Name)
		}
		seen[dep.Name] = true
		if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(dep.Version) {
			t.Fatalf("unapproved Neovim revision: %s %s", dep.Name, dep.Version)
		}
		if dep.Name == "folke/lazy.nvim" {
			lazyRevision = dep.Version
		} else {
			configured[dep.Name] = dep.Version
		}
	}
	if lazyRevision == "" {
		t.Fatal("lazy.nvim revision missing")
	}
	declared := map[string]bool{}
	modules, err := filepath.Glob(root + "/config/shared/nvim/lua/plugins/*.lua")
	if err != nil {
		t.Fatal(err)
	}
	pattern := regexp.MustCompile(`plugin\("([^"]+)"`)
	for _, module := range modules {
		data, err := os.ReadFile(module)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range pattern.FindAllStringSubmatch(string(data), -1) {
			declared[match[1]] = true
		}
	}
	if len(declared) == 0 || len(declared) != len(configured) {
		t.Fatalf("Neovim plugin membership: declared=%d pinned=%d", len(declared), len(configured))
	}
	for name := range declared {
		if _, ok := configured[name]; !ok {
			t.Fatalf("Neovim plugin lacks pin: %s", name)
		}
	}
	for name := range configured {
		if !declared[name] {
			t.Fatalf("undeclared Neovim pin: %s", name)
		}
	}
	lazyPath := data + "/selfishell/nvim/lazy/lazy.nvim"
	if err := os.MkdirAll(lazyPath+"/.git", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lazyPath+"/.git/HEAD", []byte(lazyRevision+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if output := run("lazy_detached_head.lua", lazyRevision); !strings.Contains(output, "true") {
		t.Fatalf("detached HEAD: %s", output)
	}
	if err := os.RemoveAll(lazyPath); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(lazyPath, 0700); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) string {
		t.Helper()
		command := base
		command.Dir = lazyPath
		var result bytes.Buffer
		command.Out, command.Err = &result, &result
		code, err := command.Run(ctx, "git", args...)
		if err != nil || code != 0 {
			t.Fatalf("git %v: code=%d err=%v output=%s", args, code, err, result.String())
		}
		return strings.TrimSpace(result.String())
	}
	git("init", "--quiet")
	git("-c", "user.name=Selfishell", "-c", "user.email=selfishell@example.invalid", "commit", "--allow-empty", "--quiet", "--message", "test")
	revision := git("rev-parse", "HEAD")
	head, err := os.ReadFile(lazyPath + "/.git/HEAD")
	if err != nil || !strings.HasPrefix(string(head), "ref: refs/heads/") {
		t.Fatalf("symbolic HEAD fixture invalid: %s %v", head, err)
	}
	if output := run("lazy_symbolic_head.lua", revision); !strings.Contains(output, "true") {
		t.Fatalf("symbolic HEAD: %s", output)
	}
}
