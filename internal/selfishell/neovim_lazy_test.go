package selfishell

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiminu/selfishell/internal/managed"
	"github.com/jiminu/selfishell/internal/process"
	"github.com/jiminu/selfishell/internal/report"
)

// lazyFixture is a committed lazy.nvim source, the pin naming it, and an
// operation that reports into its Out buffer.
func lazyFixture(t *testing.T) (neovimOperation, managed.Paths, string, managed.Dependency) {
	t.Helper()
	home := t.TempDir()
	var env process.Environment
	setEnv(&env, "HOME", home)
	setEnv(&env, "XDG_CONFIG_HOME", home+"/.config")
	setEnv(&env, "XDG_DATA_HOME", home+"/data")
	paths, err := managed.UserPaths(env.Get)
	if err != nil {
		t.Fatal(err)
	}
	repo := home + "/lazy-source"
	if err := os.MkdirAll(repo, 0700); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, repo, "init", "--quiet")
	writeTestFile(t, repo+"/init.lua", "return true\n", 0600)
	gitCommand(t, repo, "add", ".")
	gitCommand(t, repo, "commit", "--quiet", "-m", "initial")
	head := gitCommand(t, repo, "rev-parse", "HEAD")
	out := new(bytes.Buffer)
	op := neovimOperation{
		Process: process.Process{Inherit: env, Out: out, Err: new(bytes.Buffer)},
		Report: report.ReportFunc(func(_ string, _ report.Tone, format string, args ...any) {
			fmt.Fprintf(out, format+"\n", args...)
		}),
		Warn: func(string) {},
	}
	return op, paths, home, managed.Dependency{Kind: "nvim-plugin", Name: "folke/lazy.nvim", Version: head, Platform: "all", Arch: "all", Source: repo, Checksum: "-", Target: "-", Marker: "-"}
}

func TestLazyRevisionUpdateReplacesCleanCheckout(t *testing.T) {
	t.Parallel()
	op, paths, home, lazy := lazyFixture(t)
	target := home + "/data/selfishell/nvim/lazy/lazy.nvim"
	if err := os.MkdirAll(home+"/data/selfishell/nvim/lazy", 0700); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, home, "clone", "-q", home+"/lazy-source", target)
	writeTestFile(t, home+"/lazy-source/init.lua", "next\n", 0600)
	gitCommand(t, home+"/lazy-source", "add", ".")
	gitCommand(t, home+"/lazy-source", "commit", "-qm", "next")
	newHead := gitCommand(t, home+"/lazy-source", "rev-parse", "HEAD")
	dep := lazy
	dep.Version = newHead
	if err := op.installLazy(context.Background(), paths, dep, nil); err != nil {
		t.Fatal(err)
	}
	if got := gitCommand(t, target, "rev-parse", "HEAD"); got != newHead {
		t.Fatalf("revision: %s", got)
	}
	if !strings.Contains(op.Process.Out.(*bytes.Buffer).String(), "Updated approved lazy.nvim revision") {
		t.Fatal("update not reported")
	}
}

func TestLazyRestoreFailureIncludesPrimaryActivationError(t *testing.T) {
	home := t.TempDir()
	old, target := home+"/previous", home+"/lazy.nvim"
	writeTestFile(t, old, "previous", 0600)
	writeTestFile(t, target, "occupied", 0600)
	primary := errors.New("activation failed")
	err := restoreLazyOnFailure(old, target, primary)
	if !errors.Is(err, primary) || !strings.Contains(err.Error(), "cannot restore over occupied target") {
		t.Fatalf("activation or restoration error lost: %v", err)
	}
	if data, readErr := os.ReadFile(old); readErr != nil || string(data) != "previous" {
		t.Fatalf("previous checkout changed: %q %v", data, readErr)
	}
}

func TestLazyPreservesMalformedTargetAndBlockedParent(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ shape, want string }{
		{"file", "preserving"},
		{"git_symlink", "preserving"},
		{"blocked_parent", "Could not create Neovim plugin directory"},
	} {
		t.Run(tc.shape, func(t *testing.T) {
			t.Parallel()
			op, paths, home, lazy := lazyFixture(t)
			target := home + "/data/selfishell/nvim/lazy/lazy.nvim"
			user := map[string]string{"file": target, "git_symlink": target + "/user-file", "blocked_parent": filepath.Dir(target)}[tc.shape]
			writeTestFile(t, user, "user data", 0600)
			if tc.shape == "git_symlink" {
				if err := os.Symlink(home+"/lazy-source/.git", target+"/.git"); err != nil {
					t.Fatal(err)
				}
			}
			if err := op.installLazy(context.Background(), paths, lazy, nil); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
			if readTestFile(t, user) != "user data" {
				t.Fatal("user data changed")
			}
		})
	}
}
