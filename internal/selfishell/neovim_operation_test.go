package selfishell

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func neovimFixture(t *testing.T) (*PackageOperation, Paths, string, string, string, string) {
	t.Helper()
	op, paths, manifest, home := dependencyFixture(t)
	root := home + "/release"
	writeTestFile(t, root+"/config/shared/mise.toml", "[tools]\n", 0600)
	repo := home + "/lazy-source"
	if err := os.MkdirAll(repo, 0700); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, repo, "init", "--quiet")
	writeTestFile(t, repo+"/init.lua", "return true\n", 0600)
	gitCommand(t, repo, "add", ".")
	gitCommand(t, repo, "commit", "--quiet", "-m", "initial")
	head := gitCommand(t, repo, "rev-parse", "HEAD")
	writeTestFile(t, manifest, fmt.Sprintf("nvim-plugin folke/lazy.nvim %s all all %s - - -\nnvim-plugin demo/one %s all all %s - - -\n", head, repo, head, repo), 0600)
	bin := home + "/bin"
	writeTestFile(t, bin+"/nvim", `#!/bin/sh
printf '%s\n' "$*" >> "$NVIM_LOG"
case "$*" in *'Lazy! sync'*) mkdir -p "$XDG_DATA_HOME/nvim/lazy"; git clone -q "$LAZY_SOURCE" "$XDG_DATA_HOME/nvim/lazy/lazy-source" ;; esac
case "$*" in *nvim-treesitter*) exit "${PARSER_EXIT:-0}" ;; esac
`, 0755)
	t.Setenv("NVIM_LOG", home+"/nvim.log")
	t.Setenv("LAZY_SOURCE", repo)
	op.Process.Env = append(os.Environ(), "HOME="+home, "XDG_DATA_HOME="+home+"/data", "PATH="+bin+":/usr/bin:/bin", "NVIM_LOG="+home+"/nvim.log", "LAZY_SOURCE="+repo)
	return op, paths, root, manifest, home, head
}

func TestNeovimBootstrapsSyncsAndSkipsMatchingPins(t *testing.T) {
	op, paths, root, manifest, home, head := neovimFixture(t)
	if err := op.InstallNeovimPlugins(context.Background(), root, paths, manifest, false); err != nil {
		t.Fatal(err)
	}
	for _, checkout := range []string{home + "/data/selfishell/nvim/lazy/lazy.nvim", home + "/data/nvim/lazy/lazy-source"} {
		if got := gitCommand(t, checkout, "rev-parse", "HEAD"); got != head {
			t.Fatalf("%s: %s", checkout, got)
		}
	}
	data, _ := os.ReadFile(home + "/nvim.log")
	if strings.Count(string(data), "Lazy! sync") != 1 || strings.Count(string(data), "nvim-treesitter") != 1 {
		t.Fatalf("nvim calls: %s", data)
	}
	if strings.Count(string(data), "\n") != 2 {
		t.Fatalf("unexpected Neovim call count: %s", data)
	}
	if origin := gitCommand(t, home+"/data/selfishell/nvim/lazy/lazy.nvim", "remote", "get-url", "origin"); origin != home+"/lazy-source" {
		t.Fatalf("lazy cloned unapproved source: %s", origin)
	}
	if err := op.InstallNeovimPlugins(context.Background(), root, paths, manifest, false); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(home + "/nvim.log")
	if strings.Count(string(data), "Lazy! sync") != 1 || strings.Count(string(data), "nvim-treesitter") != 2 {
		t.Fatalf("unnecessary sync: %s", data)
	}
	if err := os.Mkdir(home+"/data/nvim/lazy/removed-plugin", 0700); err != nil {
		t.Fatal(err)
	}
	if err := op.InstallNeovimPlugins(context.Background(), root, paths, manifest, false); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(home + "/nvim.log")
	if strings.Count(string(data), "Lazy! sync") != 2 {
		t.Fatalf("extra plugin did not trigger sync: %s", data)
	}
}

