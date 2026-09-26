package selfishell

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestGitHeadReadsDetachedLoosePackedAndFallback(t *testing.T) {
	op, _, _, home := dependencyFixture(t)
	sha := "0123456789abcdef0123456789abcdef01234567"
	for _, fixture := range []struct{ name, head, ref, packed string }{
		{"detached", sha + "\nignored second line\n", "", ""},
		{"loose", "ref: refs/heads/main\n", sha + "\n", ""},
		{"packed", "ref: refs/heads/main\n", "", "# pack-refs with: peeled fully-peeled sorted\n" + sha + " refs/heads/main\n^" + sha + "\n"},
	} {
		dir := home + "/" + fixture.name
		writeTestFile(t, dir+"/.git/HEAD", fixture.head, 0600)
		if fixture.ref != "" {
			writeTestFile(t, dir+"/.git/refs/heads/main", fixture.ref, 0600)
		}
		if fixture.packed != "" {
			writeTestFile(t, dir+"/.git/packed-refs", fixture.packed, 0600)
		}
		if got, err := op.gitHead(context.Background(), dir); err != nil || got != sha {
			t.Fatalf("%s: %s %v", fixture.name, got, err)
		}
	}
	repo := home + "/fallback"
	if err := os.MkdirAll(repo, 0700); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, repo, "init", "-q")
	writeTestFile(t, repo+"/a", "a", 0600)
	gitCommand(t, repo, "add", "a")
	gitCommand(t, repo, "commit", "-qm", "initial")
	want := gitCommand(t, repo, "rev-parse", "HEAD")
	worktree := home + "/worktree"
	gitCommand(t, repo, "worktree", "add", "-q", "--detach", worktree)
	if got, err := op.gitHead(context.Background(), worktree); err != nil || got != want {
		t.Fatalf("worktree fallback: %s %v", got, err)
	}
}

func TestGitTrackedChangesIgnoreUntrackedAndGeneratedTags(t *testing.T) {
	op, _, _, home := dependencyFixture(t)
	repo := home + "/checkout"
	if err := os.MkdirAll(repo, 0700); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, repo, "init", "-q")
	writeTestFile(t, repo+"/a", "a\n", 0600)
	writeTestFile(t, repo+"/doc/tags", "tags\n", 0600)
	gitCommand(t, repo, "add", "a", "doc/tags")
	gitCommand(t, repo, "commit", "-qm", "initial")
	writeTestFile(t, repo+"/untracked", "cache\n", 0600)
	if err := os.Chtimes(repo+"/a", time.Now(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if changes, err := op.gitTrackedChanges(context.Background(), repo, false); err != nil || changes != "" {
		t.Fatalf("untracked counted: %q %v", changes, err)
	}
	writeTestFile(t, repo+"/doc/tags", "edited\n", 0600)
	if changes, err := op.gitTrackedChanges(context.Background(), repo, false); err != nil || !strings.Contains(changes, "doc/tags") {
		t.Fatalf("tracked tag omitted: %q %v", changes, err)
	}
	gitCommand(t, repo, "add", "doc/tags")
	if changes, err := op.gitTrackedChanges(context.Background(), repo, true); err != nil || changes != "" {
		t.Fatalf("excluded tag counted: %q %v", changes, err)
	}
	gitCommand(t, repo, "reset", "--hard", "-q")
	for _, change := range []string{"staged edit", "deletion", "staged deletion", "staged new file", "missing index"} {
		switch change {
		case "staged edit":
			writeTestFile(t, repo+"/a", "edited\n", 0600)
			gitCommand(t, repo, "add", "a")
		case "deletion":
			os.Remove(repo + "/a")
		case "staged deletion":
			gitCommand(t, repo, "rm", "-q", "a")
		case "staged new file":
			writeTestFile(t, repo+"/new", "new\n", 0600)
			gitCommand(t, repo, "add", "new")
		case "missing index":
			os.Remove(repo + "/.git/index")
		}
		if changes, err := op.gitTrackedChanges(context.Background(), repo, false); err != nil || changes == "" {
			t.Fatalf("%s not detected: %q %v", change, changes, err)
		}
		gitCommand(t, repo, "reset", "--hard", "-q")
	}
	if err := os.MkdirAll(repo+"/broken/.git", 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := op.gitTrackedChanges(context.Background(), repo+"/broken", false); err == nil {
		t.Fatal("broken nested checkout reached parent repository")
	}
}
