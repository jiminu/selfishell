package selfishell

import (
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
    if [[ "$SELFISHELL_TEST_ZINIT_FAIL" == occupy ]]; then
      plugin_dir="${XDG_DATA_HOME:-$HOME/.local/share}/zinit/plugins/${2//\//---}"
      command mkdir -p "$plugin_dir"
      print -r -- 'concurrent user data' > "$plugin_dir/user-data"
      return 1
    fi
    plugin_dir="$ZINIT[PLUGINS_DIR]/${2//\//---}"
    command git clone -q "$SELFISHELL_TEST_ZINIT_SOURCE" "$plugin_dir" || return 1
    command git -C "$plugin_dir" checkout -q --detach "$approved_revision" || return 1
    command mkdir -p "$plugin_dir/._zinit"
    print -r -- '*' > "$plugin_dir/._zinit/.gitignore"
    print -r -- cloneonly > "$plugin_dir/._zinit/ice"
    if [[ -f "$plugin_dir/src/_fixture" ]]; then
      completion_dir="${ZINIT[COMPLETIONS_DIR]:-${XDG_DATA_HOME:-$HOME/.local/share}/zinit/completions}"
      command mkdir -p "$completion_dir"
      [[ -e "$completion_dir/_fixture" || -L "$completion_dir/_fixture" ]] ||
        command ln -s "$plugin_dir/src/_fixture" "$completion_dir/_fixture"
    fi
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
	writeTestFile(t, repo+"/src/_fixture", "#compdef fixture\n", 0600)
	gitCommand(t, repo, "add", ".")
	gitCommand(t, repo, "commit", "--quiet", "-m", "initial")
	head := gitCommand(t, repo, "rev-parse", "HEAD")
	names := []string{"zsh-users/zsh-completions", "Aloxaf/fzf-tab", "zsh-users/zsh-autosuggestions", "zdharma-continuum/fast-syntax-highlighting"}
	revisions := []string{head}
	for index := 1; index < count; index++ {
		writeTestFile(t, repo+"/marker", fmt.Sprintf("marker %d\n", index), 0600)
		gitCommand(t, repo, "add", "marker")
		gitCommand(t, repo, "commit", "--quiet", "-m", fmt.Sprintf("revision %d", index))
		revisions = append(revisions, gitCommand(t, repo, "rev-parse", "HEAD"))
	}
	var lines strings.Builder
	for index, name := range names[:count] {
		fmt.Fprintf(&lines, "zsh-plugin %s %s all all %s - - -\n", name, revisions[index], repo)
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
	op, paths, manifest, home, _, names := zinitFixtureSetup(t, 4)
	deps, err := ReadDependencies(manifest)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	if err := installPlugins(op, paths, manifest); err != nil {
		t.Fatal(err)
	}
	log := strings.Split(strings.TrimSpace(readTestFile(t, home+"/zinit.log")), "\n")
	if len(log) != 2*len(names) {
		t.Fatalf("unexpected calls: %q", log)
	}
	for index, dep := range deps {
		if seen[dep.Version] {
			t.Fatalf("duplicate fixture revision: %s", dep.Version)
		}
		seen[dep.Version] = true
		if dep.Name != names[index] || log[2*index] != "ice cloneonly ver"+dep.Version || log[2*index+1] != "light "+dep.Name {
			t.Fatalf("wrong plugin/pin pairing at %d: %q", index, log)
		}
		if got := gitCommand(t, pluginTarget(home, dep.Name), "rev-parse", "HEAD"); got != dep.Version {
			t.Fatalf("plugin %s at %s", dep.Name, got)
		}
		if readTestFile(t, pluginTarget(home, dep.Name)+"/._zinit/ice") != "cloneonly\n" {
			t.Fatal("Zinit checkout metadata missing after activation")
		}
	}
	if err := installPlugins(op, paths, manifest); err != nil {
		t.Fatal(err)
	}
	if got := strings.Split(strings.TrimSpace(readTestFile(t, home+"/zinit.log")), "\n"); len(got) != len(log) || output(op) != "" {
		t.Fatalf("Zinit metadata caused a repeat provision: %q %q", got, output(op))
	}
}
func TestZinitMissingIsReported(t *testing.T) {
	op, paths, manifest, home, _, _ := zinitFixtureSetup(t, 1)
	os.Remove(home + "/data/zinit/zinit.git/zinit.zsh")
	if err := installPlugins(op, paths, manifest); err == nil || !strings.Contains(err.Error(), "Zinit is not installed:") {
		t.Fatalf("missing Zinit: %v", err)
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
func TestZinitFailedProvisionPreservesConcurrentCanonicalTarget(t *testing.T) {
	op, paths, manifest, home, _, names := zinitFixtureSetup(t, 1)
	t.Setenv("SELFISHELL_TEST_ZINIT_FAIL", "occupy")
	if err := installPlugins(op, paths, manifest); err == nil {
		t.Fatal("failed provision accepted")
	}
	target := pluginTarget(home, names[0])
	if got := readTestFile(t, target+"/user-data"); got != "concurrent user data\n" {
		t.Fatalf("concurrent target changed: %q", got)
	}
	if strings.Contains(output(op), "Updated Zsh plugin") {
		t.Fatalf("success reported: %q", output(op))
	}
}
func TestZinitChangedPluginReprovisioned(t *testing.T) {
	for _, shape := range []string{"drift", "tracked", "untracked"} {
		t.Run(shape, func(t *testing.T) {
			op, paths, manifest, home, head, names := zinitFixtureSetup(t, 1)
			target := pluginTarget(home, names[0])
			os.MkdirAll(filepath.Dir(target), 0700)
			gitCommand(t, filepath.Dir(target), "clone", "--quiet", home+"/repo", target)
			switch shape {
			case "drift":
				gitCommand(t, target, "commit", "--quiet", "--allow-empty", "-m", "drift")
			case "tracked":
				writeTestFile(t, target+"/marker", "edited", 0600)
			case "untracked":
				writeTestFile(t, target+"/untracked", "local", 0600)
			}
			if err := installPlugins(op, paths, manifest); err != nil {
				t.Fatal(err)
			}
			if gitCommand(t, target, "rev-parse", "HEAD") != head || readTestFile(t, target+"/marker") != "marker\n" {
				t.Fatal("checkout not restored to the approved revision")
			}
			assertNoPath(t, target+"/untracked")
			if !strings.Contains(output(op), "Updated Zsh plugin: "+names[0]) {
				t.Fatalf("repair not reported: %q", output(op))
			}
			if left, _ := filepath.Glob(target + ".previous.*"); len(left) != 0 {
				t.Fatalf("previous checkout left behind: %v", left)
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

func TestZinitChecksConcurrentlyAndProvisionsInDeclarationOrder(t *testing.T) {
	op, paths, manifest, home, head, names := zinitFixtureSetup(t, 4)
	deps, err := ReadDependencies(manifest)
	if err != nil {
		t.Fatal(err)
	}
	for index, dep := range deps {
		target := pluginTarget(home, dep.Name)
		os.MkdirAll(filepath.Dir(target), 0700)
		gitCommand(t, filepath.Dir(target), "clone", "--quiet", home+"/repo", target)
		revision := dep.Version
		if index == 1 {
			revision = head // outdated
		}
		gitCommand(t, target, "checkout", "--quiet", "--detach", revision)
		if index == 3 {
			writeTestFile(t, target+"/marker", "edited", 0600) // dirty
		}
	}
	stdin := strings.NewReader("terminal input")
	op.Process.In = stdin
	if err := installPlugins(op, paths, manifest); err != nil {
		t.Fatal(err)
	}
	want := "ice cloneonly ver" + deps[1].Version + "\nlight " + names[1] + "\nice cloneonly ver" + deps[3].Version + "\nlight " + names[3] + "\n"
	if got := readTestFile(t, home+"/zinit.log"); got != want {
		t.Fatalf("provisioning order:\n%s\nwant:\n%s", got, want)
	}
	if out := output(op); strings.Index(out, "Updated Zsh plugin: "+names[1]) > strings.Index(out, "Updated Zsh plugin: "+names[3]) {
		t.Fatalf("reports out of order: %q", out)
	}
	if stdin.Len() != len("terminal input") {
		t.Fatal("a concurrent check read the caller's stdin")
	}
	for _, dep := range deps {
		if got := gitCommand(t, pluginTarget(home, dep.Name), "rev-parse", "HEAD"); got != dep.Version {
			t.Fatalf("%s at %s", dep.Name, got)
		}
	}
}

func TestZinitOrderingAcrossInvalidNamesAndSharedCheckouts(t *testing.T) {
	op, paths, manifest, home, head, _ := zinitFixtureSetup(t, 2)
	deps, err := ReadDependencies(manifest)
	if err != nil {
		t.Fatal(err)
	}
	next := deps[1].Version
	// Two names resolve to one checkout; the second must see the first's work.
	shared := pluginTarget(home, "a/b---c")
	os.MkdirAll(filepath.Dir(shared), 0700)
	gitCommand(t, filepath.Dir(shared), "clone", "--quiet", home+"/repo", shared)
	gitCommand(t, shared, "checkout", "--quiet", "--detach", next)
	writeTestFile(t, manifest, fmt.Sprintf("zsh-plugin a/b---c %s all all %s - - -\nzsh-plugin a---b/c %s all all %s - - -\nzsh-plugin invalid %s all all %s - - -\n", head, home+"/repo", next, home+"/repo", head, home+"/repo"), 0600)
	if err := installPlugins(op, paths, manifest); err == nil || !strings.Contains(err.Error(), "invalid Zinit plugin name: invalid") {
		t.Fatalf("invalid name: %v", err)
	}
	if got := readTestFile(t, home+"/zinit.log"); strings.Count(got, "light ") != 2 {
		t.Fatalf("earlier entries were not provisioned before the invalid name: %q", got)
	}
	if got := gitCommand(t, shared, "rev-parse", "HEAD"); got != next {
		t.Fatalf("shared checkout kept a stale check: %s", got)
	}
}

func TestZinitDryRun(t *testing.T) {
	op, paths, manifest, home, _, _ := zinitFixtureSetup(t, 1)
	os.Remove(home + "/data/zinit/zinit.git/zinit.zsh")
	if err := op.InstallZinitPlugins(context.Background(), paths, manifest, true); err != nil || !strings.Contains(output(op), "Would sync") {
		t.Fatalf("dry run: %v %q", err, output(op))
	}
}

func TestZinitProvisionDoesNotPublishTemporaryCompletionLinks(t *testing.T) {
	op, paths, manifest, home, _, _ := zinitFixtureSetup(t, 1)
	if err := installPlugins(op, paths, manifest); err != nil {
		t.Fatal(err)
	}
	assertNoPath(t, home+"/data/zinit/completions/_fixture")
}

func TestZinitRepairsOnlyItsBrokenStagingCompletionLinks(t *testing.T) {
	op, paths, manifest, home, _, names := zinitFixtureSetup(t, 1)
	target := pluginTarget(home, names[0])
	os.MkdirAll(filepath.Dir(target), 0700)
	gitCommand(t, filepath.Dir(target), "clone", "--quiet", home+"/repo", target)
	dir := home + "/data/zinit/completions"
	os.MkdirAll(dir, 0700)
	stale := home + "/data/zinit/plugins/.selfishell-zinit.123/zsh-users---zsh-completions/src/_fixture"
	zinitStale := home + "/data/zinit/plugins/.selfishell-zinit.123/_local---zinit/_zinit"
	writeTestFile(t, home+"/data/zinit/zinit.git/_zinit", "#compdef zinit\n", 0600)
	for name, reference := range map[string]string{
		"_fixture": stale,
		"_zinit":   zinitStale,
		"_foreign": home + "/missing/_foreign",
		"fixture":  stale, // Disabled completion stays disabled.
		"_unknown": home + "/data/zinit/plugins/.selfishell-zinit.123/other---plugin/src/_unknown",
	} {
		if err := os.Symlink(reference, dir+"/"+name); err != nil {
			t.Fatal(err)
		}
	}
	writeTestFile(t, dir+"/_user", "user completion\n", 0600)
	for _, dryRun := range []bool{true, false, false} {
		if err := op.InstallZinitPlugins(context.Background(), paths, manifest, dryRun); err != nil {
			t.Fatal(err)
		}
		want := target + "/src/_fixture"
		if dryRun {
			want = stale
		}
		if got, err := os.Readlink(dir + "/_fixture"); err != nil || got != want {
			t.Fatalf("completion target = %q, %v; want %q", got, err, want)
		}
		zinitWant := home + "/data/zinit/zinit.git/_zinit"
		if dryRun {
			zinitWant = zinitStale
		}
		if got, err := os.Readlink(dir + "/_zinit"); err != nil || got != zinitWant {
			t.Fatalf("Zinit completion target = %q, %v; want %q", got, err, zinitWant)
		}
		if got, _ := os.Readlink(dir + "/_unknown"); got != home+"/data/zinit/plugins/.selfishell-zinit.123/other---plugin/src/_unknown" {
			t.Fatal("unmanaged plugin completion changed")
		}
		if got, _ := os.Readlink(dir + "/_foreign"); got != home+"/missing/_foreign" {
			t.Fatal("foreign link changed")
		}
		if got, _ := os.Readlink(dir + "/fixture"); got != stale {
			t.Fatal("disabled completion changed")
		}
		if got := readTestFile(t, dir+"/_user"); got != "user completion\n" {
			t.Fatal("user file changed")
		}
	}
	assertNoPath(t, home+"/zinit.log") // Repair does not reprovision valid plugins.
}

func TestZinitRepairsCanonicalCompletionLinksWithSymlinkDataHome(t *testing.T) {
	op, paths, manifest, home, _, names := zinitFixtureSetup(t, 1)
	target := pluginTarget(home, names[0])
	os.MkdirAll(filepath.Dir(target), 0700)
	gitCommand(t, filepath.Dir(target), "clone", "--quiet", home+"/repo", target)
	if err := os.Symlink(home+"/data", home+"/data-alias"); err != nil {
		t.Fatal(err)
	}
	paths.Data = home + "/data-alias/selfishell"
	dir := home + "/data/zinit/completions"
	os.MkdirAll(dir, 0700)
	if err := os.Symlink(home+"/data/zinit/plugins/.selfishell-zinit.123/zsh-users---zsh-completions/src/_fixture", dir+"/_fixture"); err != nil {
		t.Fatal(err)
	}
	if err := installPlugins(op, paths, manifest); err != nil {
		t.Fatal(err)
	}
	if got := readTestFile(t, dir+"/_fixture"); got != "#compdef fixture\n" {
		t.Fatalf("completion not recovered: %q", got)
	}
}

func TestZinitCompletionRepairRetainsSymlinkDotDotTraversal(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	target := home + "/data/zinit/plugins/zsh-users---zsh-completions"
	writeTestFile(t, target+"/src/_fixture", "#compdef fixture\n", 0600)
	os.MkdirAll(home+"/nested/deep", 0700)
	if err := os.Symlink(home+"/nested/deep", home+"/alias"); err != nil {
		t.Fatal(err)
	}
	dir := home + "/data/zinit/completions"
	os.MkdirAll(dir, 0700)
	if err := os.Symlink(home+"/data/zinit/plugins/.selfishell-zinit.123/zsh-users---zsh-completions/src/_fixture", dir+"/_fixture"); err != nil {
		t.Fatal(err)
	}
	dataHome := home + "/alias/../../data"
	if err := repairZinitCompletions(context.Background(), dataHome, dataHome+"/zinit/plugins/zsh-users---zsh-completions"); err != nil {
		t.Fatal(err)
	}
	if got := readTestFile(t, dir+"/_fixture"); got != "#compdef fixture\n" {
		t.Fatalf("completion not recovered: %q", got)
	}
}
