package selfishell

import (
	"bytes"
	"os"
	"slices"
	"strings"
	"testing"
)

func dependencyFixture(t *testing.T) (*PackageOperation, Paths, string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home+"/.config")
	t.Setenv("XDG_DATA_HOME", home+"/data")
	t.Setenv("XDG_STATE_HOME", home+"/state")
	t.Setenv("XDG_CACHE_HOME", home+"/cache")
	t.Setenv("MISE_DATA_DIR", home+"/mise/data")
	t.Setenv("MISE_STATE_DIR", home+"/mise/state")
	t.Setenv("MISE_CACHE_DIR", home+"/mise/cache")
	t.Setenv("MISE_CONFIG_DIR", home+"/.config/mise")
	t.Setenv("MISE_TRUSTED_CONFIG_PATHS", "")
	paths, err := UserPaths()
	if err != nil {
		t.Fatal(err)
	}
	manifest := home + "/dependencies.conf"
	return &PackageOperation{Process: Process{Out: new(bytes.Buffer), Err: new(bytes.Buffer)}}, paths, manifest, home
}

func TestManagedRawDownloadRestoresChangedBytesAndSkipsUnchangedTransfer(t *testing.T) {
	op, paths, manifest, home := dependencyFixture(t)
	source := home + "/source"
	directDownload(t, manifest, source, "1.0", ".local/bin/tool", false)
	target := home + "/.local/bin/tool"
	if err := installTool(op, paths, manifest); err != nil {
		t.Fatal(err)
	}
	approved := readTestFile(t, target)
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if err := installTool(op, paths, manifest); err != nil || op.UnchangedCount != 1 {
		t.Fatalf("unchanged target fetched source: %v, count=%d", err, op.UnchangedCount)
	}
	writeTestFile(t, target, "#!/bin/sh\necho changed\n", 0755)
	writeTestFile(t, source, approved, 0600)
	if err := installTool(op, paths, manifest); err != nil {
		t.Fatal(err)
	}
	if got := readTestFile(t, target); got != approved {
		t.Fatalf("managed bytes not restored: %q", got)
	}
	if op.UnchangedCount != 1 {
		t.Fatalf("changed executable counted unchanged: %d", op.UnchangedCount)
	}
}

func TestDirectDownloadPassesProxyAndTransferPolicy(t *testing.T) {
	op, paths, manifest, home := dependencyFixture(t)
	t.Setenv("TMPDIR", t.TempDir())
	bin := home + "/fake-bin"
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin+"/curl", []byte("#!/bin/sh\nprintf '%s' \"$HTTPS_PROXY\" >\"$HOME/proxy-observed\"\nprintf '%s\\n' \"$@\" >\"$HOME/curl-arguments\"\nexec /usr/bin/curl \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("HTTPS_PROXY", "http://proxy.example:8443")
	directDownload(t, manifest, home+"/source", "1.0", ".local/bin/tool", false)
	if err := installTool(op, paths, manifest); err != nil {
		t.Fatal(err)
	}
	if got := readTestFile(t, home+"/proxy-observed"); got != "http://proxy.example:8443" {
		t.Fatal(got)
	}
	args := strings.Split(strings.TrimSpace(readTestFile(t, home+"/curl-arguments")), "\n")
	for _, flag := range []string{"--connect-timeout", "--speed-limit", "--speed-time", "--retry", "--retry-max-time"} {
		if !slices.Contains(args, flag) {
			t.Fatalf("missing %s: %q", flag, args)
		}
	}
	if slices.Contains(args, "--max-time") {
		t.Fatalf("transfer has total timeout: %q", args)
	}
}
