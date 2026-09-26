package selfishell

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func writeTestFile(t *testing.T, path, value string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), mode); err != nil {
		t.Fatal(err)
	}
}
func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
func directDownload(t *testing.T, manifest, source, version, target string, bad bool) {
	t.Helper()
	data := []byte("#!/bin/sh\necho " + version + "\n")
	writeTestFile(t, source, string(data), 0600)
	sum := sha256.Sum256(data)
	if bad {
		sum = sha256.Sum256([]byte("wrong"))
	}
	writeTestFile(t, manifest, fmt.Sprintf("download tool %s linux amd64 file://%s %x %s raw\n", version, source, sum, target), 0600)
}
func installTool(op *PackageOperation, paths Paths, manifest string) error {
	return op.InstallDirect(context.Background(), paths, manifest, "required", "tool", "linux", "amd64", false)
}
func output(op *PackageOperation) string { return op.Process.Out.(*bytes.Buffer).String() }
func assertNoPath(t *testing.T, path string) {
	t.Helper()
	if ok, err := present(path); err != nil || ok {
		t.Fatalf("unexpected path %s: %v", path, err)
	}
}

func TestDirectDownloadActivationAndStateFailures(t *testing.T) {
	for _, phase := range []string{"stage", "activate", "state"} {
		t.Run(phase, func(t *testing.T) {
			op, paths, manifest, home := dependencyFixture(t)
			target := home + "/.local/bin/tool"
			state := paths.State + "/dependencies/tool"
			directDownload(t, manifest, home+"/source", "1.0", ".local/bin/tool", false)
			writeTestFile(t, target, "old", 0755)
			writeTestFile(t, state, "0.9\n", 0600)
			op.dependencyFault = func(at string) error {
				if at == phase {
					return errors.New("forced " + at)
				}
				return nil
			}
			if err := installTool(op, paths, manifest); err == nil {
				t.Fatal("failure ignored")
			}
			if got := readTestFile(t, target); got != "old" {
				t.Fatalf("prior target lost: %q", got)
			}
			if got := readTestFile(t, state); got != "0.9\n" {
				t.Fatalf("prior state lost: %q", got)
			}
			if strings.Contains(output(op), "approved dependency") {
				t.Fatalf("success reported: %s", output(op))
			}
		})
	}
}

func TestDirectDownloadXDGAndManagedCases(t *testing.T) {
	for _, tc := range []struct{ name, target, version, old, shape string }{
		{"data_directory_dependency_targets_follow_xdg_data_home", ".local/share/tool/tool", "1.0", "", ""},
		{"managed_download_valid_target_is_already_approved", ".local/bin/tool", "1.0", "1.0", "valid"},
		{"managed_download_version_bump_reports_updated", ".local/bin/tool", "2.0", "1.0", "valid"},
		{"managed_download_broken_non_executable", ".local/bin/tool", "1.0", "1.0", "nonexecutable"},
		{"managed_download_broken_valid_symlink", ".local/bin/tool", "1.0", "1.0", "symlink"},
		{"managed_symlink_target_recovery_preserves_symlink_destination", ".local/bin/tool", "1.0", "1.0", "symlink"},
		{"download_dependency_replaces_directory_target_without_nesting", ".local/bin/tool", "1.0", "1.0", "directory"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			op, paths, manifest, home := dependencyFixture(t)
			directDownload(t, manifest, home+"/source", tc.version, tc.target, false)
			target := home + "/" + tc.target
			if strings.HasPrefix(tc.target, ".local/share/") {
				target = home + "/data/" + strings.TrimPrefix(tc.target, ".local/share/")
			}
			state := paths.State + "/dependencies/tool"
			if tc.old != "" {
				writeTestFile(t, state, tc.old+"\n", 0600)
			}
			switch tc.shape {
			case "valid":
				writeTestFile(t, target, "existing-install-marker", 0755)
			case "nonexecutable":
				writeTestFile(t, target, "broken", 0644)
			case "symlink":
				writeTestFile(t, home+"/user-file", "user owned content", 0755)
				os.MkdirAll(filepath.Dir(target), 0700)
				if err := os.Symlink(home+"/user-file", target); err != nil {
					t.Fatal(err)
				}
			case "directory":
				writeTestFile(t, target+"/leftover", "leftover", 0600)
			}
			if err := installTool(op, paths, manifest); err != nil {
				t.Fatal(err)
			}
			if tc.shape == "valid" && tc.old == tc.version {
				if output(op) != "" || readTestFile(t, target) != "existing-install-marker" || op.UnchangedCount != 1 {
					t.Fatalf("noop: %q", output(op))
				}
				return
			}
			if strings.HasPrefix(tc.name, "managed_download_version") {
				if !strings.Contains(output(op), "Updated approved dependency: tool 2.0") {
					t.Fatalf("update output: %q", output(op))
				}
			} else if !strings.Contains(output(op), "Installed approved dependency: tool "+tc.version) {
				t.Fatalf("install output: %q", output(op))
			}
			if readTestFile(t, state) != tc.version+"\n" {
				t.Fatal("wrong state")
			}
			if info, err := os.Lstat(target); err != nil || !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
				t.Fatalf("not executable regular target: %v %v", info, err)
			}
			if tc.shape == "symlink" && readTestFile(t, home+"/user-file") != "user owned content" {
				t.Fatal("symlink destination changed")
			}
			if tc.shape == "directory" && readTestFile(t, target) == "leftover" {
				t.Fatal("directory nested")
			}
			if strings.HasPrefix(tc.target, ".local/share/") {
				assertNoPath(t, home+"/.local/share/tool")
			}
		})
	}
}

