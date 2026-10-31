package selfishell

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiminu/selfishell/internal/process"
	"github.com/jiminu/selfishell/internal/testutil"
)

func TestCLI(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".git"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		args     []string
		code     int
		out, err string
	}{
		{[]string{"version"}, 0, "selfishell development\n", ""},
		{[]string{"--version"}, 0, "selfishell development\n", ""},
		{[]string{"-v"}, 0, "selfishell development\n", ""},
		{[]string{"version", "--help"}, 0, "Usage: selfishell version [--available]\n", ""},
		{[]string{"version", "help", "extra"}, 0, "Usage: selfishell version [--available]\n", ""},
		{[]string{"version", "", "ignored"}, 0, "selfishell development\n", ""},
		{[]string{"version", "-h", "ignored"}, 0, "Usage: selfishell version [--available]\n", ""},
		{[]string{"version", "extra"}, 2, "", "selfishell: Usage: selfishell version [--available]\n"},
		{[]string{"help", ""}, 2, "", "selfishell: help does not accept arguments\n"},
		{[]string{"version", "--available", "extra"}, 2, "", "selfishell: Usage: selfishell version [--available]\n"},
		{[]string{"wat"}, 2, "", "selfishell: Unknown command: wat\nselfishell: Run 'selfishell help' to see available commands.\n"},
	} {
		t.Run(strings.Join(tc.args, "/"), func(t *testing.T) {
			t.Parallel()
			var out, stderr bytes.Buffer
			c := CLI{Root: root, Out: &out, Err: &stderr}
			if code := c.Run(tc.args); code != tc.code || out.String() != tc.out || stderr.String() != tc.err {
				t.Fatalf("got (%d, %q, %q), want (%d, %q, %q)", code, out.String(), stderr.String(), tc.code, tc.out, tc.err)
			}
		})
	}
}

func TestVersionAndRoot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(bin, "selfishell")
	if err := os.WriteFile(executable, nil, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "sfs")
	if err := os.Symlink(executable, link); err != nil {
		t.Fatal(err)
	}
	chained := link + "-chain"
	if err := os.Symlink("sfs", chained); err != nil {
		t.Fatal(err)
	}
	got, err := ReleaseRoot(chained)
	resolved, _ := filepath.EvalSymlinks(root)
	if err != nil || got != resolved {
		t.Fatalf("root=%q, err=%v", got, err)
	}
	if _, err := ReleaseRoot(link + "-missing"); err == nil {
		t.Fatal("missing executable accepted")
	}
	var out, stderr bytes.Buffer
	c := CLI{Root: root, Out: &out, Err: &stderr}
	if c.Run([]string{"version"}) != 1 || !strings.Contains(stderr.String(), "Version file not found: "+root+"/VERSION") {
		t.Fatal(stderr.String())
	}
	for _, contents := range []string{"1.2.3\n\n", " v1 \r\n", "1\x002\n", ""} {
		if err := os.WriteFile(filepath.Join(root, "VERSION"), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
		out.Reset()
		stderr.Reset()
		want := "selfishell " + strings.TrimRight(strings.ReplaceAll(contents, "\x00", ""), "\n") + "\n"
		if c.Run([]string{"version"}) != 0 || out.String() != want {
			t.Fatalf("got %q, want %q", out.String(), want)
		}
	}
}

func TestVersionAvailableCLI(t *testing.T) {
	t.Parallel()
	var env process.Environment
	setEnv(&env, "HOME", t.TempDir())
	root := t.TempDir()
	setEnv(&env, "SELFISHELL_RELEASE_ROOT", "file://"+root)
	os.MkdirAll(root+"/latest/download", 0700)
	testutil.WriteFile(root+"/latest/download/VERSION", []byte("1.2.3\n"), 0600)
	var out, stderr bytes.Buffer
	c := CLI{Env: env, Root: root, Out: &out, Err: &stderr}
	if code := c.Run([]string{"version", "--available"}); code != 0 || out.String() != "1.2.3\n" {
		t.Fatalf("code %d, stdout %q, stderr %q", code, out.String(), stderr.String())
	}
	testutil.WriteFile(root+"/latest/download/VERSION", []byte("unsafe\n"), 0600)
	out.Reset()
	stderr.Reset()
	if code := c.Run([]string{"version", "--available"}); code != 1 || out.Len() != 0 || !strings.Contains(stderr.String(), "Unable to determine the latest Selfishell release") {
		t.Fatal(fmt.Sprintf("code %d, stdout %q, stderr %q", code, out.String(), stderr.String()))
	}
}
