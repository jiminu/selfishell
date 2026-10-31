package selfishell

import (
	"os"
	"testing"

	"github.com/jiminu/selfishell/internal/process"
	"github.com/jiminu/selfishell/internal/testutil"
)

func TestPlatform(t *testing.T) {
	t.Parallel()
	var env process.Environment
	root := t.TempDir()
	setEnv(&env, "SELFISHELL_TEST_OS_RELEASE_FILE", root+"/os-release")
	setEnv(&env, "SELFISHELL_TEST_PROC_VERSION_FILE", root+"/proc-version")
	for _, tc := range []struct{ system, arch, id, proc, platform, wantArch string }{
		{"Darwin", "arm64", "", "", "macos", "arm64"},
		{"Linux", "x86_64", "ubuntu", "Linux", "ubuntu", "amd64"},
		{"Linux", "aarch64", "\"ubuntu\"", "Microsoft WSL2", "ubuntu-wsl", "arm64"},
		{"Linux", "amd64", "'ubuntu'", "linux", "ubuntu", "amd64"},
		{"Linux", "riscv64", "fedora", "linux", "unsupported-linux", "riscv64"},
		{"Linux", "amd64", "debian", "WSL", "unsupported-wsl", "amd64"},
		{"FreeBSD", "amd64", "", "", "unsupported", "amd64"},
	} {
		setEnv(&env, "SELFISHELL_TEST_SYSTEM_NAME", tc.system)
		setEnv(&env, "SELFISHELL_TEST_MACHINE_ARCH", tc.arch)
		if err := testutil.WriteFile(root+"/os-release", []byte("NAME=ignored\nID="+tc.id+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := testutil.WriteFile(root+"/proc-version", []byte(tc.proc), 0600); err != nil {
			t.Fatal(err)
		}
		p := detectPlatform(env)
		if p.Name != tc.platform || p.Arch != tc.wantArch {
			t.Fatalf("got %+v, want %s/%s", p, tc.platform, tc.wantArch)
		}
	}
	setEnv(&env, "SELFISHELL_TEST_SYSTEM_NAME", "Linux")
	if err := os.Remove(root + "/os-release"); err != nil {
		t.Fatal(err)
	}
	if detectPlatform(env).Name != "unsupported-linux" {
		t.Fatal("missing distro was supported")
	}
}