func TestDirectDownloadExternalTargets(t *testing.T) {
	for _, shape := range []string{"valid_symlink", "dangling_symlink", "directory"} {
		t.Run(shape, func(t *testing.T) {
			op, paths, manifest, home := dependencyFixture(t)
			directDownload(t, manifest, home+"/source", "1.0", ".local/bin/tool", false)
			target := home + "/.local/bin/tool"
			if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
				t.Fatal(err)
			}
			switch shape {
			case "valid_symlink":
				writeTestFile(t, home+"/external", "external", 0755)
				os.Symlink(home+"/external", target)
			case "dangling_symlink":
				os.Symlink(home+"/missing", target)
			case "directory":
				writeTestFile(t, target+"/user-data", "user data", 0600)
			}
			err := installTool(op, paths, manifest)
			if shape == "valid_symlink" {
				if err != nil || !strings.Contains(output(op), "Externally installed; preserving:") {
					t.Fatalf("external valid: %v %q", err, output(op))
				}
			} else if err == nil {
				t.Fatal("invalid external target accepted")
			}
			if shape == "directory" && readTestFile(t, target+"/user-data") != "user data" {
				t.Fatal("external data changed")
			}
			if shape != "directory" {
				if info, err := os.Lstat(target); err != nil || info.Mode()&os.ModeSymlink == 0 {
					t.Fatalf("link changed: %v %v", info, err)
				}
			}
			assertNoPath(t, paths.State+"/dependencies/tool")
		})
	}
}

func TestDirectDownloadDirectoryRestoredOnActivationFailure(t *testing.T) {
	op, paths, manifest, home := dependencyFixture(t)
	directDownload(t, manifest, home+"/source", "1.0", ".local/bin/tool", false)
	target := home + "/.local/bin/tool"
	writeTestFile(t, target+"/leftover", "leftover", 0600)
	writeTestFile(t, paths.State+"/dependencies/tool", "0.9\n", 0600)
	op.dependencyFault = func(at string) error {
		if at == "activate" {
			return errors.New("forced")
		}
		return nil
	}
	if err := installTool(op, paths, manifest); err == nil {
		t.Fatal("activation failure ignored")
	}
	if readTestFile(t, target+"/leftover") != "leftover" || readTestFile(t, paths.State+"/dependencies/tool") != "0.9\n" {
		t.Fatal("prior directory or version lost")
	}
	if strings.Contains(output(op), "approved dependency") {
		t.Fatal("success reported")
	}
}

func TestDirectDownloadFreshFailureLeavesNoSuccessOrTarget(t *testing.T) {
	for _, phase := range []string{"stage", "activate", "state"} {
		t.Run(phase, func(t *testing.T) {
			op, paths, manifest, home := dependencyFixture(t)
			directDownload(t, manifest, home+"/source", "1.0", ".local/bin/tool", false)
			if phase == "stage" {
				// The stage failure occurs before curl is even looked up.
				p := op.Process
				p.Env = []string{"PATH=/no/such/path"}
				op.Process = p
			}
			op.dependencyFault = func(at string) error {
				if at == phase {
					return errors.New("forced " + at)
				}
				return nil
			}
			if err := installTool(op, paths, manifest); err == nil {
				t.Fatal("failure ignored")
			}
			assertNoPath(t, home+"/.local/bin/tool")
			assertNoPath(t, paths.State+"/dependencies/tool")
			if strings.Contains(output(op), "approved dependency") {
				t.Fatalf("success reported: %q", output(op))
			}
		})
	}
}