func TestNeovimPreservesDirtyCheckout(t *testing.T) {
	op, paths, root, manifest, home, _ := neovimFixture(t)
	target := home + "/data/nvim/lazy/lazy-source"
	if err := os.MkdirAll(home+"/data/nvim/lazy", 0700); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, home, "clone", "-q", home+"/lazy-source", target)
	writeTestFile(t, target+"/init.lua", "changed\n", 0600)
	err := op.InstallNeovimPlugins(context.Background(), root, paths, manifest, false)
	if err == nil || !strings.Contains(err.Error(), "modified") {
		t.Fatalf("dirty checkout: %v", err)
	}
	if data, _ := os.ReadFile(target + "/init.lua"); string(data) != "changed\n" {
		t.Fatalf("changed user checkout: %q", data)
	}
	if _, err := os.Stat(home + "/nvim.log"); !os.IsNotExist(err) {
		t.Fatalf("ran nvim: %v", err)
	}
}

func TestNeovimReportsConcurrentCheckFailuresInDeclarationOrder(t *testing.T) {
	op, paths, root, manifest, home, head := neovimFixture(t)
	lines := fmt.Sprintf("nvim-plugin folke/lazy.nvim %s all all %s/lazy-source - - -\n", head, home)
	var plugins []string
	for i := range 2*probeLimit + 2 {
		name := fmt.Sprintf("plugin%02d", i)
		lines += fmt.Sprintf("nvim-plugin demo/%s %s all all %s/sources/%s - - -\n", name, head, home, name)
		plugins = append(plugins, home+"/data/nvim/lazy/"+name)
		gitCommand(t, home, "clone", "-q", home+"/lazy-source", plugins[i])
	}
	writeTestFile(t, manifest, lines, 0600)
	if err := op.InstallNeovimPlugins(context.Background(), root, paths, manifest, false); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(home + "/nvim.log"); err != nil {
		t.Fatal(err)
	}
	// Earlier failures win regardless of which concurrent check finishes first.
	writeTestFile(t, plugins[3]+"/init.lua", "dirty 3\n", 0600)
	if err := os.RemoveAll(plugins[6] + "/.git"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(plugins[6]+"/.git", 0700); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, plugins[9]+"/init.lua", "dirty 9\n", 0600)
	if err := os.RemoveAll(plugins[12]); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, plugins[12], "user data", 0600)
	lazy := home + "/data/selfishell/nvim/lazy/lazy.nvim"
	writeTestFile(t, lazy+"/user-file", "keep", 0600)
	stdin := strings.NewReader("terminal input")
	op.Process.In = stdin
	for _, step := range []struct {
		want string
		fix  func()
	}{
		{"lazy.nvim checkout was modified; preserving it: " + lazy, func() { os.Remove(lazy + "/user-file") }},
		{"Neovim plugin checkout was modified; preserving it: " + plugins[3] + ".", func() { gitCommand(t, plugins[3], "checkout", "--", ".") }},
		{"Could not inspect Neovim plugin checkout: " + plugins[6] + ":", func() { os.RemoveAll(plugins[6]) }},
		{"Neovim plugin checkout was modified; preserving it: " + plugins[9] + ".", func() { gitCommand(t, plugins[9], "checkout", "--", ".") }},
		{"Neovim plugin path is not an approved Git checkout; preserving it: " + plugins[12], nil},
	} {
		for range 3 {
			err := op.InstallNeovimPlugins(context.Background(), root, paths, manifest, false)
			if err == nil || !strings.Contains(err.Error(), step.want) {
				t.Fatalf("want %q, got %v", step.want, err)
			}
		}
		if step.fix != nil {
			step.fix()
		}
	}
	if _, err := os.Stat(home + "/nvim.log"); !os.IsNotExist(err) {
		t.Fatalf("ran nvim despite a failed check: %v", err)
	}
	if got := readTestFile(t, plugins[12]); got != "user data" {
		t.Fatalf("preserved path changed: %q", got)
	}
	if stdin.Len() != len("terminal input") {
		t.Fatal("a concurrent check read the caller's stdin")
	}
}

