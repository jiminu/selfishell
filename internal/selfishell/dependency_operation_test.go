package selfishell

import (
	"bytes"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/jiminu/selfishell/internal/managed"
	"github.com/jiminu/selfishell/internal/process"
	"github.com/jiminu/selfishell/internal/testutil"
)

func dependencyFixture(t *testing.T, env *process.Environment) (*PackageOperation, managed.Paths, string, string) {
	t.Helper()
	home := t.TempDir()
	testutil.SetEnv(env, "HOME", home)
	testutil.SetEnv(env, "XDG_CONFIG_HOME", home+"/.config")
	testutil.SetEnv(env, "XDG_DATA_HOME", home+"/data")
	testutil.SetEnv(env, "XDG_STATE_HOME", home+"/state")
	testutil.SetEnv(env, "XDG_CACHE_HOME", home+"/cache")
	testutil.SetEnv(env, "MISE_DATA_DIR", home+"/mise/data")
	testutil.SetEnv(env, "MISE_STATE_DIR", home+"/mise/state")
	testutil.SetEnv(env, "MISE_CACHE_DIR", home+"/mise/cache")
	testutil.SetEnv(env, "MISE_CONFIG_DIR", home+"/.config/mise")
	testutil.SetEnv(env, "MISE_TRUSTED_CONFIG_PATHS", "")
	paths, err := managed.UserPaths(env.Get)
	if err != nil {
		t.Fatal(err)
	}
	manifest := home + "/dependencies.conf"
	return &PackageOperation{Process: process.Process{Inherit: *env, Out: new(bytes.Buffer), Err: new(bytes.Buffer)}}, paths, manifest, home
}

func TestManagedRawDownloadRestoresChangedBytesAndSkipsUnchangedTransfer(t *testing.T) {
	t.Parallel()
	var env process.Environment
	op, paths, manifest, home := dependencyFixture(t, &env)
	source := home + "/source"
	directDownload(t, manifest, source, "1.0", ".local/bin/tool", false)
	target := home + "/.local/bin/tool"
	if err := installTool(op, paths, manifest); err != nil {
		t.Fatal(err)
	}
	approved := testutil.ReadTestFile(t, target)
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if err := installTool(op, paths, manifest); err != nil || op.UnchangedCount != 1 {
		t.Fatalf("unchanged target fetched source: %v, count=%d", err, op.UnchangedCount)
	}
	testutil.WriteTestFile(t, target, "#!/bin/sh\necho changed\n", 0755)
	testutil.WriteTestFile(t, source, approved, 0600)
	if err := installTool(op, paths, manifest); err != nil {
		t.Fatal(err)
	}
	if got := testutil.ReadTestFile(t, target); got != approved {
		t.Fatalf("managed bytes not restored: %q", got)
	}
	if op.UnchangedCount != 1 {
		t.Fatalf("changed executable counted unchanged: %d", op.UnchangedCount)
	}
}

func TestDirectDownloadPassesProxyAndTransferPolicy(t *testing.T) {
	t.Parallel()
	var env process.Environment
	op, paths, manifest, home := dependencyFixture(t, &env)
	testutil.SetEnv(&env, "TMPDIR", t.TempDir())
	bin := home + "/fake-bin"
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	if err := testutil.WriteFile(bin+"/curl", []byte("#!/bin/sh\nprintf '%s' \"$HTTPS_PROXY\" >\"$HOME/proxy-observed\"\nprintf '%s\\n' \"$@\" >\"$HOME/curl-arguments\"\nexec /usr/bin/curl \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	testutil.SetEnv(&env, "PATH", bin)
	testutil.SetEnv(&env, "HTTPS_PROXY", "http://proxy.example:8443")
	op.Process.Inherit = env
	directDownload(t, manifest, home+"/source", "1.0", ".local/bin/tool", false)
	if err := installTool(op, paths, manifest); err != nil {
		t.Fatal(err)
	}
	if got := testutil.ReadTestFile(t, home+"/proxy-observed"); got != "http://proxy.example:8443" {
		t.Fatal(got)
	}
	args := strings.Split(strings.TrimSpace(testutil.ReadTestFile(t, home+"/curl-arguments")), "\n")
	for _, flag := range []string{"--connect-timeout", "--speed-limit", "--speed-time", "--retry", "--retry-max-time"} {
		if !slices.Contains(args, flag) {
			t.Fatalf("missing %s: %q", flag, args)
		}
	}
	if slices.Contains(args, "--max-time") {
		t.Fatalf("transfer has total timeout: %q", args)
	}
}
