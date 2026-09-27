package selfishell

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func isolatedUpdateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for name, value := range map[string]string{
		"HOME":            home,
		"XDG_CONFIG_HOME": home + "/.config",
		"XDG_DATA_HOME":   home + "/.local/share",
		"XDG_STATE_HOME":  home + "/.local/state",
		"XDG_CACHE_HOME":  home + "/.cache",
		"MISE_DATA_DIR":   home + "/mise/data",
		"MISE_CACHE_DIR":  home + "/mise/cache",
		"MISE_CONFIG_DIR": home + "/mise/config",
		"MISE_STATE_DIR":  home + "/mise/state",
	} {
		t.Setenv(name, value)
	}
	return home
}

func isolateMiseForHome(t *testing.T, home string) {
	t.Helper()
	for name, path := range map[string]string{
		"XDG_CONFIG_HOME": "/.config", "XDG_DATA_HOME": "/.local/share",
		"XDG_STATE_HOME": "/.local/state", "XDG_CACHE_HOME": "/.cache",
		"MISE_DATA_DIR": "/mise/data", "MISE_CACHE_DIR": "/mise/cache",
		"MISE_CONFIG_DIR": "/mise/config", "MISE_STATE_DIR": "/mise/state",
	} {
		t.Setenv(name, home+path)
	}
}

func commandResult(root string, args ...string) (int, string, string) {
	var out, err bytes.Buffer
	c := CLI{Root: root, In: strings.NewReader(""), Out: &out, Err: &err}
	code := c.Run(args)
	return code, out.String(), err.String()
}

type rollbackMutationReader struct {
	mutate func()
	answer *strings.Reader
}

func (r *rollbackMutationReader) Read(p []byte) (int, error) {
	if r.mutate != nil {
		mutate := r.mutate
		r.mutate = nil
		mutate()
	}
	return r.answer.Read(p)
}

