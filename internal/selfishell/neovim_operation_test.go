package selfishell

import (
	"bytes"
	"context"
	"fmt"
	"os"
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
	if err := op.installLazy(context.Background(), paths, dep); err != nil {
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

func TestLazyRejectsDirtyMalformedAndBlockedParent(t *testing.T) {
	op, paths, _, manifest, home, _ := neovimFixture(t)
	deps, err := ReadDependencies(manifest)
	if err != nil {
		t.Fatal(err)
	}
	target := home + "/data/selfishell/nvim/lazy/lazy.nvim"
	writeTestFile(t, target, "user data", 0600)
	if err := op.installLazy(context.Background(), paths, deps[0]); err == nil || !strings.Contains(err.Error(), "preserving") {
		t.Fatalf("malformed target accepted: %v", err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, home, "clone", "-q", home+"/lazy-source", target)
	writeTestFile(t, target+"/init.lua", "dirty\n", 0600)
	if err := op.installLazy(context.Background(), paths, deps[0]); err == nil || !strings.Contains(err.Error(), "modified") {
		t.Fatalf("dirty lazy accepted: %v", err)
	}
	if got, _ := os.ReadFile(target + "/init.lua"); string(got) != "dirty\n" {
		t.Fatalf("dirty file changed: %q", got)
	}
	if err := os.RemoveAll(home + "/data/selfishell/nvim"); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, home+"/data/selfishell/nvim/lazy", "blocked", 0600)
	if err := op.installLazy(context.Background(), paths, deps[0]); err == nil || !strings.Contains(err.Error(), "Could not create Neovim plugin directory") {
		t.Fatalf("blocked parent: %v", err)
	}
}

func TestNeovimUsesManagedMiseResolutionAndExec(t *testing.T) {
	op, paths, root, manifest, home, _ := neovimFixture(t)
	if err := os.MkdirAll(home+"/.local/bin", 0700); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, home+"/.local/bin/mise", `#!/bin/sh
printf '%s|%s|%s|%s\n' "$PWD" "$MISE_GLOBAL_CONFIG_FILE" "$*" "$HOME" >> "$MISE_LOG"
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
	log, _ := os.ReadFile(home + "/mise.log")
	if !strings.Contains(string(log), "which nvim") || strings.Count(string(log), "exec -- "+managedNvim) != 2 || !strings.Contains(string(log), root+"/config/shared/mise.toml") {
		t.Fatalf("mise resolution/exec: %s", log)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(log)), "\n") {
		if !strings.HasSuffix(strings.Split(line, "|")[0], "/release/config/shared") {
			t.Fatalf("inherited caller project cwd: %s", line)
		}
	}
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
	if err := op.installLazy(context.Background(), paths, deps[0]); err == nil || !strings.Contains(err.Error(), "preserving") {
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
	if err := op.installLazy(context.Background(), paths, deps[0]); err == nil || !strings.Contains(err.Error(), "modified") {
		t.Fatalf("untracked file accepted: %v", err)
	}
	if data, _ := os.ReadFile(target + "/user-file"); string(data) != "keep" {
		t.Fatalf("untracked file lost: %q", data)
	}
}
