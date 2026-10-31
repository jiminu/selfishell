package selfishell

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jiminu/selfishell/internal/managed"
	"github.com/jiminu/selfishell/internal/process"
	"github.com/jiminu/selfishell/internal/testutil"
)

func pruneFixture(t *testing.T, env *process.Environment) (*PackageOperation, managed.Paths, string, string, []Package) {
	t.Helper()
	op, paths, _, home := dependencyFixture(t, env)
	root := home + "/selfishell/releases/2.0.0"
	for _, version := range []string{"1.0.0", "2.0.0"} {
		dir := home + "/selfishell/releases/" + version
		testutil.WriteTestFile(t, dir+"/VERSION", version+"\n", 0600)
		testutil.WriteTestFile(t, dir+"/bin/selfishell", "#!/bin/sh\n", 0755)
		testutil.WriteTestFile(t, dir+"/config/shared/mise.toml", "[tools]\nnode = \"24.18.0\"\n", 0600)
	}
	if err := os.Symlink("releases/1.0.0", home+"/selfishell/previous"); err != nil {
		t.Fatal(err)
	}
	bin := home + "/bin"
	testutil.WriteTestFile(t, bin+"/mise", `#!/bin/sh
printf '%s|%s|%s|%s|%s\n' "$PWD" "$MISE_OFFLINE" "$MISE_GLOBAL_CONFIG_FILE" "$MISE_IGNORED_CONFIG_PATHS" "$*" >> "$MISE_LOG"
case "$*" in
  *'settings get ignored_config_paths'*) printf '%s\n' "${IGNORED_RESULT:-[]}" ;;
  *'config ls --tracked-configs'*) printf '%s\n' "${TRACKED_RESULT:-$MISE_GLOBAL_CONFIG_FILE}" ;;
  *'prune --tools --yes'*) rm -rf "${PRUNE_REMOVE:-/nonexistent}"; exit "${PRUNE_EXIT:-0}" ;;
esac
`, 0755)
	op.Process.Env = append(env.List(), "HOME="+home, "PATH="+bin+":/usr/bin:/bin", "MISE_LOG="+home+"/mise.log")
	return op, paths, root, home, []Package{{Platform: "all", Manager: "mise", Name: "node"}, {Platform: "macos", Manager: "mise", Name: "go"}}
}