func retainedRollbackRelease(t *testing.T, releases string) {
	t.Helper()
	target := releases + "/2.0.0"
	if err := os.MkdirAll(target+"/bin", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target+"/VERSION", []byte("2.0.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target+"/bin/selfishell", []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateAndRollbackArguments(t *testing.T) {
	root := isolatedUpdateHome(t)
	for _, tc := range []struct {
		args []string
		code int
		text string
	}{
		{[]string{"update", "--help"}, 0, "left at their current version"},
		{[]string{"update", "--cli-only", "--tools-only"}, 2, "cannot be used together"},
		{[]string{"update", "--cli-only", "--continue-after-cli-update"}, 2, "cannot be used together"},
		{[]string{"update", "--continue-after-cli-update", "--cli-only"}, 2, "cannot be used together"},
		{[]string{"update", "--tools-only", "--version", "1.2.3"}, 2, "--version cannot be used"},
		{[]string{"update", "--version"}, 2, "requires a value"},
		{[]string{"update", "--version", "../escape"}, 2, "Invalid semantic version"},
		{[]string{"update", "--bogus"}, 2, "Unknown update option"},
		{[]string{"rollback", "--help"}, 0, "Usage:"},
		{[]string{"rollback", "1.0.0", "2.0.0"}, 2, "only one version"},
		{[]string{"rollback", "../escape"}, 2, "Invalid semantic version"},
		{[]string{"rollback", ""}, 2, "Invalid semantic version"},
		{[]string{"rollback", "v"}, 2, "Invalid semantic version"},
		{[]string{"rollback", "--bogus"}, 2, "Unknown rollback option"},
	} {
		code, out, stderr := commandResult(root, tc.args...)
		if code != tc.code || !strings.Contains(out+stderr, tc.text) {
			t.Errorf("%v: code %d, out %q, err %q", tc.args, code, out, stderr)
		}
	}
}

func TestUpdateContinuationRejectsUnreadableConfiguredMarker(t *testing.T) {
	home := isolatedUpdateHome(t)
	root := testRelease(t)
	path := home + "/.local/state/selfishell/configured"
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	code, out, stderr := commandResult(root, "update", "--continue-after-cli-update", "--skip-packages", "--dry-run", "--yes")
	if code != 1 || strings.Contains(out, "skipping tools") || strings.Contains(out, "Selfishell updated") || !strings.Contains(stderr, "configured marker") {
		t.Fatalf("directory marker: %d %q %q", code, out, stderr)
	}
}

func TestUpdateAcceptsReadableNoncanonicalConfiguredMarker(t *testing.T) {
	for _, content := range []string{"", "invalid-marker\n"} {
		t.Run("bytes-"+strconv.Itoa(len(content)), func(t *testing.T) {
			home := isolatedUpdateHome(t)
			root := testRelease(t)
			path := home + "/.local/state/selfishell/configured"
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			code, out, stderr := commandResult(root, "update", "--tools-only", "--skip-packages", "--dry-run", "--yes")
			if code != 0 || strings.Contains(out, "configuration is not installed") || !strings.Contains(out, "Tool/configuration dry run complete") {
				t.Fatalf("readable marker: %d %q %q", code, out, stderr)
			}
		})
	}
}

func TestUpdateDryRunAcceptsValidPrereleaseFromSourceCheckout(t *testing.T) {
	isolatedUpdateHome(t)
	root := testRelease(t)
	code, out, stderr := commandResult(root, "update", "--cli-only", "--version", "v1.2.3-alpha.1.x-7", "--dry-run")
	if code != 0 || !strings.Contains(out, "Would update Selfishell CLI to 1.2.3-alpha.1.x-7") {
		t.Fatalf("dry source: %d %q %q", code, out, stderr)
	}
}

func TestUpdateRequiresInstalledReleaseForMutation(t *testing.T) {
	isolatedUpdateHome(t)
	root := testRelease(t)
	code, out, stderr := commandResult(root, "update", "--cli-only", "--version", "2.0.0", "--yes")
	if code != 1 || strings.Contains(out, "Selfishell updated") || !strings.Contains(stderr, "versioned Selfishell installation") {
		t.Fatalf("source checkout update: %d %q %q", code, out, stderr)
	}
}

func TestRollbackOfflineKeepsConfigurationAndHistoryOnNoop(t *testing.T) {
	home := isolatedUpdateHome(t)
	share := filepath.Join(home, "share/selfishell")
	root := filepath.Join(share, "releases/1.0.0")
	for _, version := range []string{"1.0.0", "2.0.0"} {
		dir := filepath.Join(share, "releases", version)
		if err := os.MkdirAll(dir+"/bin", 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dir+"/VERSION", []byte(version+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dir+"/bin/selfishell", []byte("#!/bin/sh\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("releases/1.0.0", share+"/current"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("releases/2.0.0", share+"/previous"); err != nil {
		t.Fatal(err)
	}
	state := home + "/.local/state/selfishell/configured"
	if err := os.MkdirAll(filepath.Dir(state), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(state, []byte("1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	code, out, stderr := commandResult(root, "rollback", "v1.0.0", "--yes")
	if code != 0 || !strings.Contains(out, "already active") {
		t.Fatalf("noop: %d %q %q", code, out, stderr)
	}
	link, _ := os.Readlink(share + "/previous")
	if link != "releases/2.0.0" {
		t.Fatalf("noop rewrote previous: %s", link)
	}
	code, out, stderr = commandResult(root, "rollback", "--yes")
	if code != 0 || !strings.Contains(out, "rolled back to 2.0.0") {
		t.Fatalf("rollback: %d %q %q", code, out, stderr)
	}
	current, _ := os.Readlink(share + "/current")
	previous, _ := os.Readlink(share + "/previous")
	if current != "releases/2.0.0" || previous != "releases/1.0.0" {
		t.Fatalf("links: %s %s", current, previous)
	}
	code, out, stderr = commandResult(root, "rollback", "--yes")
	if code != 0 || !strings.Contains(out, "rolled back to 1.0.0") {
		t.Fatalf("return rollback: %d %q %q", code, out, stderr)
	}
	current, _ = os.Readlink(share + "/current")
	previous, _ = os.Readlink(share + "/previous")
	if current != "releases/1.0.0" || previous != "releases/2.0.0" {
		t.Fatalf("return links: %s %s", current, previous)
	}
	data, _ := os.ReadFile(state)
	if string(data) != "1\n" {
		t.Fatalf("configuration changed: %q", data)
	}
}

func TestRollbackToActiveAbsoluteLinkPreservesLinkIdentity(t *testing.T) {
	op, share, _ := releaseFixture(t)
	isolateMiseForHome(t, os.Getenv("HOME"))
	if err := os.Remove(share + "/current"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(op.Root, share+"/current"); err != nil {
		t.Fatal(err)
	}
	code, out, stderr := commandResult(op.Root, "rollback", "1.0.0", "--yes")
	if code != 0 || !strings.Contains(out, "already active") {
		t.Fatalf("same directory: %d %q %q", code, out, stderr)
	}
	current, _ := os.Readlink(share + "/current")
	if current != op.Root {
		t.Fatalf("no-op rewrote link identity: %s", current)
	}
	if _, err := os.Lstat(share + "/previous"); !os.IsNotExist(err) {
		t.Fatalf("no-op created history: %v", err)
	}
}

func TestUpdateSelectionAndDryRunDoNotMutateRelease(t *testing.T) {
	op, share, _ := releaseFixture(t)
	isolateMiseForHome(t, os.Getenv("HOME"))
	root := op.Root
	remote := publishReleaseFixture(t, "2.0.0", archiveMember{"VERSION", "", 0, "2.0.0\n", 0644}, archiveMember{"bin/selfishell", "", 0, "#!/bin/sh\n", 0755})
	if err := os.MkdirAll(filepath.Dir(filepath.Dir(remote))+"/latest/download", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Dir(filepath.Dir(remote))+"/latest/download/VERSION", []byte("2.0.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	code, out, stderr := commandResult(root, "update", "--cli-only", "--dry-run")
	if code != 0 || !strings.Contains(out, "Would update Selfishell CLI to 2.0.0") {
		t.Fatalf("dry: %d %q %q", code, out, stderr)
	}
	link, _ := os.Readlink(share + "/current")
	if link != "releases/1.0.0" {
		t.Fatalf("dry mutated current: %s", link)
	}
	code, out, stderr = commandResult(root, "update", "--cli-only", "--yes")
	if code != 0 || strings.Count(out, "Selfishell updated:") != 1 {
		t.Fatalf("updated: %d %q %q", code, out, stderr)
	}
	link, _ = os.Readlink(share + "/current")
	if link != "releases/2.0.0" {
		t.Fatalf("wrong current: %s", link)
	}
	code, out, stderr = commandResult(share+"/releases/2.0.0", "update", "--version", "v2.0.0", "--yes")
	if code != 0 || !strings.Contains(out, "up to date") {
		t.Fatalf("noop: %d %q %q", code, out, stderr)
	}
}

func TestUpdateExplicitDowngradeAndOlderLatestNoop(t *testing.T) {
	op, share, _ := releaseFixture(t)
	isolateMiseForHome(t, os.Getenv("HOME"))
	dir := publishReleaseFixture(t, "2.0.0", archiveMember{"VERSION", "", 0, "2.0.0\n", 0644}, archiveMember{"bin/selfishell", "", 0, "#!/bin/sh\n", 0755})
	remote := filepath.Dir(filepath.Dir(dir))
	older := remote + "/download/v1.0.0"
	if err := os.MkdirAll(older, 0700); err != nil {
		t.Fatal(err)
	}
	name, _ := releaseArchiveName("1.0.0")
	archive := testArchive(t, archiveMember{"VERSION", "", 0, "1.0.0\n", 0644}, archiveMember{"bin/selfishell", "", 0, "#!/bin/sh\n", 0755})
	data, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(older+"/"+name, data, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if err := os.WriteFile(older+"/SHA256SUMS", []byte(hex.EncodeToString(sum[:])+"  "+name+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	code, out, stderr := commandResult(op.Root, "update", "--version", "2.0.0", "--cli-only", "--yes")
	if code != 0 {
		t.Fatalf("upgrade: %d %q %q", code, out, stderr)
	}
	latest := remote + "/latest/download"
	if err := os.MkdirAll(latest, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(latest+"/VERSION", []byte("1.0.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	active := share + "/releases/2.0.0"
	code, out, stderr = commandResult(active, "update", "--yes")
	if code != 0 || !strings.Contains(out, "newer than the latest release") {
		t.Fatalf("older latest: %d %q %q", code, out, stderr)
	}
	current, _ := os.Readlink(share + "/current")
	if current != "releases/2.0.0" {
		t.Fatalf("latest downgraded: %s", current)
	}
	code, out, stderr = commandResult(active, "update", "--version", "v1.0.0", "--cli-only", "--yes")
	if code != 0 || !strings.Contains(out, "2.0.0 -> 1.0.0") {
		t.Fatalf("explicit downgrade: %d %q %q", code, out, stderr)
	}
	current, _ = os.Readlink(share + "/current")
	if current != "releases/1.0.0" {
		t.Fatalf("downgrade current: %s", current)
	}
}

func TestUpdateMissingExactAssetPreservesCurrentAndReportsFailure(t *testing.T) {
	op, share, _ := releaseFixture(t)
	isolateMiseForHome(t, os.Getenv("HOME"))
	remote := t.TempDir()
	t.Setenv("SELFISHELL_RELEASE_ROOT", "file://"+remote)
	code, out, stderr := commandResult(op.Root, "update", "--version", "2.0.0", "--cli-only", "--yes")
	if code != 1 || strings.Contains(out, "Selfishell updated") || stderr == "" {
		t.Fatalf("missing asset: %d %q %q", code, out, stderr)
	}
	current, _ := os.Readlink(share + "/current")
	if current != "releases/1.0.0" {
		t.Fatalf("failure rewrote current: %s", current)
	}
}

func TestUpdateFallsBackToPublishedPrereleaseOnly(t *testing.T) {
	op, share, _ := releaseFixture(t)
	isolateMiseForHome(t, os.Getenv("HOME"))
	dir := publishReleaseFixture(t, "1.1.0-beta.2", archiveMember{"VERSION", "", 0, "1.1.0-beta.2\n", 0644}, archiveMember{"bin/selfishell", "", 0, "#!/bin/sh\n", 0755})
	remote := filepath.Dir(filepath.Dir(dir))
	tags := remote + "/tags.json"
	if err := os.WriteFile(tags, []byte(`[{"name":"v1.1.0-beta.2"}]`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/VERSION", []byte("1.1.0-beta.2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SELFISHELL_RELEASE_TAGS_API_URL", "file://"+tags)
	code, out, stderr := commandResult(op.Root, "update", "--cli-only", "--yes")
	if code != 0 || !strings.Contains(out, "1.0.0 -> 1.1.0-beta.2") {
		t.Fatalf("fallback: %d %q %q", code, out, stderr)
	}
	current, _ := os.Readlink(share + "/current")
	if current != "releases/1.1.0-beta.2" {
		t.Fatalf("fallback selected wrong release: %s", current)
	}
}

func TestRollbackRejectsWrongVersionAndOccupiedCurrent(t *testing.T) {
	op, share, releases := releaseFixture(t)
	isolateMiseForHome(t, os.Getenv("HOME"))
	target := releases + "/2.0.0"
	if err := os.MkdirAll(target+"/bin", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target+"/bin/selfishell", []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target+"/VERSION", []byte("3.0.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	code, out, stderr := commandResult(op.Root, "rollback", "v2.0.0", "--yes")
	if code != 1 || out != "" || !strings.Contains(stderr, "Retained release not found") {
		t.Fatalf("mismatched release: %d %q %q", code, out, stderr)
	}
	if err := os.WriteFile(target+"/VERSION", []byte("2.0.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(share + "/current"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(share+"/current", []byte("foreign"), 0600); err != nil {
		t.Fatal(err)
	}
	code, out, stderr = commandResult(op.Root, "rollback", "v2.0.0", "--yes")
	if code != 1 || strings.Contains(out, "rolled back") {
		t.Fatalf("occupied current: %d %q %q", code, out, stderr)
	}
	data, _ := os.ReadFile(share + "/current")
	if string(data) != "foreign" {
		t.Fatalf("occupied current overwritten: %q", data)
	}
}

func TestRollbackExplicitVersionPreservesOccupiedPrevious(t *testing.T) {
	for _, kind := range []string{"file", "foreign link"} {
		t.Run(kind, func(t *testing.T) {
			op, share, releases := releaseFixture(t)
			isolateMiseForHome(t, os.Getenv("HOME"))
			retainedRollbackRelease(t, releases)
			previous := share + "/previous"
			if kind == "file" {
				if err := os.WriteFile(previous, []byte("user data\n"), 0600); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Symlink("/personal", previous); err != nil {
				t.Fatal(err)
			}
			code, out, stderr := commandResult(op.Root, "rollback", "2.0.0", "--yes")
			if code != 1 || strings.Contains(out, "rolled back") || stderr == "" {
				t.Fatalf("occupied previous: %d %q %q", code, out, stderr)
			}
			if current, err := os.Readlink(share + "/current"); err != nil || current != "releases/1.0.0" {
				t.Fatalf("current changed: %q, %v", current, err)
			}
			if kind == "file" {
				if data, err := os.ReadFile(previous); err != nil || string(data) != "user data\n" {
					t.Fatalf("previous changed: %q, %v", data, err)
				}
			} else if link, err := os.Readlink(previous); err != nil || link != "/personal" {
				t.Fatalf("previous changed: %q, %v", link, err)
			}
		})
	}
}

func TestRollbackRestoresPreviousWhenCurrentActivationFails(t *testing.T) {
	for _, retained := range []bool{false, true} {
		name := "no previous"
		if retained {
			name = "existing previous"
		}
		t.Run(name, func(t *testing.T) {
			op, share, releases := releaseFixture(t)
			isolateMiseForHome(t, os.Getenv("HOME"))
			retainedRollbackRelease(t, releases)
			if retained {
				if err := os.Symlink("releases/2.0.0", share+"/previous"); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("SELFISHELL_TEST_TTY", "1")
			reader := &rollbackMutationReader{answer: strings.NewReader("y\n"), mutate: func() {
				if err := os.Remove(share + "/current"); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(share+"/current", []byte("user data\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}}
			var out, stderr bytes.Buffer
			code := (CLI{Root: op.Root, In: reader, Out: &out, Err: &stderr}).Run([]string{"rollback", "2.0.0"})
			if code != 1 || strings.Contains(out.String(), "rolled back") || stderr.Len() == 0 {
				t.Fatalf("failed activation: %d %q %q", code, out.String(), stderr.String())
			}
			if data, err := os.ReadFile(share + "/current"); err != nil || string(data) != "user data\n" {
				t.Fatalf("current user file changed: %q, %v", data, err)
			}
			previous, err := os.Readlink(share + "/previous")
			if retained {
				if err != nil || previous != "releases/2.0.0" {
					t.Fatalf("previous not restored: %q, %v", previous, err)
				}
			} else if !os.IsNotExist(err) {
				t.Fatalf("new previous left after failure: %q, %v", previous, err)
			}
		})
	}
}

func TestRollbackLatePreviousConflictKeepsCurrent(t *testing.T) {
	op, share, releases := releaseFixture(t)
	isolateMiseForHome(t, os.Getenv("HOME"))
	retainedRollbackRelease(t, releases)
	t.Setenv("SELFISHELL_TEST_TTY", "1")
	reader := &rollbackMutationReader{answer: strings.NewReader("y\n"), mutate: func() {
		if err := os.WriteFile(share+"/previous", []byte("user data\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}}
	var out, stderr bytes.Buffer
	code := (CLI{Root: op.Root, In: reader, Out: &out, Err: &stderr}).Run([]string{"rollback", "2.0.0"})
	if code != 1 || strings.Contains(out.String(), "rolled back") || stderr.Len() == 0 {
		t.Fatalf("late previous conflict: %d %q %q", code, out.String(), stderr.String())
	}
	if current, err := os.Readlink(share + "/current"); err != nil || current != "releases/1.0.0" {
		t.Fatalf("current changed: %q, %v", current, err)
	}
	if data, err := os.ReadFile(share + "/previous"); err != nil || string(data) != "user data\n" {
		t.Fatalf("previous user file changed: %q, %v", data, err)
	}
}

func TestRollbackRejectsCorruptPreviousWithoutMutation(t *testing.T) {
	op, share, _ := releaseFixture(t)
	isolateMiseForHome(t, os.Getenv("HOME"))
	for _, bad := range []string{"../elsewhere", "releases/../1.0.0", "releases/9.9.9", "/tmp/elsewhere"} {
		if err := os.Remove(share + "/previous"); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if err := os.Symlink(bad, share+"/previous"); err != nil {
			t.Fatal(err)
		}
		code, out, stderr := commandResult(op.Root, "rollback", "--yes")
		if code != 1 || out != "" || !strings.Contains(stderr, "invalid") {
			t.Fatalf("%s: %d %q %q", bad, code, out, stderr)
		}
		current, _ := os.Readlink(share + "/current")
		if current != "releases/1.0.0" {
			t.Fatalf("%s rewrote current: %s", bad, current)
		}
	}
}

func TestToolsOnlySkipPackagesAppliesConfigurationWithoutInstallFinalization(t *testing.T) {
	home := isolatedUpdateHome(t)
	root := testRelease(t)
	t.Setenv("SELFISHELL_TEST_SYSTEM_NAME", "Darwin")
	t.Setenv("SHELL", "/bin/zsh")
	state := home + "/.local/state/selfishell"
	if err := os.MkdirAll(state, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(state+"/configured", []byte("1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	tools := t.TempDir()
	log := tools + "/calls"
	for _, name := range []string{"mise", "brew", "chsh", "curl"} {
		if err := os.WriteFile(tools+"/"+name, []byte("#!/bin/sh\nprintf '%s\\n' \"$0 $*\" >> \"$SELFISHELL_TEST_CALLS\"\nexit 91\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", tools+":/usr/bin:/bin")
	t.Setenv("SELFISHELL_TEST_CALLS", log)
	code, out, stderr := commandResult(root, "update", "--tools-only", "--skip-packages", "--yes")
	if code != 0 || !strings.Contains(out, "Selfishell tools and configuration synchronized") {
		t.Fatalf("config: %d %q %q", code, out, stderr)
	}
	if _, err := os.Stat(home + "/.config/selfishell/vim/vimrc"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(home + "/.config/mise/config.toml"); !os.IsNotExist(err) {
		t.Fatalf("created global mise config: %v", err)
	}
	if _, err := os.Lstat(state + "/ghostty"); !os.IsNotExist(err) {
		t.Fatalf("created Ghostty marker: %v", err)
	}
	if _, err := os.Lstat(home + "/.config/ghostty/config.ghostty"); !os.IsNotExist(err) {
		t.Fatalf("Ghostty selected without saved choice: %v", err)
	}
	data, err := os.ReadFile(log)
	if err != nil || !strings.Contains(string(data), "mise trust") || strings.Contains(string(data), "prune") {
		t.Fatalf("calls: %q %v", data, err)
	}
}

func TestToolsOnlyDryRunLeavesHomeAndMiseUntouched(t *testing.T) {
	home := isolatedUpdateHome(t)
	root := testRelease(t)
	t.Setenv("SELFISHELL_TEST_SYSTEM_NAME", "Linux")
	osRelease := home + "/os-release"
	if err := os.WriteFile(osRelease, []byte("ID=ubuntu\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SELFISHELL_TEST_OS_RELEASE_FILE", osRelease)
	state := home + "/.local/state/selfishell"
	if err := os.MkdirAll(state, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(state+"/configured", []byte("1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	code, out, stderr := commandResult(root, "update", "--tools-only", "--dry-run")
	if code != 0 || !strings.Contains(out, "Would install required apt packages") || !strings.Contains(out, "git") || !strings.Contains(out, "Would sync declared Neovim plugins") || !strings.Contains(out, "Would prune unused mise versions") {
		t.Fatalf("dry: %d %q %q", code, out, stderr)
	}
	after, err := os.ReadDir(home)
	if err != nil || len(after) != len(before) {
		t.Fatalf("dry top-level mutation: %v %v", after, err)
	}
	if _, err := os.Lstat(home + "/.config/selfishell"); !os.IsNotExist(err) {
		t.Fatalf("dry config mutation: %v", err)
	}
}

func TestToolsOnlyPreflightRejectsChangedManagedFileBeforePackageWork(t *testing.T) {
	home := isolatedUpdateHome(t)
	root := t.TempDir()
	copy := exec.Command("cp", "-R", testRelease(t)+"/config", root+"/config")
	if output, err := copy.CombinedOutput(); err != nil {
		t.Fatalf("copy fixture: %v %s", err, output)
	}
	for _, name := range []string{"packages.conf", "dependencies.conf"} {
		if err := os.Symlink(testRelease(t)+"/"+name, root+"/"+name); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("SELFISHELL_TEST_SYSTEM_NAME", "Darwin")
	t.Setenv("SHELL", "/bin/zsh")
	code, _, stderr := commandResult(root, "install", "--skip-packages", "--yes")
	if code != 0 {
		t.Fatalf("install: %s", stderr)
	}
	target := home + "/.config/selfishell/vim/vimrc"
	if err := os.WriteFile(target, []byte("personal data\n"), 0600); err != nil {
		t.Fatal(err)
	}
	state := home + "/.local/state/selfishell/resources/vimrc.state"
	before, err := os.ReadFile(state)
	if err != nil {
		t.Fatal(err)
	}
	earlier := home + "/.config/selfishell/zsh/runtime.zsh"
	earlierBefore, err := os.ReadFile(earlier)
	if err != nil {
		t.Fatal(err)
	}
	source := root + "/config/shared/zsh/runtime.zsh"
	f, err := os.OpenFile(source, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\n# new approved runtime\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	tools := t.TempDir()
	log := tools + "/calls"
	if err := os.WriteFile(tools+"/brew", []byte("#!/bin/sh\nprintf invoked >> \"$SELFISHELL_TEST_CALLS\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tools+":/usr/bin:/bin")
	t.Setenv("SELFISHELL_TEST_CALLS", log)
	code, out, stderr := commandResult(root, "update", "--tools-only", "--yes")
	if code != 1 || strings.Contains(out, "synchronized") || !strings.Contains(stderr, "Managed file was modified") {
		t.Fatalf("conflict: %d %q %q", code, out, stderr)
	}
	got, _ := os.ReadFile(target)
	stateAfter, _ := os.ReadFile(state)
	if string(got) != "personal data\n" || !bytes.Equal(before, stateAfter) {
		t.Fatalf("conflict changed user data/state")
	}
	earlierAfter, err := os.ReadFile(earlier)
	if err != nil || !bytes.Equal(earlierBefore, earlierAfter) {
		t.Fatalf("late conflict changed earlier file: %q %v", earlierAfter, err)
	}
	newSource, err := os.ReadFile(source)
	if err != nil || bytes.Equal(earlierBefore, newSource) {
		t.Fatalf("approved source did not change in fixture: %v", err)
	}
	if _, err := os.Lstat(home + "/.local/state/selfishell/backups"); !os.IsNotExist(err) {
		t.Fatalf("late conflict created backup: %v", err)
	}
	if _, err := os.Lstat(log); !os.IsNotExist(err) {
		t.Fatal("package work ran before preflight")
	}
}

func TestUpdateAsksConflictBeforeFailingPackageOperation(t *testing.T) {
	home := isolatedUpdateHome(t)
	root := testRelease(t)
	t.Setenv("SELFISHELL_TEST_SYSTEM_NAME", "Darwin")
	t.Setenv("SHELL", "/bin/zsh")
	if code, _, stderr := commandResult(root, "install", "--skip-packages", "--yes"); code != 0 {
		t.Fatal(stderr)
	}
	target := home + "/.config/selfishell/zsh/completion.zsh"
	if err := os.WriteFile(target, []byte("personal completion\n"), 0600); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	log := home + "/brew-called"
	if err := os.WriteFile(bin+"/brew", []byte("#!/bin/sh\nprintf called > \"$SELFISHELL_TEST_BREW_CALLS\"\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SELFISHELL_TEST_BREW_CALLS", log)
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	t.Setenv("SELFISHELL_TEST_TTY", "1")
	var out, stderr bytes.Buffer
	code := (CLI{Root: root, In: strings.NewReader("y\nn\n"), Out: &out, Err: &stderr}).Run([]string{"update", "--tools-only"})
	if code == 0 || !strings.Contains(out.String(), "Managed file was modified") || strings.Contains(out.String(), "synchronized") {
		t.Fatalf("prompt/package order: %d %q %q", code, out.String(), stderr.String())
	}
	if _, err := os.Stat(log); err != nil {
		t.Fatalf("package fake did not fail after prompt: %v", err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "personal completion\n" {
		t.Fatalf("conflict changed: %q %v", data, err)
	}
}

func TestToolsPhaseRequiredPackageFailureLeavesConfigurationUnchanged(t *testing.T) {
	home := isolatedUpdateHome(t)
	root := t.TempDir()
	if err := os.Symlink(testRelease(t)+"/config", root+"/config"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(testRelease(t)+"/dependencies.conf", root+"/dependencies.conf"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/packages.conf", []byte("package ubuntu required apt fixture-unavailable\n"), 0600); err != nil {
		t.Fatal(err)
	}
	state := home + "/.local/state/selfishell"
	if err := os.MkdirAll(state, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(state+"/configured", []byte("1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	tools := t.TempDir()
	for name, body := range map[string]string{"dpkg-query": "#!/bin/sh\nexit 1\n", "apt-get": "#!/bin/sh\nprintf called > \"$HOME/apt-called\"\nexit 1\n", "sudo": "#!/bin/sh\nexec \"$@\"\n"} {
		if err := os.WriteFile(tools+"/"+name, []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	osRelease := tools + "/os-release"
	if err := os.WriteFile(osRelease, []byte("ID=ubuntu\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tools+":/usr/bin:/bin")
	t.Setenv("SELFISHELL_TEST_SYSTEM_NAME", "Linux")
	t.Setenv("SELFISHELL_TEST_OS_RELEASE_FILE", osRelease)
	code, out, stderr := commandResult(root, "update", "--tools-only", "--yes")
	if code != 1 || strings.Contains(out, "synchronized") || !strings.Contains(stderr, "Could not update apt package indexes") {
		t.Fatalf("required: %d %q %q", code, out, stderr)
	}
	if _, err := os.Lstat(home + "/.zshrc"); !os.IsNotExist(err) {
		t.Fatalf("package failure applied configuration: %v", err)
	}
	if _, err := os.Stat(home + "/apt-called"); err != nil {
		t.Fatal("required package fake did not run")
	}
}

func TestToolsPhaseChecksApprovedPinsBeforePackages(t *testing.T) {
	home := isolatedUpdateHome(t)
	root := t.TempDir()
	if err := os.Symlink(testRelease(t)+"/config", root+"/config"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(testRelease(t)+"/dependencies.conf", root+"/dependencies.conf"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/packages.conf", []byte("package ubuntu required apt fixture-apt\npackage all required mise unknown-tool\n"), 0600); err != nil {
		t.Fatal(err)
	}
	state := home + "/.local/state/selfishell"
	if err := os.MkdirAll(state, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(state+"/configured", []byte("1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	tools := t.TempDir()
	if err := os.WriteFile(tools+"/dpkg-query", []byte("#!/bin/sh\nprintf called > \"$HOME/package-called\"\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	osRelease := tools + "/os-release"
	if err := os.WriteFile(osRelease, []byte("ID=ubuntu\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tools+":/usr/bin:/bin")
	t.Setenv("SELFISHELL_TEST_SYSTEM_NAME", "Linux")
	t.Setenv("SELFISHELL_TEST_OS_RELEASE_FILE", osRelease)
	code, _, stderr := commandResult(root, "update", "--tools-only", "--yes")
	if code != 1 || !strings.Contains(stderr, "missing approved mise version") {
		t.Fatalf("pins: %d %q", code, stderr)
	}
	if _, err := os.Lstat(home + "/package-called"); !os.IsNotExist(err) {
		t.Fatal("package query ran before pin check")
	}
}

func TestToolsOnlyCancelledBeforeConfigurationHasNoEffects(t *testing.T) {
	home := isolatedUpdateHome(t)
	root := testRelease(t)
	t.Setenv("SELFISHELL_TEST_SYSTEM_NAME", "Darwin")
	state := home + "/.local/state/selfishell"
	if err := os.MkdirAll(state, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(state+"/configured", []byte("1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, stderr bytes.Buffer
	code := (CLI{Root: root, Context: ctx, In: strings.NewReader(""), Out: &out, Err: &stderr}).Run([]string{"update", "--tools-only", "--skip-packages", "--yes"})
	if code != 1 || !strings.Contains(stderr.String(), "context canceled") {
		t.Fatalf("cancel: %d %q %q", code, out.String(), stderr.String())
	}
	if _, err := os.Lstat(home + "/.config/selfishell"); !os.IsNotExist(err) {
		t.Fatalf("cancel applied configuration: %v", err)
	}
}

func updateCleanupFixture(t *testing.T, optional, failPrune bool) (string, string, string) {
	t.Helper()
	_, paths, initialRoot, manifest, home, _ := neovimFixture(t)
	share := home + "/.local/share/selfishell"
	root := share + "/releases/1.0.0"
	if err := os.MkdirAll(filepath.Dir(root), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(initialRoot, root); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(root + "/config"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(testRelease(t)+"/config", root+"/config"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/VERSION", []byte("1.0.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root+"/bin", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/bin/selfishell", []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("releases/1.0.0", share+"/current"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(manifest, root+"/dependencies.conf"); err != nil {
		t.Fatal(err)
	}
	packages := "package all required mise node\n"
	if optional {
		packages += "package ubuntu optional apt fixture-optional\n"
	}
	if err := os.WriteFile(root+"/packages.conf", []byte(packages), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.State, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.State+"/configured", []byte("1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	bin := home + "/bin"
	mise := `#!/bin/sh
printf '%s\n' "$*" >> "$HOME/mise-calls"
case "$*" in
  *'settings get ignored_config_paths') printf '[]\n' ;;
  *'config ls --tracked-configs') printf '%s/config/shared/mise.toml\n' "$SELFISHELL_TEST_RELEASE_ROOT" ;;
  *'which nvim') exit 1 ;;
  *' exec -- '*) while [ "$1" != "--" ]; do shift; done; shift; exec "$@" ;;
  *'prune --tools --yes'*) exit "${FAIL_PRUNE:-0}" ;;
esac
`
	if err := os.WriteFile(bin+"/mise", []byte(mise), 0755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"dpkg-query": "#!/bin/sh\nexit 1\n",
		"apt-get":    "#!/bin/sh\nexit 0\n",
		"apt-cache":  "#!/bin/sh\nexit 1\n",
		"sudo":       "#!/bin/sh\nexec \"$@\"\n",
	} {
		if err := os.WriteFile(bin+"/"+name, []byte(body), 0755); err != nil {
			t.Fatal(err)
		}
	}
	osRelease := home + "/os-release"
	if err := os.WriteFile(osRelease, []byte("ID=ubuntu\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SELFISHELL_TEST_SYSTEM_NAME", "Linux")
	t.Setenv("SELFISHELL_TEST_OS_RELEASE_FILE", osRelease)
	t.Setenv("SELFISHELL_TEST_RELEASE_ROOT", root)
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	t.Setenv("MISE_IGNORED_CONFIG_PATHS", "")
	if failPrune {
		t.Setenv("FAIL_PRUNE", "9")
	} else {
		t.Setenv("FAIL_PRUNE", "0")
	}
	return root, home, home + "/mise-calls"
}

func TestUpdatePrunesOnlyAfterCompleteToolsAndEditor(t *testing.T) {
	root, home, log := updateCleanupFixture(t, false, false)
	code, out, stderr := commandResult(root, "update", "--tools-only", "--yes")
	if code != 0 || !strings.Contains(out, "Selfishell tools and configuration synchronized") {
		t.Fatalf("complete: %d %q %q", code, out, stderr)
	}
	calls, err := os.ReadFile(log)
	if err != nil || !strings.Contains(string(calls), "prune --tools --yes node") {
		t.Fatalf("no scoped prune: %q %v", calls, err)
	}
	if _, err := os.Stat(home + "/nvim.log"); err != nil {
		t.Fatal("Neovim phase did not run before cleanup")
	}
}

func TestUpdateOptionalFailureSkipsCleanupButCompletes(t *testing.T) {
	root, _, log := updateCleanupFixture(t, true, false)
	code, out, stderr := commandResult(root, "update", "--tools-only", "--yes")
	if code != 0 || !strings.Contains(out, "synchronized") || !strings.Contains(stderr, "Skipped optional packages") || !strings.Contains(stderr, "Skipping mise cleanup") {
		t.Fatalf("optional: %d %q %q", code, out, stderr)
	}
	calls, err := os.ReadFile(log)
	if err != nil || strings.Contains(string(calls), "prune --tools") {
		t.Fatalf("optional pruned: %q %v", calls, err)
	}
}

func TestUpdateCleanupFailureWarnsAfterSynchronization(t *testing.T) {
	root, _, log := updateCleanupFixture(t, false, true)
	code, out, stderr := commandResult(root, "update", "--tools-only", "--yes")
	if code != 0 || !strings.Contains(out, "synchronized") || !strings.Contains(stderr, "Could not prune unused mise versions") {
		t.Fatalf("cleanup warning: %d %q %q", code, out, stderr)
	}
	calls, _ := os.ReadFile(log)
	if !strings.Contains(string(calls), "prune --tools --yes node") {
		t.Fatalf("cleanup not attempted: %q", calls)
	}
}

func TestUpdateEditorFailureStopsBeforeCleanup(t *testing.T) {
	root, home, log := updateCleanupFixture(t, false, false)
	if err := os.WriteFile(home+"/bin/nvim", []byte("#!/bin/sh\nexit 9\n"), 0755); err != nil {
		t.Fatal(err)
	}
	code, out, stderr := commandResult(root, "update", "--tools-only", "--yes")
	if code != 1 || strings.Contains(out, "synchronized") || stderr == "" {
		t.Fatalf("editor failure: %d %q %q", code, out, stderr)
	}
	calls, err := os.ReadFile(log)
	if err != nil || strings.Contains(string(calls), "prune --tools") {
		t.Fatalf("editor failure pruned: %q %v", calls, err)
	}
}

func TestUpdateNeverPrunesAnEmptyToolScope(t *testing.T) {
	root, _, log := updateCleanupFixture(t, false, false)
	if err := os.WriteFile(root+"/packages.conf", nil, 0600); err != nil {
		t.Fatal(err)
	}
	code, out, stderr := commandResult(root, "update", "--tools-only", "--yes")
	if code != 0 || !strings.Contains(out, "synchronized") {
		t.Fatalf("empty scope: %d %q %q", code, out, stderr)
	}
	calls, err := os.ReadFile(log)
	if err != nil || strings.Contains(string(calls), "prune --tools") {
		t.Fatalf("empty scope pruned: %q %v", calls, err)
	}
}

func TestUpdateDoesNotSwallowInterruptDuringConfigurationTrust(t *testing.T) {
	// Run an actual CLI process: SIGINT must retain the normal process behavior
	// while trustMise uses its existing background context.
	home := isolatedUpdateHome(t)
	t.Setenv("SELFISHELL_TEST_SYSTEM_NAME", "Darwin")
	t.Setenv("SHELL", "/bin/zsh")
	root := t.TempDir()
	if err := os.Mkdir(root+"/bin", 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"config", "packages.conf", "dependencies.conf"} {
		if err := os.Symlink(filepath.Join(testRelease(t), name), filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	candidate := root + "/bin/selfishell"
	buildNativeTestCLI(t, testRelease(t), home, candidate)
	code, _, stderr := commandResult(root, "install", "--skip-packages", "--yes")
	if code != 0 {
		t.Fatal(stderr)
	}
	bin := t.TempDir()
	marker := home + "/trust-child"
	script := "#!/bin/sh\nprintf '%s\\n' $$ > \"$SELFISHELL_TEST_TRUST_CHILD\"\nexec sleep 30\n"
	if err := os.WriteFile(bin+"/mise", []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(candidate, "update", "--tools-only", "--skip-packages", "--yes")
	cmd.Env = append(os.Environ(), "PATH="+bin+":/usr/bin:/bin", "SELFISHELL_TEST_TRUST_CHILD="+marker)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	outFile, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer outFile.Close()
	errFile, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	defer errFile.Close()
	cmd.Stdout, cmd.Stderr = outFile, errFile
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill() }()
	var childPID int
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(marker); err == nil {
			childPID, _ = strconv.Atoi(strings.TrimSpace(string(data)))
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if childPID == 0 {
		t.Fatalf("mise trust did not start; stdout %q stderr %q", readUpdateTestFile(outFile.Name()), readUpdateTestFile(errFile.Name()))
	}
	defer syscall.Kill(childPID, syscall.SIGKILL)
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err == nil || strings.Contains(readUpdateTestFile(outFile.Name()), "synchronized") {
			t.Fatalf("interrupt swallowed: %v %q %q", err, readUpdateTestFile(outFile.Name()), readUpdateTestFile(errFile.Name()))
		}
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("update retained SIGINT handler around blocked configuration trust")
	}
}

func TestUpdateCancelsBlockedContinuationAndPreservesFailure(t *testing.T) {
	op, share, _ := releaseFixture(t)
	isolateMiseForHome(t, os.Getenv("HOME"))
	marker := filepath.Join(os.Getenv("HOME"), "child-started")
	t.Setenv("SELFISHELL_TEST_CHILD_STARTED", marker)
	publishReleaseFixture(t, "2.0.0", archiveMember{"VERSION", "", 0, "2.0.0\n", 0644}, archiveMember{"bin/selfishell", "", 0, "#!/bin/sh\nprintf started > \"$SELFISHELL_TEST_CHILD_STARTED\"\nexec sleep 30\n", 0755})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var out, stderr bytes.Buffer
	c := CLI{Root: op.Root, Context: ctx, In: strings.NewReader(""), Out: &out, Err: &stderr}
	done := make(chan int, 1)
	go func() { done <- c.Run([]string{"update", "--version", "2.0.0", "--yes"}) }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(marker); err != nil {
		cancel()
		t.Fatalf("child did not start: %v", err)
	}
	cancel()
	select {
	case code := <-done:
		if code != 130 || strings.Contains(out.String(), "Selfishell updated") {
			t.Fatalf("canceled child: %d %q %q", code, out.String(), stderr.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("blocked continuation was not reaped after cancellation")
	}
	current, _ := os.Readlink(share + "/current")
	if current != "releases/2.0.0" {
		t.Fatalf("canceled child changed CLI selection: %s", current)
	}
}

func readUpdateTestFile(path string) string { data, _ := os.ReadFile(path); return string(data) }

func TestUpdatePreflightsUserBlocksAndSavedGhosttyBeforeAnyWrite(t *testing.T) {
	for _, tc := range []struct{ name, platform, target, kind string }{
		{"zshenv-link", "ubuntu", ".zshenv", "link"},
		{"ghostty-link", "macos", ".config/ghostty/config.ghostty", "link"},
		{"ghostty-directory", "macos", ".config/ghostty/config.ghostty", "directory"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := isolatedUpdateHome(t)
			root := testRelease(t)
			t.Setenv("SHELL", "/bin/zsh")
			if tc.platform == "macos" {
				t.Setenv("SELFISHELL_TEST_SYSTEM_NAME", "Darwin")
			} else {
				t.Setenv("SELFISHELL_TEST_SYSTEM_NAME", "Linux")
				osRelease := home + "/os-release"
				if err := os.WriteFile(osRelease, []byte("ID=ubuntu\n"), 0600); err != nil {
					t.Fatal(err)
				}
				t.Setenv("SELFISHELL_TEST_OS_RELEASE_FILE", osRelease)
			}
			code, _, stderr := commandResult(root, "install", "--skip-packages", "--yes")
			if code != 0 {
				t.Fatalf("setup: %s", stderr)
			}
			target := home + "/" + tc.target
			if err := os.Remove(target); err != nil {
				t.Fatal(err)
			}
			if tc.kind == "link" {
				foreign := home + "/foreign-" + tc.name
				if err := os.WriteFile(foreign, []byte("personal\n"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(foreign, target); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.MkdirAll(target+"/keep", 0700); err != nil {
					t.Fatal(err)
				}
			}
			zshBefore, _ := os.ReadFile(home + "/.zshrc")
			state := home + "/.local/state/selfishell/resources/vimrc.state"
			stateBefore, _ := os.ReadFile(state)
			cache := home + "/.cache/selfishell/zoxide-init.zsh"
			if err := os.MkdirAll(filepath.Dir(cache), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(cache, []byte("stale"), 0600); err != nil {
				t.Fatal(err)
			}
			code, out, stderr := commandResult(root, "update", "--tools-only", "--skip-packages", "--yes")
			if code != 1 || strings.Contains(out, "synchronized") || stderr == "" {
				t.Fatalf("preflight: %d %q %q", code, out, stderr)
			}
			info, err := os.Lstat(target)
			if err != nil {
				t.Fatal(err)
			}
			if tc.kind == "link" && info.Mode()&os.ModeSymlink == 0 || tc.kind == "directory" && !info.IsDir() {
				t.Fatal("foreign path replaced")
			}
			zshAfter, _ := os.ReadFile(home + "/.zshrc")
			stateAfter, _ := os.ReadFile(state)
			if !bytes.Equal(zshBefore, zshAfter) || !bytes.Equal(stateBefore, stateAfter) {
				t.Fatal("preflight mutated unrelated resource")
			}
			if data, err := os.ReadFile(cache); err != nil || string(data) != "stale" {
				t.Fatal("preflight cleared cache")
			}
		})
	}
}

func TestUpdateInteractiveManagedFileOverwriteAndSkip(t *testing.T) {
	for _, accept := range []bool{true, false} {
		name := "skip"
		if accept {
			name = "overwrite"
		}
		t.Run(name, func(t *testing.T) {
			home := isolatedUpdateHome(t)
			root := testRelease(t)
			t.Setenv("SELFISHELL_TEST_SYSTEM_NAME", "Darwin")
			t.Setenv("SHELL", "/bin/zsh")
			code, _, stderr := commandResult(root, "install", "--skip-packages", "--yes")
			if code != 0 {
				t.Fatal(stderr)
			}
			target := home + "/.config/selfishell/zsh/completion.zsh"
			state := home + "/.local/state/selfishell/resources/zsh-completion.state"
			stateBefore, _ := os.ReadFile(state)
			if err := os.WriteFile(target, []byte("personal completion\n"), 0600); err != nil {
				t.Fatal(err)
			}
			later := home + "/.config/selfishell/vim/vimrc"
			if err := os.Remove(later); err != nil {
				t.Fatal(err)
			}
			t.Setenv("SELFISHELL_TEST_TTY", "1")
			answer := "n"
			if accept {
				answer = "y"
			}
			var out, errOut bytes.Buffer
			cli := CLI{Root: root, In: strings.NewReader("y\n" + answer + "\n"), Out: &out, Err: &errOut}
			code = cli.Run([]string{"update", "--tools-only", "--skip-packages"})
			if code != 0 {
				t.Fatalf("interactive: %d %q %q", code, out.String(), errOut.String())
			}
			if _, err := os.Stat(later); err != nil {
				t.Fatal("later resource not applied")
			}
			got, _ := os.ReadFile(target)
			stateAfter, _ := os.ReadFile(state)
			if accept {
				if string(got) == "personal completion\n" || !bytes.Equal(stateBefore, stateAfter) {
					t.Fatal("accepted overwrite did not restore approved content/state")
				}
				backups, _ := filepath.Glob(home + "/.local/state/selfishell/backups/zsh-completion.backup.*")
				if len(backups) != 1 {
					t.Fatalf("missing conflict backup: %v", backups)
				}
				backup, _ := os.ReadFile(backups[0])
				if string(backup) != "personal completion\n" {
					t.Fatalf("wrong backup: %q", backup)
				}
			} else if string(got) != "personal completion\n" || !bytes.Equal(stateBefore, stateAfter) {
				t.Fatal("skipped resource/state changed")
			}
		})
	}
}
