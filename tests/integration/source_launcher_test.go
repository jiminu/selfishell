package integration_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jiminu/selfishell/internal/testutil"
)

func TestDeveloperScriptsGoPolicy(t *testing.T) {
	t.Parallel()
	const pinnedVersion = "1.23.4"
	for _, mode := range []string{"missing", "wrong", "controlled"} {
		for _, tc := range []struct {
			script string
			args   []string
		}{
			{"build-cli.sh", nil},
			{"build-release.sh", []string{"--version", "1.2.3"}},
			{"benchmark.sh", []string{"--mode", "base"}},
			{"verify-published-release.sh", []string{"1.2.3"}},
			{"update-dependencies.sh", nil},
		} {
			t.Run(mode+"/"+tc.script, func(t *testing.T) {
				root, home, _ := sourceLauncherFixture(t)
				mustFS(t, copyTree(filepath.Join(repoRoot(), "scripts"), filepath.Join(root, "scripts")))
				mustFS(t, copyFile(filepath.Join(repoRoot(), "dependencies.conf"), filepath.Join(root, "dependencies.conf")))
				mustFS(t, testutil.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.invalid/test\n\ngo "+pinnedVersion+"\n"), 0600))
				bin, temp := t.TempDir(), t.TempDir()
				for _, name := range []string{"dirname", "awk", "mkdir", "mktemp", "rm"} {
					path, err := exec.LookPath(name)
					mustFS(t, err)
					mustFS(t, os.Symlink(path, filepath.Join(bin, name)))
				}
				for _, name := range []string{"gh", "curl"} {
					mustFS(t, testutil.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nexit 72\n"), 0700))
				}
				if mode != "missing" {
					mustFS(t, testutil.WriteFile(filepath.Join(bin, "go"), []byte(`#!/bin/sh
case "$1 $2" in
  'env GOVERSION')
    printf '%s\n' "$GOTOOLCHAIN|$GOENV|$GOWORK" >"$TEST_GO_CHECK"
    printf '%s\n' "$TEST_GO_VERSION" ;;
  'env GOHOSTOS') printf '%s\n' "$TEST_HOST_OS" ;;
  'env GOHOSTARCH') printf '%s\n' "$TEST_HOST_ARCH" ;;
  'build '*)
    printf '%s\n' "GOTOOLCHAIN=$GOTOOLCHAIN" "GO111MODULE=$GO111MODULE" "GOENV=$GOENV" "GOWORK=$GOWORK" "GOFLAGS=$GOFLAGS" "GOOS=$GOOS" "GOARCH=$GOARCH" "CGO_ENABLED=$CGO_ENABLED" "GOAMD64=$GOAMD64" "GOARM64=$GOARM64" "GOEXPERIMENT=$GOEXPERIMENT" "GOPROXY=$GOPROXY" "GOSUMDB=$GOSUMDB" "GOCACHE=$GOCACHE" "GOMODCACHE=$GOMODCACHE" >"$TEST_BUILD_ENV"
    exit 73 ;;
  *) exit 74 ;;
esac
`), 0700))
				}
				version := "go" + pinnedVersion
				if mode == "wrong" {
					version = "go0.0.0"
				}
				log := filepath.Join(root, "build-env")
				check := filepath.Join(root, "version-env")
				cache, modules := filepath.Join(home, "go-cache"), filepath.Join(home, "go-modcache")
				env := append(maintenanceMiseEnv(home), "PATH="+bin, "TMPDIR="+temp, "TEST_GO_VERSION="+version, "TEST_HOST_OS="+runtime.GOOS, "TEST_HOST_ARCH="+runtime.GOARCH, "TEST_BUILD_ENV="+log, "TEST_GO_CHECK="+check,
					"GOTOOLCHAIN=auto", "GO111MODULE=off", "GOENV="+filepath.Join(home, "user-go-env"), "GOWORK="+filepath.Join(home, "foreign.work"), "GOFLAGS=-tags=unapproved", "GOOS=plan9", "GOARCH=386", "CGO_ENABLED=1", "GOAMD64=v4", "GOARM64=v9.5", "GOEXPERIMENT=unapproved", "GOPROXY=https://proxy.invalid", "GOSUMDB=unapproved", "GOCACHE="+cache, "GOMODCACHE="+modules)
				args := append([]string{"/bin/bash", filepath.Join(root, "scripts", tc.script)}, tc.args...)
				got, err := runCommandIn(home, home, args, nil, env, 5*time.Second)
				mustFS(t, err)
				if mode != "missing" && string(readBytes(t, check)) != "local|off|off\n" {
					t.Fatal("version check inherited toolchain or workspace selection")
				}
				if mode == "controlled" {
					requireStatus(t, "compiler failure propagated", got, 73)
					want := "GOTOOLCHAIN=local\nGO111MODULE=on\nGOENV=off\nGOWORK=off\nGOFLAGS=\nGOOS=\nGOARCH=\nCGO_ENABLED=0\nGOAMD64=v1\nGOARM64=v8.0\nGOEXPERIMENT=none\nGOPROXY=off\nGOSUMDB=off\nGOCACHE=" + cache + "\nGOMODCACHE=" + modules + "\n"
					if tc.script == "build-cli.sh" {
						want = strings.Replace(want, "GOOS=\nGOARCH=\n", "GOOS="+runtime.GOOS+"\nGOARCH="+runtime.GOARCH+"\n", 1)
					}
					if data := string(readBytes(t, log)); data != want {
						t.Fatalf("compiler environment:\ngot %s\nwant %s", data, want)
					}
				} else {
					if got.Status != 1 || !strings.Contains(string(got.Stderr), "Go "+pinnedVersion) {
						t.Fatalf("toolchain rejection: %d %s", got.Status, got.Stderr)
					}
					requireAbsent(t, log)
					requireAbsent(t, filepath.Join(root, ".build"))
				}
				entries, err := os.ReadDir(temp)
				mustFS(t, err)
				if len(entries) != 0 {
					t.Fatalf("temporary build files retained: %v", entries)
				}
			})
		}
	}
}

// Copy the real tracked entrypoint so absence tests never touch the checkout's
// shared .build directory, which benchmark tests may use in parallel.
func sourceLauncherFixture(t *testing.T) (string, string, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "source with spaces")
	home := filepath.Join(t.TempDir(), "private home")
	mustFS(t, os.MkdirAll(filepath.Join(root, "bin"), 0700))
	mustFS(t, os.MkdirAll(home, 0700))
	entry := filepath.Join(root, "bin", "selfishell")
	mustFS(t, copyFile(filepath.Join(repoRoot(), "bin", "selfishell"), entry))
	return root, home, entry
}

func TestSourceLauncherRequiresExplicitBuild(t *testing.T) {
	t.Parallel()
	root, home, entry := sourceLauncherFixture(t)
	got, err := runCommandIn(home, home, []string{entry, "version"}, nil, maintenanceMiseEnv(home), 5*time.Second)
	mustFS(t, err)
	if got.Status == 0 || len(got.Stdout) != 0 || !strings.Contains(string(got.Stderr), "bash scripts/build-cli.sh") {
		t.Fatalf("missing binary: status=%d stdout=%q stderr=%q", got.Status, got.Stdout, got.Stderr)
	}
	if _, err := os.Lstat(filepath.Join(root, ".build")); !os.IsNotExist(err) {
		t.Fatalf("launcher built implicitly: %v", err)
	}
}

func TestSourceLauncherForwardsThroughSymlinkFromHostileCWD(t *testing.T) {
	t.Parallel()
	root, home, entry := sourceLauncherFixture(t)
	mustFS(t, os.MkdirAll(filepath.Join(root, ".build"), 0700))
	binary := filepath.Join(root, ".build", "selfishell")
	mustFS(t, testutil.WriteFile(binary, []byte("#!/bin/sh\nprintf 'arg:%s\\n' \"$1\"\ncat\nprintf 'error\\n' >&2\nexit 17\n"), 0700))
	link := filepath.Join(home, "sfs")
	mustFS(t, os.Symlink(entry, link))
	got, err := runCommandIn(home, home, []string{link, "argument with spaces"}, []byte("input\n"), maintenanceMiseEnv(home), 5*time.Second)
	mustFS(t, err)
	if got.Status != 17 || string(got.Stdout) != "arg:argument with spaces\ninput\n" || string(got.Stderr) != "error\n" {
		t.Fatalf("forwarding: status=%d stdout=%q stderr=%q", got.Status, got.Stdout, got.Stderr)
	}
}