func TestNeovimDryRunAndMissingBinaryDoNotMutate(t *testing.T) {
	op, paths, root, manifest, home, _ := neovimFixture(t)
	if err := op.InstallNeovimPlugins(context.Background(), root, paths, manifest, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(home + "/data/selfishell/nvim/lazy/lazy.nvim"); !os.IsNotExist(err) {
		t.Fatalf("dry-run mutated: %v", err)
	}
	if _, err := os.Stat(home + "/nvim.log"); !os.IsNotExist(err) {
		t.Fatalf("dry-run invoked Neovim: %v", err)
	}
	plan := op.Process.Out.(*bytes.Buffer).String()
	for _, line := range []string{"Would sync declared Neovim plugins.", "Would sync lazy.nvim bootstrap repository.", "Would update installed Tree-sitter parsers."} {
		if !strings.Contains(plan, line) {
			t.Fatalf("dry-run omitted %q: %s", line, plan)
		}
	}
	if err := os.Remove(home + "/bin/nvim"); err != nil {
		t.Fatal(err)
	}
	if err := op.InstallNeovimPlugins(context.Background(), root, paths, manifest, false); err == nil || !strings.Contains(err.Error(), "Could not locate Neovim") {
		t.Fatalf("missing nvim: %v", err)
	}
	if _, err := os.Stat(home + "/data/selfishell/nvim/lazy/lazy.nvim"); !os.IsNotExist(err) {
		t.Fatalf("missing nvim cloned: %v", err)
	}
}

func TestNeovimCancelledBeforeWork(t *testing.T) {
	op, paths, root, manifest, home, _ := neovimFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := op.InstallNeovimPlugins(ctx, root, paths, manifest, false); err != context.Canceled {
		t.Fatalf("cancellation: %v", err)
	}
	if _, err := os.Stat(home + "/data/selfishell/nvim/lazy/lazy.nvim"); !os.IsNotExist(err) {
		t.Fatalf("cancelled install mutated: %v", err)
	}
}

func TestNeovimParserFailureWarns(t *testing.T) {
	op, paths, root, manifest, _, _ := neovimFixture(t)
	op.Process.Env = append(op.Process.Env, "PARSER_EXIT=1")
	if err := op.InstallNeovimPlugins(context.Background(), root, paths, manifest, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(op.Process.Err.(*bytes.Buffer).String(), "Could not update Tree-sitter parsers") {
		t.Fatal("missing warning")
	}
}

func TestLazyRevisionUpdateReportsAndPreservesStaleTemporaryPath(t *testing.T) {
	op, paths, _, manifest, home, _ := neovimFixture(t)
	deps, err := ReadDependencies(manifest)
	if err != nil {
		t.Fatal(err)
	}
	target := home + "/data/selfishell/nvim/lazy/lazy.nvim"
	if err := os.MkdirAll(home+"/data/selfishell/nvim/lazy", 0700); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, home, "clone", "-q", home+"/lazy-source", target)
	writeTestFile(t, home+"/lazy-source/init.lua", "next\n", 0600)
	gitCommand(t, home+"/lazy-source", "add", ".")
	gitCommand(t, home+"/lazy-source", "commit", "-qm", "next")
	newHead := gitCommand(t, home+"/lazy-source", "rev-parse", "HEAD")
	dep := deps[0]
	dep.Version = newHead
	stale := target + ".tmp.stale"
	writeTestFile(t, stale+"/marker", "stale", 0600)
	if err := op.installLazy(context.Background(), paths, dep, nil); err != nil {
		t.Fatal(err)
	}
	if got := gitCommand(t, target, "rev-parse", "HEAD"); got != newHead {
		t.Fatalf("revision: %s", got)
	}
	if got, _ := os.ReadFile(stale + "/marker"); string(got) != "stale" {
		t.Fatalf("stale path overwritten: %q", got)
	}
	if !strings.Contains(op.Process.Out.(*bytes.Buffer).String(), "Updated approved lazy.nvim revision") {
		t.Fatal("update not reported")
	}
}

func TestLazyPreviousCheckoutCleanupFailureWarnsAfterActivation(t *testing.T) {
	op, paths, _, manifest, home, _ := neovimFixture(t)
	deps, err := ReadDependencies(manifest)
	if err != nil {
		t.Fatal(err)
	}
	target := home + "/data/selfishell/nvim/lazy/lazy.nvim"
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, home, "clone", "-q", home+"/lazy-source", target)
	writeTestFile(t, home+"/lazy-source/init.lua", "next\n", 0600)
	gitCommand(t, home+"/lazy-source", "add", ".")
	gitCommand(t, home+"/lazy-source", "commit", "-qm", "next")
	dep := deps[0]
	dep.Version = gitCommand(t, home+"/lazy-source", "rev-parse", "HEAD")
	op.lazyRemoveAll = func(path string) error {
		if strings.Contains(path, ".previous.") {
			return errors.New("injected cleanup failure")
		}
		return os.RemoveAll(path)
	}
	if err := op.installLazy(context.Background(), paths, dep, nil); err != nil {
		t.Fatal(err)
	}
	if got := gitCommand(t, target, "rev-parse", "HEAD"); got != dep.Version {
		t.Fatalf("activation did not complete: %s", got)
	}
	if warning := op.Process.Err.(*bytes.Buffer).String(); !strings.Contains(warning, "injected cleanup failure") || !strings.Contains(warning, "previous") {
		t.Fatalf("missing previous-checkout cleanup warning: %q", warning)
	}
}