func TestMisePruneScopesAndValidatesRetention(t *testing.T) {
	t.Parallel()
	var env process.Environment
	op, paths, root, home, packages := pruneFixture(t, &env)
	if err := op.PruneMise(context.Background(), root, paths, packages, "ubuntu-wsl", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(home + "/mise.log"); !os.IsNotExist(err) {
		t.Fatalf("dry-run called mise: %v", err)
	}
	if err := op.PruneMise(context.Background(), root, paths, packages, "ubuntu-wsl", false); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(home + "/mise.log")
	if !strings.Contains(string(data), "|1|"+root+"/config/shared/mise.toml|") || !strings.Contains(string(data), "/releases/1.0.0/config/shared/mise.toml|") {
		t.Fatalf("scope: %s", data)
	}
	if !strings.Contains(string(data), "prune --tools --yes node") || strings.Contains(string(data), "prune --tools --yes node go") {
		t.Fatalf("tools: %s", data)
	}
	if got, _ := os.Readlink(home + "/selfishell/previous"); got != "releases/1.0.0" {
		t.Fatalf("changed previous: %s", got)
	}
	installs := process.Environment(op.Process.Env).Get("MISE_DATA_DIR") + "/installs/node"
	testutil.WriteTestFile(t, installs+"/24.18.0/bin/node", "", 0755)
	if err := os.Symlink("24.18.0", installs+"/latest"); err != nil {
		t.Fatal(err)
	}
	for _, remove := range []string{installs + "/latest", installs + "/24.18.0"} {
		op.Process = process.WithEnvironment(op.Process, map[string]string{"PRUNE_REMOVE": remove})
		if err := op.PruneMise(context.Background(), root, paths, packages, "ubuntu-wsl", false); err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(op.Process.Out.(*bytes.Buffer).String(), "exec zsh"); got != (remove == installs+"/24.18.0") {
			t.Fatalf("restart note after removing %s: %t", remove, got)
		}
	}
	python := process.Environment(op.Process.Env).Get("MISE_DATA_DIR") + "/installs/python"
	dataHome := process.Environment(op.Process.Env).Get("XDG_DATA_HOME")
	if dataHome == "" {
		dataHome = home + "/.local/share"
	}
	mason := dataHome + "/nvim/mason/packages"
	testutil.WriteTestFile(t, python+"/3.14.8/bin/python3", "", 0755)
	for name, base := range map[string]string{"debugpy": python + "/3.14.8/bin", "current": python + "/3.14.9/bin", "homebrew": "/opt/homebrew/opt/python@3.14/bin"} {
		testutil.WriteTestFile(t, mason+"/"+name+"/venv/pyvenv.cfg", "home = "+base+"\nversion = 3.14\n", 0600)
	}
	op.Process = process.WithEnvironment(op.Process, map[string]string{"PRUNE_REMOVE": python + "/3.14.8"})
	op.Process.Out = &bytes.Buffer{}
	if err := op.PruneMise(context.Background(), root, paths, packages, "ubuntu-wsl", false); err != nil {
		t.Fatal(err)
	}
	if got := op.Process.Out.(*bytes.Buffer).String(); !strings.Contains(got, "':MasonInstall --force debugpy'") {
		t.Fatalf("Mason virtualenv on removed Python: %s", got)
	}
	if err := testutil.WriteFile(home+"/selfishell/releases/1.0.0/VERSION", []byte("invalid\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(home + "/mise.log"); err != nil {
		t.Fatal(err)
	}
	if err := op.PruneMise(context.Background(), root, paths, packages, "ubuntu", false); err == nil {
		t.Fatal("accepted corrupt release")
	}
	if _, err := os.Stat(home + "/mise.log"); !os.IsNotExist(err) {
		t.Fatalf("mise called after bad release: %v", err)
	}
}

func TestMisePruneRejectsIgnoredAndUnsafePrevious(t *testing.T) {
	t.Parallel()
	var env process.Environment
	op, paths, root, home, packages := pruneFixture(t, &env)
	op.Process.Env = append(op.Process.Env, "IGNORED_RESULT=[\"project\"]")
	if err := op.PruneMise(context.Background(), root, paths, packages, "ubuntu", false); err == nil {
		t.Fatal("accepted ignored paths")
	}
	data, _ := os.ReadFile(home + "/mise.log")
	if strings.Contains(string(data), "prune --tools") {
		t.Fatalf("pruned: %s", data)
	}
	op.Process.Env = append(op.Process.Env, "IGNORED_RESULT=[]")
	oldConfig := home + "/selfishell/releases/1.0.0/config/shared/mise.toml"
	if err := os.Remove(oldConfig); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(home+"/project/mise.toml", oldConfig); err != nil {
		t.Fatal(err)
	}
	if err := op.PruneMise(context.Background(), root, paths, packages, "ubuntu", false); err == nil {
		t.Fatal("accepted linked previous config")
	}
}

func TestMisePruneSkipsIncompleteAndEmptyScope(t *testing.T) {
	t.Parallel()
	var env process.Environment
	op, paths, root, home, packages := pruneFixture(t, &env)
	op.SkippedOptional = []string{"go"}
	if err := op.PruneMise(context.Background(), root, paths, packages, "ubuntu", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(home + "/mise.log"); !os.IsNotExist(err) {
		t.Fatalf("mise called for partial setup: %v", err)
	}
	op.SkippedOptional = nil
	if err := op.PruneMise(context.Background(), root, paths, nil, "ubuntu", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(home + "/mise.log"); !os.IsNotExist(err) {
		t.Fatalf("mise called for empty tools: %v", err)
	}
}

func TestMisePruneRequiresTrackedCurrentPinsAndReturnsPruneFailure(t *testing.T) {
	t.Parallel()
	var env process.Environment
	op, paths, root, home, packages := pruneFixture(t, &env)
	op.Process.Env = append(op.Process.Env, "TRACKED_RESULT="+home+"/project/mise.toml")
	if err := op.PruneMise(context.Background(), root, paths, packages, "ubuntu", false); err == nil {
		t.Fatal("accepted untracked current pins")
	}
	data, _ := os.ReadFile(home + "/mise.log")
	if strings.Contains(string(data), "prune --tools") {
		t.Fatalf("pruned without tracked current pins: %s", data)
	}
	op.Process.Env = append(op.Process.Env, "TRACKED_RESULT="+root+"/config/shared/mise.toml", "PRUNE_EXIT=9")
	if err := op.PruneMise(context.Background(), root, paths, packages, "ubuntu", false); err == nil {
		t.Fatal("suppressed cleanup failure")
	}
	if value := process.Environment(op.Process.Env).Get("MISE_IGNORED_CONFIG_PATHS"); value != "" {
		t.Fatalf("leaked scoped ignore: %s", value)
	}
}

func TestRetainedReleaseRejectsLinkSpellingAndIncompleteDirectory(t *testing.T) {
	t.Parallel()
	var env process.Environment
	_, _, root, home, _ := pruneFixture(t, &env)
	previous := home + "/selfishell/previous"
	for _, target := range []string{"../project/1.0.0", "releases/01.0.0", "releases/1.0.0/../2.0.0"} {
		if err := os.Remove(previous); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, previous); err != nil {
			t.Fatal(err)
		}
		if _, _, err := ValidRetainedRelease(root); err == nil {
			t.Fatalf("accepted previous link %s", target)
		}
	}
	if err := os.Remove(previous); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("releases/1.0.0", previous); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(home + "/selfishell/releases/1.0.0/bin/selfishell"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ValidRetainedRelease(root); err == nil {
		t.Fatal("accepted missing retained executable")
	}
}

func TestRetainedReleaseAcceptsAbsoluteRawAliasSpelling(t *testing.T) {
	t.Parallel()
	var env process.Environment
	_, _, root, home, _ := pruneFixture(t, &env)
	alias := home + "/release alias"
	if err := os.Symlink(home+"/selfishell", alias); err != nil {
		t.Fatal(err)
	}
	rawRoot := alias + "//releases/2.0.0"
	previous := home + "/selfishell/previous"
	if err := os.Remove(previous); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(alias+"//releases/1.0.0", previous); err != nil {
		t.Fatal(err)
	}
	version, dir, err := ValidRetainedRelease(rawRoot)
	got, gotErr := os.Stat(dir)
	want, wantErr := os.Stat(home + "/selfishell/releases/1.0.0")
	if err != nil || version != "1.0.0" || gotErr != nil || wantErr != nil || !os.SameFile(got, want) {
		t.Fatalf("raw link rejected: %q %q %v (root %s)", version, dir, err, root)
	}
}
