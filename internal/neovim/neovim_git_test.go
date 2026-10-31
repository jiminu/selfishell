package neovim

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jiminu/selfishell/internal/process"
	"github.com/jiminu/selfishell/internal/testutil"
)

// gitFixture isolates git from the developer's HOME and configuration.
func gitFixture(t *testing.T) (process.Process, string) {
	t.Helper()
	home := t.TempDir()
	var env process.Environment
	testutil.SetEnv(&env, "HOME", home)
	testutil.SetEnv(&env, "XDG_CONFIG_HOME", home+"/.config")
	return process.Process{Inherit: env, Out: new(bytes.Buffer), Err: new(bytes.Buffer)}, home
}

func TestGitHeadReadsDetachedLoosePackedAndFallback(t *testing.T) {
	t.Parallel()
	p, home := gitFixture(t)
	sha := "0123456789abcdef0123456789abcdef01234567"
	for _, fixture := range []struct{ name, head, ref, packed string }{
		{"detached", sha + "\nignored second line\n", "", ""},
		{"loose", "ref: refs/heads/main\n", sha + "\n", ""},
		{"packed", "ref: refs/heads/main\n", "", "# pack-refs with: peeled fully-peeled sorted\n" + sha + " refs/heads/main\n^" + sha + "\n"},
	} {
		dir := home + "/" + fixture.name
		testutil.WriteTestFile(t, dir+"/.git/HEAD", fixture.head, 0600)
		if fixture.ref != "" {
			testutil.WriteTestFile(t, dir+"/.git/refs/heads/main", fixture.ref, 0600)
		}
		if fixture.packed != "" {
			testutil.WriteTestFile(t, dir+"/.git/packed-refs", fixture.packed, 0600)
		}
		if got, err := GitHead(context.Background(), p, dir); err != nil || got != sha {
			t.Fatalf("%s: %s %v", fixture.name, got, err)
		}
	}
	repo := home + "/fallback"
	if err := os.MkdirAll(repo, 0700); err != nil {
		t.Fatal(err)
	}
	testutil.GitCommand(t, repo, "init", "-q")
	testutil.WriteTestFile(t, repo+"/a", "a", 0600)
	testutil.GitCommand(t, repo, "add", "a")
	testutil.GitCommand(t, repo, "commit", "-qm", "initial")
	want := testutil.GitCommand(t, repo, "rev-parse", "HEAD")
	worktree := home + "/worktree"
	testutil.GitCommand(t, repo, "worktree", "add", "-q", "--detach", worktree)
	if got, err := GitHead(context.Background(), p, worktree); err != nil || got != want {
		t.Fatalf("worktree fallback: %s %v", got, err)
	}
}

func TestGitHeadFallbackCannotReadParentRepository(t *testing.T) {
	t.Parallel()
	p, home := gitFixture(t)
	parent := home + "/parent"
	if err := os.MkdirAll(parent, 0700); err != nil {
		t.Fatal(err)
	}
	testutil.GitCommand(t, parent, "init", "-q")
	testutil.WriteTestFile(t, parent+"/tracked", "parent\n", 0600)
	testutil.GitCommand(t, parent, "add", "tracked")
	testutil.GitCommand(t, parent, "commit", "-qm", "parent")
	parentHead := testutil.GitCommand(t, parent, "rev-parse", "HEAD")
	for _, child := range []struct {
		name      string
		malformed bool
	}{{"missing", false}, {"malformed", true}} {
		dir := parent + "/" + child.name
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if child.malformed {
			testutil.WriteTestFile(t, dir+"/.git", "gitdir: missing-metadata\n", 0600)
		}
		got, err := GitHead(context.Background(), p, dir)
		if err == nil || got == parentHead {
			t.Fatalf("%s child read parent HEAD: %q, err=%v", child.name, got, err)
		}
	}
}

func TestGitTrackedChangesIgnoreUntrackedAndGeneratedTags(t *testing.T) {
	t.Parallel()
	p, home := gitFixture(t)
	repo := home + "/checkout"
	if err := os.MkdirAll(repo, 0700); err != nil {
		t.Fatal(err)
	}
	testutil.GitCommand(t, repo, "init", "-q")
	testutil.WriteTestFile(t, repo+"/a", "a\n", 0600)
	testutil.WriteTestFile(t, repo+"/doc/tags", "tags\n", 0600)
	testutil.GitCommand(t, repo, "add", "a", "doc/tags")
	testutil.GitCommand(t, repo, "commit", "-qm", "initial")
	testutil.WriteTestFile(t, repo+"/untracked", "cache\n", 0600)
	if err := os.Chtimes(repo+"/a", time.Now(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if changes, err := gitTrackedChanges(context.Background(), p, repo, false); err != nil || changes != "" {
		t.Fatalf("untracked counted: %q %v", changes, err)
	}
	testutil.WriteTestFile(t, repo+"/doc/tags", "edited\n", 0600)
	if changes, err := gitTrackedChanges(context.Background(), p, repo, false); err != nil || !strings.Contains(changes, "doc/tags") {
		t.Fatalf("tracked tag omitted: %q %v", changes, err)
	}
	testutil.GitCommand(t, repo, "add", "doc/tags")
	if changes, err := gitTrackedChanges(context.Background(), p, repo, true); err != nil || changes != "" {
		t.Fatalf("excluded tag counted: %q %v", changes, err)
	}
	testutil.GitCommand(t, repo, "reset", "--hard", "-q")
	os.Remove(repo + "/.git/index")
	if changes, err := gitTrackedChanges(context.Background(), p, repo, false); err != nil || changes == "" {
		t.Fatalf("missing index not detected: %q %v", changes, err)
	}
	if err := os.MkdirAll(repo+"/broken/.git", 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := gitTrackedChanges(context.Background(), p, repo+"/broken", false); err == nil {
		t.Fatal("broken nested checkout reached parent repository")
	}
}
