package selfishell

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRealMisePruneRetainsCurrentAndProjectNotRollbackOnly(t *testing.T) {
	mise, err := exec.LookPath("mise")
	if err != nil {
		t.Skip("mise unavailable for cleanup integration")
	}
	info, err := os.Stat(mise)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
		t.Skip("mise executable unavailable")
	}
	home := t.TempDir()
	root := home + "/selfishell/releases/2.0.0"
	old := home + "/selfishell/releases/1.0.0"
	project := home + "/project with spaces"
	for _, item := range []struct{ dir, version, pin string }{{root, "2.0.0", "24.18.0"}, {old, "1.0.0", "24.13.0"}} {
		writeTestFile(t, item.dir+"/VERSION", item.version+"\n", 0600)
		writeTestFile(t, item.dir+"/bin/selfishell", "#!/bin/sh\n", 0755)
		writeTestFile(t, item.dir+"/config/shared/mise.toml", "[tools]\nnode = \""+item.pin+"\"\n", 0600)
	}
	writeTestFile(t, project+"/mise.toml", "[tools]\nnode = \"22.0.0\"\n", 0600)
	if err := os.Symlink("releases/1.0.0", home+"/selfishell/previous"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(home+"/selfishell", home+"/release alias"); err != nil {
		t.Fatal(err)
	}
	rootAlias := home + "/release alias//releases/2.0.0"
	env := withEnvironment(Process{Env: os.Environ()}, map[string]string{
		"HOME": home, "XDG_CONFIG_HOME": home + "/.config", "XDG_DATA_HOME": home + "/.local/share", "XDG_STATE_HOME": home + "/.local/state", "XDG_CACHE_HOME": home + "/.cache",
		"MISE_DATA_DIR": home + "/.local/share/mise", "MISE_CACHE_DIR": home + "/.cache/mise", "MISE_STATE_DIR": home + "/.local/state/mise", "MISE_OFFLINE": "1", "MISE_TRUSTED_CONFIG_PATHS": project,
	}, "MISE_GLOBAL_CONFIG_FILE", "MISE_IGNORED_CONFIG_PATHS", "__MISE_DIFF", "__MISE_SESSION").Env
	var out, stderr bytes.Buffer
	op := &PackageOperation{Process: Process{Out: &out, Err: &stderr, Env: env, Dir: project}}
	paths := Paths{Config: home + "/.config/selfishell", State: home + "/.local/state/selfishell", Data: home + "/.local/share/selfishell", Cache: home + "/.cache/selfishell"}
	for _, version := range []string{"20.0.0", "22.0.0", "24.13.0", "24.18.0"} {
		writeTestFile(t, home+"/.local/share/mise/installs/node/"+version+"/bin/node", "#!/bin/sh\nexit 0\n", 0755)
	}
	writeTestFile(t, home+"/.local/share/mise/installs/go/1.20.0/bin/go", "#!/bin/sh\nexit 0\n", 0755)
	projectTrack := exec.Command(mise, "-C", project, "config", "ls")
	projectTrack.Env = env
	if output, err := projectTrack.CombinedOutput(); err != nil {
		t.Fatalf("track project: %v %s", err, output)
	}
	packages := []Package{{Platform: "all", Manager: "mise", Name: "node"}, {Platform: "macos", Manager: "mise", Name: "go"}}
	before := treeNames(t, home)
	if err := op.PruneMise(context.Background(), rootAlias, paths, packages, "ubuntu-wsl", true); err != nil {
		t.Fatal(err)
	}
	if after := treeNames(t, home); strings.Join(before, "\n") != strings.Join(after, "\n") {
		t.Fatal("dry-run changed filesystem")
	}
	if err := os.WriteFile(old+"/VERSION", []byte("invalid\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := op.PruneMise(context.Background(), rootAlias, paths, packages, "ubuntu-wsl", false); err == nil {
		t.Fatal("accepted corrupt rollback release")
	}
	if _, err := os.Stat(home + "/.local/share/mise/installs/node/20.0.0"); err != nil {
		t.Fatal("retention failure pruned tools")
	}
	writeTestFile(t, old+"/VERSION", "1.0.0\n", 0600)
	previousTrack := exec.Command(mise, "-C", old+"/config/shared", "config", "ls")
	previousTrack.Env = append(env, "MISE_TRUSTED_CONFIG_PATHS="+home)
	if output, err := previousTrack.CombinedOutput(); err != nil {
		t.Fatalf("track previous: %v %s", err, output)
	}
	op.Process.Env = append(env, "MISE_IGNORED_CONFIG_PATHS="+old)
	if err := op.PruneMise(context.Background(), rootAlias, paths, packages, "ubuntu-wsl", false); err == nil {
		t.Fatal("accepted ignored rollback pins")
	}
	if _, err := os.Stat(home + "/.local/share/mise/installs/node/24.13.0"); err != nil {
		t.Fatal("ignored rollback pin was removed")
	}
	op.Process.Env = env
	if err := op.PruneMise(context.Background(), rootAlias, paths, packages, "ubuntu-wsl", false); err != nil {
		t.Fatalf("prune: %v %s", err, stderr.String())
	}
	for _, version := range []string{"20.0.0", "24.13.0"} {
		if _, err := os.Stat(home + "/.local/share/mise/installs/node/" + version); !os.IsNotExist(err) {
			t.Fatalf("unused node@%s retained: %v", version, err)
		}
	}
	for _, version := range []string{"22.0.0", "24.18.0"} {
		if _, err := os.Stat(home + "/.local/share/mise/installs/node/" + version + "/bin/node"); err != nil {
			t.Fatalf("needed node@%s removed: %v", version, err)
		}
	}
	if _, err := os.Stat(home + "/.local/share/mise/installs/go/1.20.0"); err != nil {
		t.Fatal("other platform tool removed")
	}
	if data, _ := os.ReadFile(old + "/config/shared/mise.toml"); string(data) != "[tools]\nnode = \"24.13.0\"\n" {
		t.Fatalf("old config changed: %q", data)
	}
	if link, _ := os.Readlink(home + "/selfishell/previous"); link != "releases/1.0.0" {
		t.Fatalf("rollback link changed: %s", link)
	}
	writeTestFile(t, home+"/.local/share/mise/installs/node/24.13.0/bin/node", "#!/bin/sh\nexit 0\n", 0755)
	writeTestFile(t, project+"/mise.toml", "[tools]\nnode = [\"22.0.0\", \"24.13.0\"]\n", 0600)
	if err := op.PruneMise(context.Background(), rootAlias, paths, packages, "ubuntu-wsl", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(home + "/.local/share/mise/installs/node/24.13.0/bin/node"); err != nil {
		t.Fatal("project version removed")
	}
	if err := os.Rename(old+"/config/shared/mise.toml", home+"/previous.toml"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(project+"/mise.toml", old+"/config/shared/mise.toml"); err != nil {
		t.Fatal(err)
	}
	if err := op.PruneMise(context.Background(), rootAlias, paths, packages, "ubuntu-wsl", false); err == nil {
		t.Fatal("accepted linked rollback config")
	}
	if _, err := os.Stat(home + "/.local/share/mise/installs/node/22.0.0/bin/node"); err != nil {
		t.Fatal("linked project version removed")
	}
	if err := os.Remove(old + "/config/shared/mise.toml"); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(home+"/previous.toml", old+"/config/shared/mise.toml"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(home + "/selfishell/previous"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("releases/2.0.0", home+"/selfishell/previous"); err != nil {
		t.Fatal(err)
	}
	if err := op.PruneMise(context.Background(), rootAlias, paths, packages, "ubuntu-wsl", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(home + "/.local/share/mise/installs/node/24.18.0/bin/node"); err != nil {
		t.Fatal("current version removed")
	}
	if err := os.Remove(home + "/selfishell/previous"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("releases/1.0.0", home+"/selfishell/previous"); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(home+"/selfishell", home+"/selfishell:unsafe"); err != nil {
		t.Fatal(err)
	}
	unsafeRoot := home + "/selfishell:unsafe/releases/2.0.0"
	if err := op.PruneMise(context.Background(), unsafeRoot, paths, packages, "ubuntu-wsl", false); err == nil {
		t.Fatal("accepted colon in previous config")
	}
	if err := op.PruneMise(context.Background(), unsafeRoot, paths, nil, "ubuntu", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(home + "/.local/share/mise/installs/go/1.20.0"); err != nil {
		t.Fatal("empty scope pruned unrelated tool")
	}
}

func treeNames(t *testing.T, root string) []string {
	t.Helper()
	var names []string
	err := filepath.Walk(root, func(path string, _ os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		names = append(names, strings.TrimPrefix(path, root))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return names
}
