package selfishell

import (
	"context"
	"os"
	"strings"
	"testing"
)

func pruneFixture(t *testing.T) (*PackageOperation, Paths, string, string, []Package) {
	t.Helper()
	op, paths, _, home := dependencyFixture(t)
	root := home + "/selfishell/releases/2.0.0"
	for _, version := range []string{"1.0.0", "2.0.0"} {
		dir := home + "/selfishell/releases/" + version
		writeTestFile(t, dir+"/VERSION", version+"\n", 0600)
		writeTestFile(t, dir+"/bin/selfishell", "#!/bin/sh\n", 0755)
		writeTestFile(t, dir+"/config/shared/mise.toml", "[tools]\nnode = \"24.18.0\"\n", 0600)
	}
	if err := os.Symlink("releases/1.0.0", home+"/selfishell/previous"); err != nil {
		t.Fatal(err)
	}
	bin := home + "/bin"
	writeTestFile(t, bin+"/mise", `#!/bin/sh
printf '%s|%s|%s|%s|%s\n' "$PWD" "$MISE_OFFLINE" "$MISE_GLOBAL_CONFIG_FILE" "$MISE_IGNORED_CONFIG_PATHS" "$*" >> "$MISE_LOG"
case "$*" in
  *'settings get ignored_config_paths'*) printf '%s\n' "${IGNORED_RESULT:-[]}" ;;
  *'config ls --tracked-configs'*) printf '%s\n' "${TRACKED_RESULT:-$MISE_GLOBAL_CONFIG_FILE}" ;;
  *'prune --tools --yes'*) exit "${PRUNE_EXIT:-0}" ;;
esac
`, 0755)
	op.Process.Env = append(os.Environ(), "HOME="+home, "PATH="+bin+":/usr/bin:/bin", "MISE_LOG="+home+"/mise.log")
	return op, paths, root, home, []Package{{Platform: "all", Manager: "mise", Name: "node"}, {Platform: "macos", Manager: "mise", Name: "go"}}
}

func TestMisePruneScopesAndValidatesRetention(t *testing.T) {
	op, paths, root, home, packages := pruneFixture(t)
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
	if err := os.WriteFile(home+"/selfishell/releases/1.0.0/VERSION", []byte("invalid\n"), 0600); err != nil {
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
	op, paths, root, home, packages := pruneFixture(t)
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
	op, paths, root, home, packages := pruneFixture(t)
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
	op, paths, root, home, packages := pruneFixture(t)
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
	if value := envValue(op.Process.Env, "MISE_IGNORED_CONFIG_PATHS"); value != "" {
		t.Fatalf("leaked scoped ignore: %s", value)
	}
}

func TestValidReleaseVersionMatchesLegacyVectors(t *testing.T) {
	for _, version := range []string{"0.0.0", "1.2.3", "1.2.3-alpha", "1.2.3-alpha.1", "1.2.3-0.3.7", "1.2.3-x.7.z-92", "1.2.3-01alpha"} {
		if !ValidReleaseVersion(version) {
			t.Fatalf("valid version rejected: %s", version)
		}
	}
	for _, version := range []string{"v1.2.3", "01.2.3", "1.02.3", "1.2.03", "1.2", "1.2.3-", "1.2.3-alpha..1", "1.2.3-alpha_1", "1.2.3-01", "1.2.3-alpha.01", "1.2.3+build"} {
		if ValidReleaseVersion(version) {
			t.Fatalf("invalid version accepted: %s", version)
		}
	}
}

func TestRetainedReleaseRejectsLinkSpellingAndIncompleteDirectory(t *testing.T) {
	_, _, root, home, _ := pruneFixture(t)
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
	_, _, root, home, _ := pruneFixture(t)
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