func TestLazyStageCleanupFailureWarnsAlongsideCheckoutFailure(t *testing.T) {
	op, paths, _, manifest, _, _ := neovimFixture(t)
	deps, err := ReadDependencies(manifest)
	if err != nil {
		t.Fatal(err)
	}
	dep := deps[0]
	dep.Version = strings.Repeat("0", 40)
	op.lazyRemoveAll = func(path string) error {
		if strings.Contains(path, ".tmp.") {
			return errors.New("injected stage cleanup failure")
		}
		return os.RemoveAll(path)
	}
	if err := op.installLazy(context.Background(), paths, dep, nil); err == nil {
		t.Fatal("unavailable approved revision accepted")
	}
	if warning := op.Process.Err.(*bytes.Buffer).String(); !strings.Contains(warning, "injected stage cleanup failure") || !strings.Contains(warning, "staging path") {
		t.Fatalf("missing stage cleanup warning: %q", warning)
	}
}

func TestLazyRestoreFailureIncludesPrimaryActivationError(t *testing.T) {
	home := t.TempDir()
	old, target := home+"/previous", home+"/lazy.nvim"
	writeTestFile(t, old, "previous", 0600)
	writeTestFile(t, target, "occupied", 0600)
	primary := errors.New("activation failed")
	err := restoreLazyOnFailure(old, target, primary)
	if !errors.Is(err, primary) || !strings.Contains(err.Error(), "cannot restore over occupied target") {
		t.Fatalf("activation or restoration error lost: %v", err)
	}
	if data, readErr := os.ReadFile(old); readErr != nil || string(data) != "previous" {
		t.Fatalf("previous checkout changed: %q %v", data, readErr)
	}
}

