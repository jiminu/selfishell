package selfishell

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jiminu/selfishell/internal/testutil"
)

func TestNeovimGitRemote(t *testing.T) {
	t.Parallel()
	root := testRelease(t)
	nvim, ok := standaloneNeovimExecutable(t, root)
	if !ok {
		t.Skip("requires the pinned physical Neovim executable")
	}
	runNeovimGitRemote(t, root, nvim, os.Getenv("SELFISHELL_TEST_NVIM_PLUGIN_ROOT"))
}

func runNeovimGitRemote(t *testing.T, root, nvim, pluginRoot string) {
	t.Helper()
	p := withEnvironment(standaloneNeovimProcess(t, nvim), map[string]string{
		"SELFISHELL_NVIM_CONFIG":           root + "/config/shared/nvim",
		"SELFISHELL_TEST_NVIM_PLUGIN_ROOT": pluginRoot,
		"GIT_CONFIG_NOSYSTEM":              "1", "GIT_CONFIG_GLOBAL": "/dev/null",
	}, "WSL_DISTRO_NAME", "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE")
	home, err := filepath.EvalSymlinks(envValue(p.Env, "HOME"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	git := func(dir string, args ...string) string {
		t.Helper()
		command := p
		command.Dir = dir
		var output bytes.Buffer
		command.Out, command.Err = &output, &output
		args = append([]string{"-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false"}, args...)
		if code, err := command.Run(ctx, "git", args...); err != nil || code != 0 {
			t.Fatalf("git %v: %d %v %s", args, code, err, output.String())
		}
		return output.String()
	}
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := testutil.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if pluginRoot != "" {
		manifest, err := os.ReadFile(root + "/dependencies.conf")
		if err != nil {
			t.Fatal(err)
		}
		write(home+"/.config/nvim/plugin-versions.conf", string(manifest))
		deps, err := ReadDependencies(root + "/dependencies.conf")
		if err != nil {
			t.Fatal(err)
		}
		for _, dep := range deps {
			if dep.Name == "nvim-tree/nvim-tree.lua" || dep.Name == "nvim-lualine/lualine.nvim" {
				if got := strings.TrimSpace(git(pluginRoot+"/"+filepath.Base(dep.Name), "rev-parse", "HEAD")); got != dep.Version {
					t.Fatalf("UI probe requires approved %s pin", dep.Name)
				}
			}
		}
	}
	server, client := home+"/remote", home+"/client with spaces"
	git(home, "init", "-b", "main", server)
	for _, name := range []string{"src/change.ts", "deleted.ts", "old.ts", "local.ts", "odd\nname.ts"} {
		write(server+"/"+name, name+"\n")
	}
	git(server, "add", ".")
	git(server, "commit", "-m", "base")
	git(home, "clone", server, client)
	write(server+"/src/change.ts", "remote\n")
	write(server+"/odd\nname.ts", "remote\n")
	write(server+"/src/added.ts", "new\n")
	git(server, "rm", "deleted.ts")
	git(server, "mv", "old.ts", "renamed.ts")
	git(server, "add", ".")
	git(server, "commit", "-m", "remote changes")

	command := exec.CommandContext(ctx, nvim, "--headless", "-u", "NONE", "-i", "NONE", "-l", root+"/tests/fixtures/neovim/git_remote.lua")
	command.Env, command.Dir = p.Env, client
	var stderr bytes.Buffer
	command.Stderr = &stderr
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		stdin.Close()
		if err := command.Wait(); err != nil {
			t.Errorf("Neovim: %v %s", err, stderr.String())
		}
		if t.Failed() {
			t.Log(stderr.String())
		}
		cancel()
	}()
	scanner := bufio.NewScanner(stdout)
	type snapshot struct {
		Status string            `json:"status"`
		Files  map[string]string `json:"files"`
	}
	probe := func(path string, want snapshot, poll bool, treePath ...string) {
		t.Helper()
		var names []string
		for name := range want.Files {
			names = append(names, name)
		}
		request := map[string]any{"path": path, "files": names, "want": want, "poll": poll}
		if len(treePath) > 0 {
			request["tree_path"] = treePath[0]
		}
		data, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fmt.Fprintln(stdin, string(data)); err != nil {
			t.Fatal(err)
		}
		if !scanner.Scan() {
			t.Fatal("Neovim probe ended before reporting Git decorations")
		}
		var got struct {
			snapshot
			Tree string `json:"tree"`
			Line string `json:"line"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &got); err != nil || !reflect.DeepEqual(got.snapshot, want) {
			t.Fatalf("Git decoration: got %s want %+v: %v", scanner.Text(), want, err)
		}
		if pluginRoot != "" {
			status := strings.ReplaceAll(want.Status, " ", "")
			if status != "" && !strings.Contains(got.Line, status) || status == "" && strings.ContainsAny(got.Line, "↑↓") {
				t.Fatalf("statusline: %q, want count %q", got.Line, status)
			}
			incoming := false
			for name, badge := range want.Files {
				incoming = incoming || badge != ""
				if _, err := os.Stat(path + "/" + name); err == nil && badge != "" {
					if !strings.Contains(got.Tree, badge) {
						t.Fatalf("tree missing %s: %s", badge, got.Tree)
					}
				}
			}
			if !incoming && strings.Contains(got.Tree, "↓") {
				t.Fatalf("stale incoming badge: %s", got.Tree)
			}
		}
	}
	check := func(path, status string, incoming, poll bool, treePath ...string) {
		t.Helper()
		want := snapshot{status, map[string]string{}}
		for _, name := range []string{"src/change.ts", "src/added.ts", "deleted.ts", "old.ts", "local.ts", "odd\nname.ts", "src"} {
			want.Files[name] = ""
		}
		if incoming {
			want.Files["src/change.ts"], want.Files["odd\nname.ts"] = "↓M", "↓M"
			want.Files["src/added.ts"], want.Files["deleted.ts"], want.Files["old.ts"], want.Files["src"] = "↓A", "↓D", "↓R", "↓"
		}
		probe(path, want, poll, treePath...)
	}
	check(client, "", false, false) // Remote commits remain invisible until fetch.
	git(client, "fetch", "origin")
	check(client, "↓1", true, true) // External fetch refreshes an idle tree/statusline.
	write(client+"/local.ts", "local commit\n")
	git(client, "add", "local.ts")
	git(client, "commit", "-m", "local change")
	write(client+"/src/change.ts", "uncommitted\n")
	check(client, "↑1 ↓1", true, false)
	git(client, "switch", "-c", "no-upstream")
	check(client, "", false, false)
	git(client, "checkout", "--detach")
	check(client, "", false, false)
	git(client, "switch", "main")
	check(client, "↑1 ↓1", true, false)
	git(client, "reset", "--hard", "origin/main")
	check(client, "", false, false)
	git(client, "commit", "--allow-empty", "-m", "ahead only")
	check(client, "↑1", false, false)
	worktree := home + "/linked"
	git(client, "worktree", "add", "-b", "linked", worktree, "origin/main")
	check(worktree, "", false, false)
	git(worktree, "commit", "--allow-empty", "-m", "worktree ahead one")
	git(worktree, "commit", "--allow-empty", "-m", "worktree ahead two")
	if pluginRoot != "" {
		check(client, "↑1", false, false, worktree)
	}
	check(client, "↑1", false, false)
	check(home, "", false, false)

	// Rename similarity must not lazily download missing partial-clone blobs.
	git(server, "config", "uploadpack.allowFilter", "true")
	partial := home + "/partial"
	git(home, "clone", "--filter=blob:none", "file://"+server, partial)
	git(server, "mv", "renamed.ts", "newer.ts")
	write(server+"/newer.ts", "old.ts\nx\n")
	git(server, "add", ".")
	git(server, "commit", "-m", "rename with edit")
	git(partial, "fetch", "origin")
	missing := git(partial, "rev-list", "--objects", "--missing=print", "HEAD..origin/main")
	if !strings.Contains(missing, "?") {
		t.Fatal("partial clone fixture has no missing blob")
	}
	probe(partial, snapshot{"↓1", map[string]string{"renamed.ts": "↓D", "newer.ts": "↓A"}}, false)
	if after := git(partial, "rev-list", "--objects", "--missing=print", "HEAD..origin/main"); after != missing {
		t.Fatal("Git decoration downloaded missing partial-clone objects")
	}
}
