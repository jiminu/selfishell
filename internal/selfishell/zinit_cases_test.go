package selfishell

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const zinitFixture = `zinit() {
  print -r -- "$*" >> "$SELFISHELL_TEST_ZINIT_LOG"
  if [[ "$1" == ice ]]; then
    approved_revision="${3#ver}"
    return 0
  fi
  if [[ "$1" == light ]]; then
    [[ "$SELFISHELL_TEST_ZINIT_FAIL" != before ]] || return 1
    plugin_dir="${XDG_DATA_HOME:-$HOME/.local/share}/zinit/plugins/${2//\//---}"
    command git clone -q "$SELFISHELL_TEST_ZINIT_SOURCE" "$plugin_dir" || return 1
    command git -C "$plugin_dir" checkout -q --detach "$approved_revision" || return 1
    [[ "$SELFISHELL_TEST_ZINIT_FAIL" != after ]]
  fi
}
`

func zinitFixtureSetup(t *testing.T, count int) (*PackageOperation, Paths, string, string, string, []string) {
	t.Helper()
	op, paths, manifest, home := dependencyFixture(t)
	repo := home + "/repo"
	os.MkdirAll(repo, 0700)
	gitCommand(t, repo, "init", "--quiet")
	writeTestFile(t, repo+"/marker", "marker\n", 0600)
	gitCommand(t, repo, "add", "marker")
	gitCommand(t, repo, "commit", "--quiet", "-m", "initial")
	head := gitCommand(t, repo, "rev-parse", "HEAD")
	names := []string{"zsh-users/zsh-completions", "Aloxaf/fzf-tab", "zsh-users/zsh-autosuggestions", "zdharma-continuum/fast-syntax-highlighting"}
	var lines strings.Builder
	for _, name := range names[:count] {
		fmt.Fprintf(&lines, "zsh-plugin %s %s all all %s - - -\n", name, head, repo)
	}
	writeTestFile(t, manifest, lines.String(), 0600)
	writeTestFile(t, home+"/data/zinit/zinit.git/zinit.zsh", zinitFixture, 0600)
	t.Setenv("SELFISHELL_TEST_ZINIT_LOG", home+"/zinit.log")
	t.Setenv("SELFISHELL_TEST_ZINIT_SOURCE", repo)
	return op, paths, manifest, home, head, names[:count]
}
func pluginTarget(home, name string) string {
	return home + "/data/zinit/plugins/" + strings.ReplaceAll(name, "/", "---")
}
func installPlugins(op *PackageOperation, paths Paths, manifest string) error {
	return op.InstallZinitPlugins(context.Background(), paths, manifest, false)
}

