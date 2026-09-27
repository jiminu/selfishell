package integration_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jiminu/selfishell/internal/selfishell"
)

func maintenanceMiseEnv(home string) []string {
	return []string{
		"MISE_DATA_DIR=" + filepath.Join(home, "mise-data"),
		"MISE_CACHE_DIR=" + filepath.Join(home, "mise-cache"),
		"MISE_CONFIG_DIR=" + filepath.Join(home, "mise-config"),
		"MISE_STATE_DIR=" + filepath.Join(home, "mise-state"),
	}
}

func publishedFixture(t *testing.T, version string) (string, []string) {
	t.Helper()
	home := t.TempDir()
	assets := nativeVersionAssets(t, version)
	assertAssetSet(t, assets, version)
	releaseRoot := filepath.Join(home, "releases")
	rawRoot := filepath.Join(home, "raw")
	for _, dir := range []string{filepath.Join(releaseRoot, "download", "v"+version), filepath.Join(releaseRoot, "latest", "download"), filepath.Join(rawRoot, "v"+version), filepath.Join(home, "bin"), filepath.Join(home, "tmp")} {
		mustFS(t, os.MkdirAll(dir, 0700))
	}
	for _, name := range append(releaseAssetNames(version), "SHA256SUMS", "VERSION") {
		mustFS(t, copyFile(filepath.Join(assets, name), filepath.Join(releaseRoot, "download", "v"+version, name)))
	}
	mustFS(t, copyFile(filepath.Join(repoRoot(), "install.sh"), filepath.Join(rawRoot, "v"+version, "install.sh")))
	mustFS(t, os.WriteFile(filepath.Join(releaseRoot, "latest", "download", "VERSION"), []byte(version+"\n"), 0600))
	gh := `#!/bin/bash
set -euo pipefail
if [[ "$1 $2" == "release view" && "$*" == *'--json assets'* ]]; then
  find "$TEST_ASSETS" -maxdepth 1 -type f -exec basename {} \; | LC_ALL=C sort
  [[ "${TEST_EXTRA_ASSET:-0}" != 1 ]] || printf 'unexpected.txt\n'
elif [[ "$1 $2" == "release view" ]]; then
  printf '%s\t%s\thttps://example.invalid/releases/tag/v%s\n' "${TEST_TAG:-v$TEST_VERSION}" "$TEST_PRERELEASE" "$TEST_VERSION"
elif [[ "$1 $2" == "release download" ]]; then
  while (($#)); do
    if [[ "$1" == --dir ]]; then
      shift
      cp "$TEST_ASSETS"/* "$1/"
      [[ "${TEST_BAD_CHECKSUM:-0}" != 1 ]] || printf 'corrupt\n' >>"$1/selfishell-$TEST_VERSION-linux-amd64.tar.gz"
      exit 0
    fi
    shift
  done
  exit 2
elif [[ "$1 $2" == "attestation verify" ]]; then
  [[ "${TEST_NO_ATTESTATION:-0}" != 1 ]] || exit 2
  [[ "${3:-}" != --help ]] || exit 0
  [[ "${TEST_BAD_ATTESTATION:-0}" != 1 ]] || exit 1
else
  exit 2
fi
`
	mustFS(t, os.WriteFile(filepath.Join(home, "bin", "gh"), []byte(gh), 0700))
	classification := "false"
	if strings.Contains(version, "-") {
		classification = "true"
	}
	env := []string{
		"PATH=" + filepath.Join(home, "bin") + ":" + filepath.Join(runtime.GOROOT(), "bin") + ":/usr/bin:/bin:/usr/sbin:/sbin",
		"GOTOOLCHAIN=local", "GOCACHE=" + filepath.Join(home, "go-cache"),
		"MISE_DATA_DIR=" + filepath.Join(home, "mise-data"), "MISE_CACHE_DIR=" + filepath.Join(home, "mise-cache"), "MISE_CONFIG_DIR=" + filepath.Join(home, "mise-config"), "MISE_STATE_DIR=" + filepath.Join(home, "mise-state"),
		"TEST_ASSETS=" + assets, "TEST_VERSION=" + version, "TEST_PRERELEASE=" + classification,
		"SELFISHELL_VERIFY_RAW_ROOT=file://" + rawRoot, "SELFISHELL_VERIFY_RELEASE_ROOT=file://" + releaseRoot,
	}
	return home, env
}

