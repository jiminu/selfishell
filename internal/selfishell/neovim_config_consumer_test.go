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
	nvim, ok := standaloneNeovimExecutable(t, root)
	if !ok {
		t.Skip("No physical Neovim matching the current mise pin on PATH or in the mise install directory")
	}
	runNeovimConfigFixtures(t, root, standaloneNeovimProcess(t, nvim), nvim, "")
}

func standaloneNeovimExecutable(t *testing.T, root string) (string, bool) {
	t.Helper()
	pins, err := approvedMisePins(root+"/config/shared/mise.toml", []string{"neovim"})
	if err != nil {
		t.Fatal(err)
	}
	version := strings.TrimPrefix(pins[0], "neovim@")
	data := envDefault("MISE_DATA_DIR", os.Getenv("HOME")+"/.local/share/mise")
	candidates := filepath.SplitList(os.Getenv("PATH"))
	candidates = append(candidates, data+"/installs/neovim/"+version+"/bin")
	for _, dir := range candidates {
		if dir == "" {
			continue
		}
		candidate, err := filepath.Abs(filepath.Join(dir, "nvim"))
		if err != nil || strings.Contains(candidate, "/shims/") {
			continue
		}
		physical, err := filepath.EvalSymlinks(candidate)
		if err != nil || strings.Contains(physical, "/shims/") {
			continue
		}
		info, err := os.Stat(physical)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		var out, stderr bytes.Buffer
		probe := standaloneNeovimProcess(t, physical)
		probe.Out, probe.Err = &out, &stderr
		code, err := probe.Run(ctx, physical, "--version")
		cancel()
		if err != nil || code != 0 {
			t.Fatalf("Neovim version probe %s: code=%d err=%v stderr=%q", physical, code, err, stderr.String())
		}
		first, _, _ := strings.Cut(out.String(), "\n")
		if first == "NVIM v"+version {
			return physical, true
		}
	}
	return "", false
}