func TestLazyRejectsDirtyMalformedAndBlockedParent(t *testing.T) {
	op, paths, _, manifest, home, _ := neovimFixture(t)
	deps, err := ReadDependencies(manifest)
	if err != nil {
		t.Fatal(err)
	}
	target := home + "/data/selfishell/nvim/lazy/lazy.nvim"
	writeTestFile(t, target, "user data", 0600)
	if err := op.installLazy(context.Background(), paths, deps[0], nil); err == nil || !strings.Contains(err.Error(), "preserving") {
		t.Fatalf("malformed target accepted: %v", err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, home, "clone", "-q", home+"/lazy-source", target)
	writeTestFile(t, target+"/init.lua", "dirty\n", 0600)
	if err := op.installLazy(context.Background(), paths, deps[0], nil); err == nil || !strings.Contains(err.Error(), "modified") {
		t.Fatalf("dirty lazy accepted: %v", err)
	}
	if got, _ := os.ReadFile(target + "/init.lua"); string(got) != "dirty\n" {
		t.Fatalf("dirty file changed: %q", got)
	}
	if err := os.RemoveAll(home + "/data/selfishell/nvim"); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, home+"/data/selfishell/nvim/lazy", "blocked", 0600)
	if err := op.installLazy(context.Background(), paths, deps[0], nil); err == nil || !strings.Contains(err.Error(), "Could not create Neovim plugin directory") {
		t.Fatalf("blocked parent: %v", err)
	}
}

func TestNeovimUsesManagedMiseResolutionAndExec(t *testing.T) {
	op, paths, root, manifest, home, _ := neovimFixture(t)
	if err := os.MkdirAll(home+"/.local/bin", 0700); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, home+"/.local/bin/mise", `#!/bin/sh
printf '%s|%s|%s|%s\n' "$PWD" "$MISE_GLOBAL_CONFIG_FILE" "$(printf '%s' "$*" | tr '\n' ' ')" "$HOME" >> "$MISE_LOG"
if [ "$3" = which ]; then printf '%s\n' "$MANAGED_NVIM"; exit 0; fi
if [ "$3" = exec ]; then shift 4; "$@"; exit; fi
`, 0755)
	managedNvim := home + "/managed/nvim"
	writeTestFile(t, managedNvim, `#!/bin/sh
printf '%s\n' "$*" >> "$NVIM_LOG"
case "$*" in *'Lazy! sync'*) mkdir -p "$XDG_DATA_HOME/nvim/lazy"; git clone -q "$LAZY_SOURCE" "$XDG_DATA_HOME/nvim/lazy/lazy-source" ;; esac
`, 0755)
	op.Process.Env = append(os.Environ(), "HOME="+home, "XDG_DATA_HOME="+home+"/data", "PATH=/usr/bin:/bin", "MISE_LOG="+home+"/mise.log", "MANAGED_NVIM="+managedNvim, "NVIM_LOG="+home+"/nvim.log", "LAZY_SOURCE="+home+"/lazy-source")
	if err := op.InstallNeovimPlugins(context.Background(), root, paths, manifest, false); err != nil {
		t.Fatal(err)
	}
	if err := op.UpdateDefaultLSP(context.Background(), root, paths, false); err != nil {
		t.Fatal(err)
	}
	log, _ := os.ReadFile(home + "/mise.log")
	if strings.Count(string(log), "which nvim") != 1 || strings.Count(string(log), "exec -- "+managedNvim) != 3 || !strings.Contains(string(log), root+"/config/shared/mise.toml") {
		t.Fatalf("mise resolution/exec: %s", log)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(log)), "\n") {
		if !strings.HasSuffix(strings.Split(line, "|")[0], "/release/config/shared") {
			t.Fatalf("inherited caller project cwd: %s", line)
		}
	}
}