func TestPublishedReleaseVerification(t *testing.T) {
	version := nativeArchiveVersion
	home, env := publishedFixture(t, version)
	script := filepath.Join(repoRoot(), "scripts", "verify-published-release.sh")
	for _, tc := range []struct {
		name   string
		extra  []string
		status int
		output string
		stderr string
	}{
		{"attested", nil, 0, "Artifact attestations verified.\n", ""},
		{"unavailable", []string{"TEST_NO_ATTESTATION=1"}, 1, "", "SELFISHELL_VERIFY_SKIP_ATTESTATION"},
		{"optout", []string{"TEST_NO_ATTESTATION=1", "SELFISHELL_VERIFY_SKIP_ATTESTATION=1"}, 0, "skipped (SELFISHELL_VERIFY_SKIP_ATTESTATION=1)", ""},
		{"bad-attestation", []string{"TEST_BAD_ATTESTATION=1", "SELFISHELL_VERIFY_SKIP_ATTESTATION=1"}, 1, "", ""},
		{"wrong-tag", []string{"TEST_TAG=v0.0.0"}, 1, "", "Published tag mismatch"},
		{"wrong-classification", []string{"TEST_PRERELEASE=true"}, 1, "", "Release classification mismatch"},
		{"wrong-assets", []string{"TEST_EXTRA_ASSET=1"}, 1, "", "Published asset set mismatch"},
		{"bad-checksum", []string{"TEST_BAD_CHECKSUM=1"}, 1, "FAILED", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := runCommand(home, []string{"/bin/bash", script, version}, nil, append(env, tc.extra...), 90*time.Second)
			mustFS(t, err)
			if got.Status != tc.status || !strings.Contains(string(got.Stdout), tc.output) || !strings.Contains(string(got.Stderr), tc.stderr) {
				t.Fatalf("status=%d stdout=%q stderr=%q", got.Status, got.Stdout, got.Stderr)
			}
			if tc.status == 0 && !strings.Contains(string(got.Stdout), "Published release "+version+" verified:") {
				t.Fatalf("success missing: %s", got.Stdout)
			}
		})
	}
}

func TestPublishedReleaseInvalidInputBeforeEffects(t *testing.T) {
	home := t.TempDir()
	for _, args := range [][]string{{"invalid"}, {"1.2.3", "invalid-repository"}} {
		got, err := runCommand(home, append([]string{"/bin/bash", filepath.Join(repoRoot(), "scripts", "verify-published-release.sh")}, args...), nil, append(maintenanceMiseEnv(home), "PATH=/usr/bin:/bin", "TMPDIR="+home), 5*time.Second)
		mustFS(t, err)
		if got.Status != 2 {
			t.Fatalf("%v status=%d stderr=%q", args, got.Status, got.Stderr)
		}
		entries, err := os.ReadDir(home)
		mustFS(t, err)
		if len(entries) != 0 {
			t.Fatalf("invalid input caused effects: %v", entries)
		}
	}
}

func TestNextPatchVersionContract(t *testing.T) {
	home := t.TempDir()
	script := filepath.Join(repoRoot(), "scripts", "next-patch-version.sh")
	for _, tc := range []struct {
		version, output string
		status          int
	}{{"1.2.3", "1.2.4\n", 0}, {"01.2.3", "", 1}} {
		got, err := runCommand(home, []string{"/bin/bash", script, "--current", tc.version}, nil, maintenanceMiseEnv(home), 5*time.Second)
		mustFS(t, err)
		if got.Status != tc.status || string(got.Stdout) != tc.output {
			t.Fatalf("%s: status=%d stdout=%q stderr=%q", tc.version, got.Status, got.Stdout, got.Stderr)
		}
	}
}

func TestMaintenanceScriptsNoRuntimeCommonImport(t *testing.T) {
	for _, name := range []string{"verify-published-release.sh", "next-patch-version.sh", "build-release.sh"} {
		data, err := os.ReadFile(filepath.Join(repoRoot(), "scripts", name))
		mustFS(t, err)
		if strings.Contains(string(data), "lib/common.sh") {
			t.Errorf("%s imports common.sh", name)
		}
	}
}