func TestZinitProvisionsDeclaredPluginsWithoutLoading(t *testing.T) {
	op, paths, manifest, home, head, names := zinitFixtureSetup(t, 4)
	if err := installPlugins(op, paths, manifest); err != nil {
		t.Fatal(err)
	}
	log := readTestFile(t, home+"/zinit.log")
	if strings.Count(log, "light ") != 4 {
		t.Fatalf("expected four plugins: %s", log)
	}
	for _, name := range names {
		if !strings.Contains(log, "ice cloneonly ver"+head) || !strings.Contains(log, "light "+name) {
			t.Fatalf("cloneonly pin missing: %s", log)
		}
		if got := gitCommand(t, pluginTarget(home, name), "rev-parse", "HEAD"); got != head {
			t.Fatalf("plugin %s at %s", name, got)
		}
	}
}
func TestZinitMissingIsReported(t *testing.T) {
	op, paths, manifest, home, _, _ := zinitFixtureSetup(t, 1)
	os.Remove(home + "/data/zinit/zinit.git/zinit.zsh")
	if err := installPlugins(op, paths, manifest); err == nil || !strings.Contains(err.Error(), "Zinit is not installed:") {
		t.Fatalf("missing Zinit: %v", err)
	}
}
func TestZinitProvisioningFailure(t *testing.T) {
	op, paths, manifest, _, _, _ := zinitFixtureSetup(t, 1)
	t.Setenv("SELFISHELL_TEST_ZINIT_FAIL", "before")
	if err := installPlugins(op, paths, manifest); err == nil {
		t.Fatal("provisioning failure ignored")
	}
}
func TestZinitFailedFreshPluginCleanedForRetry(t *testing.T) {
	op, paths, manifest, home, head, names := zinitFixtureSetup(t, 1)
	t.Setenv("SELFISHELL_TEST_ZINIT_FAIL", "after")
	if err := installPlugins(op, paths, manifest); err == nil {
		t.Fatal("failed fresh plugin accepted")
	}
	target := pluginTarget(home, names[0])
	assertNoPath(t, target)
	t.Setenv("SELFISHELL_TEST_ZINIT_FAIL", "")
	if err := installPlugins(op, paths, manifest); err != nil || gitCommand(t, target, "rev-parse", "HEAD") != head {
		t.Fatalf("retry failed: %v", err)
	}
}
func TestZinitApprovedPluginNoop(t *testing.T) {
	op, paths, manifest, home, head, names := zinitFixtureSetup(t, 1)
	target := pluginTarget(home, names[0])
	os.MkdirAll(filepath.Dir(target), 0700)
	gitCommand(t, filepath.Dir(target), "clone", "--quiet", home+"/repo", target)
	gitCommand(t, target, "checkout", "--quiet", "--detach", head)
	if err := installPlugins(op, paths, manifest); err != nil {
		t.Fatal(err)
	}
	assertNoPath(t, home+"/zinit.log")
	if output(op) != "" {
		t.Fatalf("noop output: %q", output(op))
	}
}
func TestZinitOutdatedPluginReprovisioned(t *testing.T) {
	op, paths, manifest, home, head, names := zinitFixtureSetup(t, 1)
	target := pluginTarget(home, names[0])
	os.MkdirAll(filepath.Dir(target), 0700)
	gitCommand(t, filepath.Dir(target), "clone", "--quiet", home+"/repo", target)
	gitCommand(t, target, "commit", "--quiet", "--allow-empty", "-m", "drift")
	writeTestFile(t, target+"/untracked", "old", 0600)
	if err := installPlugins(op, paths, manifest); err != nil {
		t.Fatal(err)
	}
	if got := gitCommand(t, target, "rev-parse", "HEAD"); got != head {
		t.Fatalf("drift remains: %s", got)
	}
	assertNoPath(t, target+"/untracked")
	if !strings.Contains(output(op), "Updated Zsh plugin: "+names[0]) {
		t.Fatalf("update not reported: %q", output(op))
	}
	entries, err := os.ReadDir(filepath.Dir(target))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".previous.") {
			t.Fatal("previous checkout left behind")
		}
	}
}
func TestZinitDirtyApprovedPluginReprovisioned(t *testing.T) {
	for _, shape := range []string{"tracked", "untracked"} {
		t.Run(shape, func(t *testing.T) {
			op, paths, manifest, home, head, names := zinitFixtureSetup(t, 1)
			target := pluginTarget(home, names[0])
			os.MkdirAll(filepath.Dir(target), 0700)
			gitCommand(t, filepath.Dir(target), "clone", "--quiet", home+"/repo", target)
			gitCommand(t, target, "checkout", "--quiet", "--detach", head)
			if shape == "tracked" {
				writeTestFile(t, target+"/marker", "edited", 0600)
			} else {
				writeTestFile(t, target+"/untracked", "local", 0600)
			}
			if err := installPlugins(op, paths, manifest); err != nil {
				t.Fatal(err)
			}
			if readTestFile(t, target+"/marker") != "marker\n" {
				t.Fatal("tracked edit remains")
			}
			assertNoPath(t, target+"/untracked")
			if !strings.Contains(output(op), "Updated Zsh plugin: "+names[0]) {
				t.Fatalf("dirty repair not reported: %q", output(op))
			}
		})
	}
}
func TestZinitFailedReprovisionRestoresPreviousCheckout(t *testing.T) {
	op, paths, manifest, home, _, names := zinitFixtureSetup(t, 1)
	target := pluginTarget(home, names[0])
	writeTestFile(t, target+"/marker", "old data", 0600)
	t.Setenv("SELFISHELL_TEST_ZINIT_FAIL", "after")
	if err := installPlugins(op, paths, manifest); err == nil {
		t.Fatal("failed reprovision accepted")
	}
	if got := readTestFile(t, target+"/marker"); got != "old data" {
		t.Fatalf("previous checkout lost: %q", got)
	}
	entries, err := os.ReadDir(filepath.Dir(target))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".previous.") {
			t.Fatal("previous checkout left behind")
		}
	}
}

func TestZinitDryRunAndCancellation(t *testing.T) {
	op, paths, manifest, home, _, _ := zinitFixtureSetup(t, 1)
	os.Remove(home + "/data/zinit/zinit.git/zinit.zsh")
	if err := op.InstallZinitPlugins(context.Background(), paths, manifest, true); err != nil || !strings.Contains(output(op), "Would sync") {
		t.Fatalf("dry run: %v %q", err, output(op))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	op.Process.Out = new(bytes.Buffer)
	if err := op.InstallZinitPlugins(ctx, paths, manifest, false); err != context.Canceled || output(op) != "" {
		t.Fatalf("cancelled operation: %v %q", err, output(op))
	}
}