func TestNeovimMiseResolutionReusesOnlyAnExecutableSuccess(t *testing.T) {
	op, paths, root, _, home, _ := neovimFixture(t)
	writeTestFile(t, home+"/.local/bin/mise", `#!/bin/sh
printf '%s\n' "$*" >> "$MISE_LOG"
[ "$3" = which ] && [ -f "$HOME/which-ok" ] && printf '%s\n' "$MANAGED_NVIM"
[ -f "$HOME/which-ok" ]
`, 0755)
	managed := home + "/managed/nvim"
	writeTestFile(t, managed, "#!/bin/sh\n", 0755)
	op.Process.Env = append(op.Process.Env, "MISE_LOG="+home+"/mise.log", "MANAGED_NVIM="+managed)
	resolve := func(want string, calls int) {
		t.Helper()
		nvim, _, err := op.nvimCommand(context.Background(), root, paths)
		if err != nil || nvim != want {
			t.Fatalf("resolved %q %v, want %q", nvim, err, want)
		}
		if got := strings.Count(readTestFile(t, home+"/mise.log"), "which nvim"); got != calls {
			t.Fatalf("which calls %d, want %d", got, calls)
		}
	}
	resolve(home+"/bin/nvim", 1)
	resolve(home+"/bin/nvim", 2) // a failed resolution is retried
	writeTestFile(t, home+"/which-ok", "", 0600)
	resolve(managed, 3)
	resolve(managed, 3)
	if err := os.Remove(managed); err != nil {
		t.Fatal(err)
	}
	resolve(home+"/bin/nvim", 4)
}

func TestNeovimVerificationFailureSurfacesSyncLog(t *testing.T) {
	op, paths, root, manifest, home, _ := neovimFixture(t)
	writeTestFile(t, home+"/bin/nvim", "#!/bin/sh\nprintf 'sync diagnostic\\n'\n", 0755)
	err := op.InstallNeovimPlugins(context.Background(), root, paths, manifest, false)
	if err == nil || !strings.Contains(err.Error(), "missing after sync") {
		t.Fatalf("verification: %v", err)
	}
	if !strings.Contains(op.Process.Err.(*bytes.Buffer).String(), "sync diagnostic") {
		t.Fatal("sync log hidden on verification failure")
	}
}

func TestNeovimPreservesMalformedExistingPluginPath(t *testing.T) {
	op, paths, root, manifest, home, _ := neovimFixture(t)
	target := home + "/data/nvim/lazy/lazy-source"
	writeTestFile(t, target, "user data", 0600)
	err := op.InstallNeovimPlugins(context.Background(), root, paths, manifest, false)
	if err == nil || !strings.Contains(err.Error(), "preserving") {
		t.Fatalf("malformed plugin accepted: %v", err)
	}
	if data, _ := os.ReadFile(target); string(data) != "user data" {
		t.Fatalf("user path changed: %q", data)
	}
	if _, err := os.Stat(home + "/nvim.log"); !os.IsNotExist(err) {
		t.Fatalf("ran nvim with malformed path: %v", err)
	}
}

func TestLazyRejectsGitDirectorySymlink(t *testing.T) {
	op, paths, _, manifest, home, _ := neovimFixture(t)
	deps, err := ReadDependencies(manifest)
	if err != nil {
		t.Fatal(err)
	}
	target := home + "/data/selfishell/nvim/lazy/lazy.nvim"
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(home+"/lazy-source/.git", target+"/.git"); err != nil {
		t.Fatal(err)
	}
	if err := op.installLazy(context.Background(), paths, deps[0], nil); err == nil || !strings.Contains(err.Error(), "preserving") {
		t.Fatalf("linked git metadata accepted: %v", err)
	}
}

func TestLazyPreservesUntrackedUserFile(t *testing.T) {
	op, paths, _, manifest, home, _ := neovimFixture(t)
	deps, err := ReadDependencies(manifest)
	if err != nil {
		t.Fatal(err)
	}
	target := home + "/data/selfishell/nvim/lazy/lazy.nvim"
	if err := os.MkdirAll(home+"/data/selfishell/nvim/lazy", 0700); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, home, "clone", "-q", home+"/lazy-source", target)
	writeTestFile(t, target+"/user-file", "keep", 0600)
	if err := op.installLazy(context.Background(), paths, deps[0], nil); err == nil || !strings.Contains(err.Error(), "modified") {
		t.Fatalf("untracked file accepted: %v", err)
	}
	if data, _ := os.ReadFile(target + "/user-file"); string(data) != "keep" {
		t.Fatalf("untracked file lost: %q", data)
	}
}