func TestReleaseVersionValidatorParity(t *testing.T) {
	script := filepath.Join(repoRoot(), "scripts", "release-version.sh")
	for _, version := range []string{"0.0.0", "1.2.3", "1.2.3-alpha", "1.2.3-alpha.1", "1.2.3-0.3.7", "1.2.3-x.7.z-92", "1.2.3-01alpha", "v1.2.3", "01.2.3", "1.02.3", "1.2.03", "1.2", "1.2.3-", "1.2.3-alpha..1", "1.2.3-alpha_1", "1.2.3-01", "1.2.3-alpha.01", "1.2.3+build"} {
		home := t.TempDir()
		got, err := runCommand(home, []string{"/bin/bash", "-c", "source \"$1\"; selfishell_version_is_valid \"$2\"", "bash", script, version}, nil, maintenanceMiseEnv(home), 5*time.Second)
		mustFS(t, err)
		if (got.Status == 0) != selfishell.ValidReleaseVersion(version) {
			t.Errorf("validator disagreement for %q: shell status %d", version, got.Status)
		}
	}
}

func TestNextPatchUsesLocalStableTags(t *testing.T) {
	home := t.TempDir()
	env := maintenanceMiseEnv(home)
	for _, args := range [][]string{{"init", "-q"}, {"-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-qm", "test"}, {"tag", "v1.2.3"}, {"tag", "v1.2.4-beta.1"}, {"tag", "v1.2.4"}} {
		got, err := runCommandIn(home, home, append([]string{"git"}, args...), nil, env, 5*time.Second)
		mustFS(t, err)
		if got.Status != 0 {
			t.Fatalf("git %v: %d %s", args, got.Status, got.Stderr)
		}
	}
	got, err := runCommandIn(home, home, []string{"/bin/bash", filepath.Join(repoRoot(), "scripts", "next-patch-version.sh")}, nil, env, 5*time.Second)
	mustFS(t, err)
	if got.Status != 0 || string(got.Stdout) != "1.2.5\n" {
		t.Fatalf("local tag patch: %d %q %q", got.Status, got.Stdout, got.Stderr)
	}
}

func TestCurrentProductionBuilderRejectsInvalidVersions(t *testing.T) {
	home := t.TempDir()
	script := filepath.Join(repoRoot(), "scripts", "build-release.sh")
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"invalid", []string{"--version", "1.2.3-alpha..1"}},
		{"missing", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := filepath.Join(home, tc.name)
			args := append(append([]string{"/bin/bash", script}, tc.args...), "--output", output)
			got, err := runCommand(home, args, nil, maintenanceMiseEnv(home), 5*time.Second)
			mustFS(t, err)
			if got.Status != 2 {
				t.Fatalf("status=%d stderr=%q", got.Status, got.Stderr)
			}
			if _, err := os.Lstat(output); !os.IsNotExist(err) {
				t.Fatalf("builder created output: %v", err)
			}
		})
	}
}

func TestPublishedPrereleaseLatestPolicy(t *testing.T) {
	version := prereleaseNativeVersion
	home, env := publishedFixture(t, version)
	script := filepath.Join(repoRoot(), "scripts", "verify-published-release.sh")
	failed, err := runCommand(home, []string{"/bin/bash", script, version}, nil, env, 90*time.Second)
	mustFS(t, err)
	if failed.Status != 1 || !strings.Contains(string(failed.Stderr), "Pre-release unexpectedly replaced") {
		t.Fatalf("prerelease latest accepted: %d %q", failed.Status, failed.Stderr)
	}
	latest := filepath.Join(home, "releases", "latest", "download", "VERSION")
	mustFS(t, os.WriteFile(latest, []byte("1.3.2\n"), 0600))
	passed, err := runCommand(home, []string{"/bin/bash", script, version}, nil, env, 90*time.Second)
	mustFS(t, err)
	if passed.Status != 0 || !strings.Contains(string(passed.Stdout), "Published release "+version+" verified:") {
		t.Fatalf("prerelease stable unchanged: %d %q %q", passed.Status, passed.Stdout, passed.Stderr)
	}
}

func TestPublishedStableLatestMustMatch(t *testing.T) {
	home, env := publishedFixture(t, nativeArchiveVersion)
	latest := filepath.Join(home, "releases", "latest", "download", "VERSION")
	mustFS(t, os.WriteFile(latest, []byte("1.3.1\n"), 0600))
	got, err := runCommand(home, []string{"/bin/bash", filepath.Join(repoRoot(), "scripts", "verify-published-release.sh"), nativeArchiveVersion}, nil, env, 90*time.Second)
	mustFS(t, err)
	if got.Status != 1 || !strings.Contains(string(got.Stderr), "Latest stable version mismatch") {
		t.Fatalf("stable latest accepted: %d %q", got.Status, got.Stderr)
	}
}
