package integration_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func previewFixture(t *testing.T) string {
	t.Helper()
	home := nativeHome(t)
	fakeExecutable(t, home, "fzf", "printf ':\\n'")
	nativeWrite(t, filepath.Join(home, ".local/share/zinit/zinit.git/zinit.zsh"), `typeset -gA ZINIT
ZINIT[PLUGINS_DIR]="$HOME/.local/share/zinit/plugins"
zinit() { :; }
`, 0600)
	return home
}
func previewCommands(t *testing.T, home string) map[string]string {
	t.Helper()
	mustFS(t, os.MkdirAll(filepath.Join(home, ".local/share/zinit/plugins/Aloxaf---fzf-tab/.git"), 0700))
	contexts := map[string]string{"directory": ":fzf-tab:complete:cd:x", "file": ":fzf-tab:complete:vim:x", "branch": ":fzf-tab:complete:git-switch:x", "diff": ":fzf-tab:complete:git-add:x", "stash": ":fzf-tab:complete:git-stash-show:x", "process": ":fzf-tab:complete:kill:argument-rest"}
	commands := map[string]string{}
	for name, context := range contexts {
		r := nativeRun(t, home, `source "$SELFISHELL_SOURCE"; zstyle -s "$SELFISHELL_TEST_CONTEXT" fzf-preview command_string || exit 10; print -rn -- "$command_string"`, "PATH="+filepath.Join(home, "bin")+":"+nativePath, "SELFISHELL_SOURCE="+nativeCommon(), "SELFISHELL_TEST_CONTEXT="+context)
		if len(r.Stdout) == 0 {
			t.Fatalf("empty %s preview", name)
		}
		commands[name] = string(r.Stdout)
	}
	return commands
}
func previewRun(t *testing.T, home, dir, code, word, realpath, path string) capture {
	t.Helper()
	r, err := runCommandIn(home, dir, []string{nativeZsh, "-f", "-c", code, "zsh"}, nil, []string{"PATH=" + path, "word=" + word, "realpath=" + realpath, "ZDOTDIR=", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null"}, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func TestNativeFzfTabPreviewsWaitForPlugin(t *testing.T) {
	t.Parallel()
	home := previewFixture(t)
	r := nativeRun(t, home, `source "$SELFISHELL_SOURCE"; zstyle -L ':fzf-tab:complete:*' fzf-preview || true`, "PATH="+filepath.Join(home, "bin")+":"+nativePath, "SELFISHELL_SOURCE="+nativeCommon())
	nativeQuiet(t, r)
}
func TestNativeFzfTabPreviewContexts(t *testing.T) {
	t.Parallel()
	home := previewFixture(t)
	commands := previewCommands(t, home)
	if len(commands) != 6 {
		t.Fatalf("commands: %v", commands)
	}
}
func TestNativeFzfTabPreviewFallbacks(t *testing.T) {
	t.Parallel()
	home := previewFixture(t)
	commands := previewCommands(t, home)
	sandbox := filepath.Join(home, "sandbox", "a dir")
	mustFS(t, os.MkdirAll(sandbox, 0700))
	nativeWrite(t, filepath.Join(sandbox, "a file.txt"), "one\ntwo\n", 0600)
	bin := filepath.Join(home, "restricted-bin")
	mustFS(t, os.Mkdir(bin, 0700))
	for _, name := range []string{"git", "ps", "head", "ls"} {
		p, err := exec.LookPath(name)
		if err != nil {
			t.Fatal(err)
		}
		mustFS(t, os.Symlink(p, filepath.Join(bin, name)))
	}
	cases := []struct {
		name, word, realpath, contains string
		empty                          bool
	}{{"directory", "", sandbox, "a file.txt", false}, {"file", "", filepath.Join(sandbox, "a file.txt"), "two", false}, {"directory", "", filepath.Join(sandbox, "gone"), "", true}, {"branch", "main", "", "", true}, {"diff", "a file.txt", "", "", true}, {"stash", "stash@{0}", "", "", true}, {"process", "0", "", "", false}}
	for i, tc := range cases {
		t.Run(tc.name+string(rune('0'+i)), func(t *testing.T) {
			r := previewRun(t, home, sandbox, commands[tc.name], tc.word, tc.realpath, bin)
			if len(r.Stderr) != 0 {
				t.Fatalf("stderr %q", r.Stderr)
			}
			out := string(r.Stdout)
			if tc.empty && out != "" {
				t.Fatalf("unexpected output %q", out)
			}
			if tc.contains != "" && !strings.Contains(out, tc.contains) {
				t.Fatalf("output %q lacks %q", out, tc.contains)
			}
		})
	}
}
func TestNativeFzfTabFilePreviewTheme(t *testing.T) {
	t.Parallel()
	for _, tool := range []string{"bat", "batcat"} {
		t.Run(tool, func(t *testing.T) {
			t.Parallel()
			home := previewFixture(t)
			command := previewCommands(t, home)["file"]
			bin := fakeExecutable(t, home, tool, `printf '%s\n' "$@"`)
			file := filepath.Join(home, "notes.txt")
			nativeWrite(t, file, "note\n", 0600)
			for theme, want := range map[string]string{"": "--theme=ansi", "Nord": "--theme=Nord"} {
				r, err := runCommandIn(home, home, []string{nativeZsh, "-f", "-c", command, "zsh"}, nil, []string{"PATH=" + bin, "realpath=" + file, "BAT_THEME=" + theme}, 10*time.Second)
				if err != nil || r.Status != 0 || len(r.Stderr) != 0 {
					t.Fatalf("BAT_THEME=%q: %+v %v", theme, r, err)
				}
				if !strings.Contains("\n"+string(r.Stdout), "\n"+want+"\n") {
					t.Fatalf("BAT_THEME=%q: %q lacks %q", theme, r.Stdout, want)
				}
			}
		})
	}
}
func gitPreview(t *testing.T, home, repo string, args ...string) string {
	t.Helper()
	r, err := runCommandIn(home, repo, append([]string{"/usr/bin/git"}, args...), nil, []string{"PATH=" + nativePath, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_AUTHOR_NAME=selfishell", "GIT_AUTHOR_EMAIL=selfishell@example.invalid", "GIT_COMMITTER_NAME=selfishell", "GIT_COMMITTER_EMAIL=selfishell@example.invalid", "GIT_CONFIG_NOSYSTEM=1"}, 10*time.Second)
	if err != nil || r.Status != 0 {
		t.Fatalf("git %v: %+v %v", args, r, err)
	}
	return string(r.Stdout)
}
func TestNativeFzfTabGitPreviews(t *testing.T) {
	t.Parallel()
	home := previewFixture(t)
	commands := previewCommands(t, home)
	repo := filepath.Join(home, "a repository")
	mustFS(t, os.Mkdir(repo, 0700))
	gitPreview(t, home, repo, "init", "-q", "-b", "main")
	nativeWrite(t, filepath.Join(repo, "staged only.txt"), "base\n", 0600)
	nativeWrite(t, filepath.Join(repo, "worktree only.txt"), "base\n", 0600)
	gitPreview(t, home, repo, "add", "-A")
	gitPreview(t, home, repo, "commit", "-q", "-m", "record the first revision")
	gitPreview(t, home, repo, "branch", "feature/login")
	gitPreview(t, home, repo, "branch", "feature/main")
	gitPreview(t, home, repo, "update-ref", "refs/remotes/origin/release-2", "HEAD")
	gitPreview(t, home, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/release-2")
	gitPreview(t, home, repo, "update-ref", "refs/remotes/origin/release-3", "HEAD")
	gitPreview(t, home, repo, "commit", "-q", "--allow-empty", "-m", "drop the unused flag")
	gitPreview(t, home, repo, "update-ref", "refs/remotes/upstream/release-3", "HEAD")
	branch := func(word string) string {
		t.Helper()
		r := previewRun(t, home, repo, commands["branch"], word, "", nativePath)
		if len(r.Stderr) != 0 {
			t.Fatalf("branch %s stderr %q", word, r.Stderr)
		}
		return string(r.Stdout)
	}
	login := branch("feature/login")
	if !strings.Contains(login, "record the first revision") || strings.Contains(login, "drop the unused flag") {
		t.Fatalf("feature/login %q", login)
	}
	if !strings.Contains(branch("release-2"), "record the first revision") {
		t.Fatal("remote only branch unresolved")
	}
	for _, word := range []string{"main", "HEAD"} {
		if !strings.Contains(branch(word), "drop the unused flag") {
			t.Fatalf("%s resolved wrong ref", word)
		}
	}
	first := strings.TrimSpace(gitPreview(t, home, repo, "rev-parse", "main~1"))
	gitPreview(t, home, repo, "tag", "v1.0", first)
	gitPreview(t, home, repo, "update-ref", "refs/remotes/origin/release-4", "HEAD")
	gitPreview(t, home, repo, "update-ref", "refs/remotes/upstream/release-4", "HEAD")
	gitPreview(t, home, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	// Paths named like refs must not replace or hide the resolved commit.
	nativeWrite(t, filepath.Join(repo, "v1.0"), "tag-named file\n", 0600)
	nativeWrite(t, filepath.Join(repo, "release-3"), "branch-named file\n", 0600)
	nativeWrite(t, filepath.Join(repo, "[m]ain"), "glob-named file\n", 0600)
	gitPreview(t, home, repo, "add", "v1.0", "release-3", ":(literal)[m]ain")
	gitPreview(t, home, repo, "commit", "-q", "-m", "add ref-named files")
	for _, word := range []string{"origin/release-2", "v1.0", first, "HEAD~2"} {
		if got := branch(word); !strings.Contains(got, "record the first revision") || strings.Contains(got, "drop the unused flag") {
			t.Fatalf("%s resolved wrong ref: %q", word, got)
		}
	}
	// Ambiguous or missing refs preview nothing, even when a path matches, every
	// remote candidate has the same commit, or the word is a ref glob.
	for _, word := range []string{"release-3", "release-4", "staged only.txt", "--quiet", "[m]ain"} {
		if got := branch(word); got != "" {
			t.Fatalf("unresolved %s: %q", word, got)
		}
	}
	nativeWrite(t, filepath.Join(repo, "staged only.txt"), "base\nstaged-change\n", 0600)
	gitPreview(t, home, repo, "add", "staged only.txt")
	nativeWrite(t, filepath.Join(repo, "worktree only.txt"), "base\nworktree-change\n", 0600)
	diff := func(word string) string {
		t.Helper()
		r := previewRun(t, home, repo, commands["diff"], word, "", nativePath)
		if len(r.Stderr) != 0 {
			t.Fatalf("diff stderr %q", r.Stderr)
		}
		return string(r.Stdout)
	}
	if !strings.Contains(diff("worktree only.txt"), "worktree-change") {
		t.Fatal("unstaged diff absent")
	}
	if strings.Contains(diff("staged only.txt"), "staged-change") {
		t.Fatal("staged change leaked")
	}
	gitPreview(t, home, repo, "stash", "push", "-q", "-m", "set the parser aside")
	r := previewRun(t, home, repo, commands["stash"], "stash@{0}", "", nativePath)
	if len(r.Stderr) != 0 || !strings.Contains(string(r.Stdout), "worktree-change") {
		t.Fatalf("stash %+v", r)
	}
}