func TestStandaloneNeovimSelectsPinnedPhysicalPathWithPrivateOuterHome(t *testing.T) {
	root := testRelease(t)
	pins, err := approvedMisePins(root+"/config/shared/mise.toml", []string{"neovim"})
	if err != nil {
		t.Fatal(err)
	}
	version := strings.TrimPrefix(pins[0], "neovim@")
	fixture := t.TempDir()
	home := fixture + "/outer-home"
	shimDir := fixture + "/shims"
	wrongDir := fixture + "/wrong-version"
	physical := fixture + "/installs/neovim/" + version + "/bin/nvim"
	for _, dir := range []string{home, shimDir, wrongDir, filepath.Dir(physical)} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	shimCalled := fixture + "/shim-called"
	if err := os.WriteFile(shimDir+"/nvim", []byte("#!/bin/sh\nprintf called >\"$SELFISHELL_NVIM_SELECTION_SHIM_LOG\"\nexit 99\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wrongDir+"/nvim", []byte("#!/bin/sh\nprintf 'NVIM v0.0.0\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	probeLog := fixture + "/probe-log"
	if err := os.WriteFile(physical, []byte("#!/bin/sh\nprintf '%s\\n%s\\n' \"$HOME\" \"$MISE_DATA_DIR\" >\"$SELFISHELL_NVIM_SELECTION_PROBE_LOG\"\nprintf 'NVIM v"+version+"\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("MISE_DATA_DIR", fixture+"/unused-mise-data")
	t.Setenv("MISE_CONFIG_DIR", fixture+"/unused-mise-config")
	t.Setenv("MISE_STATE_DIR", fixture+"/unused-mise-state")
	t.Setenv("MISE_CACHE_DIR", fixture+"/unused-mise-cache")
	t.Setenv("TMPDIR", t.TempDir())
	for _, key := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"} {
		t.Setenv(key, fixture+"/outer-"+key)
	}
	t.Setenv("PATH", shimDir+":"+wrongDir+":"+filepath.Dir(physical)+":/usr/bin:/bin")
	t.Setenv("SELFISHELL_NVIM_SELECTION_SHIM_LOG", shimCalled)
	t.Setenv("SELFISHELL_NVIM_SELECTION_PROBE_LOG", probeLog)
	if _, err := os.Stat(home + "/.local/share/mise/installs/neovim/" + version + "/bin/nvim"); !os.IsNotExist(err) {
		t.Fatalf("old HOME-based selection unexpectedly available: %v", err)
	}
	nvim, ok := standaloneNeovimExecutable(t, root)
	physicalResolved, err := filepath.EvalSymlinks(physical)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || nvim != physicalResolved {
		t.Fatalf("selected %q want physical %q (found=%t)", nvim, physicalResolved, ok)
	}
	if _, err := os.Lstat(shimCalled); !os.IsNotExist(err) {
		t.Fatalf("ambient shim invoked: %v", err)
	}
	data, err := os.ReadFile(probeLog)
	if err != nil || strings.Contains(string(data), home) || !strings.Contains(string(data), "/mise/data\n") {
		t.Fatalf("version probe used ambient environment: %q %v", data, err)
	}
}

func standaloneNeovimProcess(t *testing.T, nvim string) Process {
	t.Helper()
	root := t.TempDir()
	home := root + "/home"
	if err := os.MkdirAll(home+"/tmp", 0700); err != nil {
		t.Fatal(err)
	}
	var env []string
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "MISE_") {
			env = append(env, entry)
		}
	}
	return withEnvironment(Process{Env: env}, map[string]string{
		"HOME": home, "PATH": filepath.Dir(nvim) + ":/usr/bin:/bin",
		"XDG_CONFIG_HOME": home + "/.config", "XDG_DATA_HOME": home + "/.local/share",
		"XDG_STATE_HOME": home + "/.local/state", "XDG_CACHE_HOME": home + "/.cache", "TMPDIR": home + "/tmp",
		"MISE_CONFIG_DIR": home + "/mise/config", "MISE_DATA_DIR": home + "/mise/data",
		"MISE_STATE_DIR": home + "/mise/state", "MISE_CACHE_DIR": home + "/mise/cache", "MISE_OFFLINE": "1",
	})
}

func TestStandaloneNeovimFixtureIsolation(t *testing.T) {
	root := t.TempDir()
	bin := root + "/bin"
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	nvim := bin + "/nvim"
	if err := os.WriteFile(nvim, []byte("#!/bin/sh\nprintf '%s\\n%s\\n%s\\n%s\\n%s\\n%s\\n%s\\n%s\\n%s\\n' \"$HOME\" \"$MISE_GLOBAL_CONFIG_FILE\" \"$MISE_CONFIG_DIR\" \"$MISE_DATA_DIR\" \"$MISE_CACHE_DIR\" \"$MISE_STATE_DIR\" \"$MISE_OFFLINE\" \"$MISE_OVERRIDE_CONFIG_FILENAMES\" \"$PATH\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root+"/ambient", 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", root+"/ambient")
	t.Setenv("MISE_GLOBAL_CONFIG_FILE", "ambient-sentinel")
	t.Setenv("MISE_CONFIG_DIR", "ambient-sentinel")
	t.Setenv("MISE_OVERRIDE_CONFIG_FILENAMES", "ambient-sentinel")
	var out bytes.Buffer
	p := standaloneNeovimProcess(t, nvim)
	p.Out, p.Err = &out, &out
	code, err := p.Run(context.Background(), nvim)
	if err != nil || code != 0 {
		t.Fatalf("fake Neovim: %d %v", code, err)
	}
	home := envValue(p.Env, "HOME")
	want := strings.Join([]string{home, "", home + "/mise/config", home + "/mise/data", home + "/mise/cache", home + "/mise/state", "1", "", bin + ":/usr/bin:/bin"}, "\n") + "\n"
	if home == root+"/ambient" || !strings.HasSuffix(home, "/home") || out.String() != want {
		t.Fatalf("ambient mise or HOME reached Neovim: %q", out.String())
	}
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