func TestDirectOptionalFailureAndCancellation(t *testing.T) {
	op, paths, manifest, home := dependencyFixture(t)
	directDownload(t, manifest, home+"/source", "1.0", ".local/bin/tool", true)
	if err := op.InstallDirect(context.Background(), paths, manifest, "optional", "tool", "linux", "amd64", false); err != nil {
		t.Fatal(err)
	}
	if len(op.SkippedOptional) != 1 || op.SkippedOptional[0] != "tool" {
		t.Fatalf("optional skip: %v", op.SkippedOptional)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	op.SkippedOptional = nil
	op.Process.Out = new(bytes.Buffer)
	if err := op.InstallDirect(ctx, paths, manifest, "optional", "tool", "linux", "amd64", false); !errors.Is(err, context.Canceled) || len(op.SkippedOptional) != 0 || output(op) != "" {
		t.Fatalf("cancelled optional: %v %v %q", err, op.SkippedOptional, output(op))
	}
	assertNoPath(t, home+"/.local/bin/tool")
}

func TestDirectDryRunReadOnly(t *testing.T) {
	op, paths, manifest, home := dependencyFixture(t)
	if err := op.InstallDirect(context.Background(), paths, manifest, "required", "tool", "linux", "amd64", true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output(op), "Would sync required direct package: tool") {
		t.Fatalf("dry run: %q", output(op))
	}
	assertNoPath(t, paths.State)
	assertNoPath(t, home+"/.local/bin/tool")
}

func TestDirectDataTargetPreservesRawXDGSpelling(t *testing.T) {
	op, _, manifest, home := dependencyFixture(t)
	writeTestFile(t, home+"/actual/.keep", "keep", 0600)
	if err := os.Symlink(home+"/actual", home+"/link"); err != nil {
		t.Fatal(err)
	}
	raw := home + "/link/../link"
	t.Setenv("XDG_DATA_HOME", raw)
	paths, err := UserPaths()
	if err != nil {
		t.Fatal(err)
	}
	directDownload(t, manifest, home+"/source", "1.0", ".local/share/tool/tool", false)
	if err := installTool(op, paths, manifest); err != nil {
		t.Fatal(err)
	}
	if readTestFile(t, raw+"/tool/tool") != "#!/bin/sh\necho 1.0\n" {
		t.Fatal("raw XDG data target not used")
	}
	assertNoPath(t, home+"/tool")
}

func TestDirectArchiveMemberAndUnsafeEntries(t *testing.T) {
	for _, kind := range []string{"valid", "traversal", "symlink", "missing"} {
		t.Run(kind, func(t *testing.T) {
			op, paths, manifest, home := dependencyFixture(t)
			var archive bytes.Buffer
			gz := gzip.NewWriter(&archive)
			tw := tar.NewWriter(gz)
			entry := "bin/tool"
			typ := byte(tar.TypeReg)
			if kind == "traversal" {
				entry = "../escape"
			}
			if kind == "symlink" {
				typ = tar.TypeSymlink
			}
			if kind != "missing" {
				hdr := &tar.Header{Name: entry, Mode: 0755, Size: int64(len("payload")), Typeflag: typ}
				if typ == tar.TypeSymlink {
					hdr.Size = 0
					hdr.Linkname = "/tmp/escape"
				}
				if err := tw.WriteHeader(hdr); err != nil {
					t.Fatal(err)
				}
				if typ == tar.TypeReg {
					if _, err := tw.Write([]byte("payload")); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := tw.Close(); err != nil {
				t.Fatal(err)
			}
			if err := gz.Close(); err != nil {
				t.Fatal(err)
			}
			source := home + "/archive.tar.gz"
			writeTestFile(t, source, archive.String(), 0600)
			sum := sha256.Sum256(archive.Bytes())
			writeTestFile(t, manifest, fmt.Sprintf("download tool 1.0 linux amd64 file://%s %x .local/bin/tool bin/tool\n", source, sum), 0600)
			err := installTool(op, paths, manifest)
			if kind == "valid" {
				if err != nil || readTestFile(t, home+"/.local/bin/tool") != "payload" {
					t.Fatalf("archive install: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("unsafe or missing archive member accepted")
			}
			assertNoPath(t, home+"/.local/bin/tool")
			assertNoPath(t, paths.State+"/dependencies/tool")
		})
	}
}

func gitCommand(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.invalid", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.invalid")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s: %v", args, out, err)
	}
	return strings.TrimSpace(string(out))
}
func directGitFixture(t *testing.T) (*PackageOperation, Paths, string, string, string, string) {
	t.Helper()
	op, paths, manifest, home := dependencyFixture(t)
	repo := home + "/repo"
	os.MkdirAll(repo, 0700)
	gitCommand(t, repo, "init", "--quiet")
	writeTestFile(t, repo+"/marker", "marker\n", 0600)
	gitCommand(t, repo, "add", "marker")
	gitCommand(t, repo, "commit", "--quiet", "-m", "initial")
	gitCommand(t, repo, "tag", "v1.0")
	head := gitCommand(t, repo, "rev-parse", "HEAD")
	writeTestFile(t, manifest, fmt.Sprintf("git testgit v1.0 linux amd64 %s %s .local/share/testgit marker\n", repo, head), 0600)
	return op, paths, manifest, home, repo, head
}
func installGit(op *PackageOperation, paths Paths, manifest string) error {
	return op.InstallDirect(context.Background(), paths, manifest, "required", "testgit", "linux", "amd64", false)
}

func TestDirectGitPinAndRepair(t *testing.T) {
	op, paths, manifest, home, repo, head := directGitFixture(t)
	target := home + "/data/testgit"
	if err := installGit(op, paths, manifest); err != nil {
		t.Fatal(err)
	}
	if gitCommand(t, target, "rev-parse", "HEAD") != head || readTestFile(t, paths.State+"/dependencies/testgit") != "v1.0\n" {
		t.Fatal("initial pin/state wrong")
	}
	op.Process.Out = new(bytes.Buffer)
	if err := installGit(op, paths, manifest); err != nil || output(op) != "" || op.UnchangedCount != 1 {
		t.Fatalf("noop: %v %q", err, output(op))
	}
	writeTestFile(t, target+"/cache", "untracked", 0600)
	if err := installGit(op, paths, manifest); err != nil || output(op) != "" {
		t.Fatalf("untracked file caused repair: %v %q", err, output(op))
	}
	writeTestFile(t, target+"/marker", "edited", 0600)
	if err := installGit(op, paths, manifest); err != nil || !strings.Contains(output(op), "Installed approved dependency") || readTestFile(t, target+"/marker") != "marker\n" {
		t.Fatalf("dirty repair: %v %q", err, output(op))
	}
	gitCommand(t, target, "commit", "--quiet", "--allow-empty", "-m", "drift")
	if err := installGit(op, paths, manifest); err != nil || gitCommand(t, target, "rev-parse", "HEAD") != head {
		t.Fatalf("commit drift not repaired: %v", err)
	}
	gitCommand(t, repo, "commit", "--quiet", "--allow-empty", "-m", "moved")
	gitCommand(t, repo, "tag", "-f", "v1.0")
	if err := os.RemoveAll(target); err != nil {
		t.Fatal(err)
	}
	os.Remove(paths.State + "/dependencies/testgit")
	if err := installGit(op, paths, manifest); err == nil || !strings.Contains(err.Error(), "no longer points to its approved commit") {
		t.Fatalf("moved tag accepted: %v", err)
	}
	assertNoPath(t, target)
}

func TestShippedZinitDependencyPinsCommit(t *testing.T) {
	deps, err := ReadDependencies("../../dependencies.conf")
	if err != nil {
		t.Fatal(err)
	}
	for _, dep := range deps {
		if dep.Kind == "git" && dep.Name == "zinit" {
			if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(dep.Checksum) {
				t.Fatalf("zinit commit pin: %q", dep.Checksum)
			}
			return
		}
	}
	t.Fatal("shipped zinit dependency missing")
}

func TestDirectGitBrokenTargetsAndCheckoutFailure(t *testing.T) {
	for _, shape := range []string{"missing_marker", "missing_git", "symlink", "checkout_failure", "stale_previous"} {
		t.Run(shape, func(t *testing.T) {
			op, paths, manifest, home, repo, head := directGitFixture(t)
			target := home + "/data/testgit"
			state := paths.State + "/dependencies/testgit"
			writeTestFile(t, state, "v1.0\n", 0600)
			switch shape {
			case "missing_marker":
				os.MkdirAll(target+"/.git", 0700)
			case "missing_git":
				writeTestFile(t, target+"/marker", "marker", 0600)
			case "symlink":
				writeTestFile(t, home+"/elsewhere/marker", "user marker", 0600)
				os.MkdirAll(filepath.Dir(target), 0700)
				os.Symlink(home+"/elsewhere", target)
			case "checkout_failure":
				if err := installGit(op, paths, manifest); err != nil {
					t.Fatal(err)
				}
				writeTestFile(t, manifest, fmt.Sprintf("git testgit missing linux amd64 %s %s .local/share/testgit marker\n", repo, head), 0600)
				op.dependencies = nil
			case "stale_previous":
				writeTestFile(t, target+".previous.stale/marker", "stale", 0600)
			}
			err := installGit(op, paths, manifest)
			if shape == "checkout_failure" {
				if err == nil || readTestFile(t, target+"/marker") != "marker\n" || readTestFile(t, state) != "v1.0\n" {
					t.Fatalf("checkout failure damaged prior state: %v", err)
				}
				return
			}
			if err != nil || gitCommand(t, target, "rev-parse", "HEAD") != head {
				t.Fatalf("repair failed: %v", err)
			}
			if shape == "symlink" && readTestFile(t, home+"/elsewhere/marker") != "user marker" {
				t.Fatal("symlink destination changed")
			}
			if shape == "stale_previous" && readTestFile(t, target+".previous.stale/marker") != "stale" {
				t.Fatal("stale previous path changed")
			}
		})
	}
}
