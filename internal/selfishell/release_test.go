package selfishell

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSelectedChecksumIgnoresCompanionAssets(t *testing.T) {
	name := "selfishell-2.0.0-linux-amd64.tar.gz"
	sum := strings.Repeat("a", 64)
	manifest := sum + "  " + name + "\n" + sum + "  " + name + ".sig\n" + sum + "  " + name + ".sbom.json\n"
	if got, err := selectedChecksum([]byte(manifest), name); err != nil || got != sum {
		t.Fatalf("checksum %q, %v", got, err)
	}
	for _, row := range []string{name, "bad  " + name, sum + "  " + name + " extra", strings.Repeat("b", 64) + "  " + name} {
		if _, err := selectedChecksum([]byte(manifest+row+"\n"), name); err == nil {
			t.Fatalf("accepted selected archive row %q", row)
		}
	}
}

func TestPruneInactiveReleasesRequiresValidCurrentLink(t *testing.T) {
	_, share, releases := releaseFixture(t)
	stale := releases + "/0.9.0"
	if err := os.MkdirAll(stale+"/bin", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale+"/VERSION", []byte("0.9.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale+"/bin/selfishell", []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(share + "/current"); err != nil {
		t.Fatal(err)
	}
	pruneInactiveReleases(releaseLayout{current: share + "/current", previous: share + "/previous", releases: releases})
	if _, err := os.Stat(stale); err != nil {
		t.Fatalf("pruned with unknown current: %v", err)
	}
}

func TestReleaseVersionFixture(t *testing.T) {
	path := filepath.Join("..", "..", "tests", "fixtures", "version-precedence.txt")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 {
			t.Fatalf("bad fixture %q", scanner.Text())
		}
		got := compareReleaseVersions(fields[0], fields[1]) > 0
		if got != (fields[2] == "1") {
			t.Errorf("%s > %s: got %v, want %s", fields[0], fields[1], got, fields[2])
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseLatestMetadata(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	t.Setenv("SELFISHELL_RELEASE_ROOT", "file://"+root)
	t.Setenv("SELFISHELL_RELEASE_TAGS_API_URL", "")
	if err := os.MkdirAll(root+"/latest/download", 0700); err != nil {
		t.Fatal(err)
	}
	op := releaseOperation{Process: Process{Err: &bytes.Buffer{}}}
	for _, tc := range []struct{ body, want string }{{"1.2.3\n", "1.2.3"}, {"v1.2.3\n", "1.2.3"}, {"../escape\n", ""}, {"1.2.3+build\n", ""}, {"\n", ""}} {
		if err := os.WriteFile(root+"/latest/download/VERSION", []byte(tc.body), 0600); err != nil {
			t.Fatal(err)
		}
		got, err := op.latest(context.Background())
		if tc.want == "" {
			if err == nil {
				t.Errorf("accepted %q as %q", tc.body, got)
			}
		} else if err != nil || got != tc.want {
			t.Errorf("%q: got %q, %v", tc.body, got, err)
		}
	}
}

func TestReleaseLatestPublishedTagFallback(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	t.Setenv("SELFISHELL_RELEASE_ROOT", "file://"+root)
	tags := root + "/tags.json"
	t.Setenv("SELFISHELL_RELEASE_TAGS_API_URL", "file://"+tags)
	if err := os.MkdirAll(root+"/download/v0.3.0-beta.2", 0700); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(tags, []byte(`[{"name":"v0.3.0-beta.2"}]`), 0600)
	published := root + "/download/v0.3.0-beta.2/VERSION"
	os.WriteFile(published, []byte("0.3.0-beta.2\n"), 0600)
	op := releaseOperation{Process: Process{Err: &bytes.Buffer{}}}
	got, err := op.latest(context.Background())
	if err != nil || got != "0.3.0-beta.2" {
		t.Fatalf("got %q, %v", got, err)
	}
	for _, value := range []string{"9.9.9\n", "../bad\n"} {
		os.WriteFile(published, []byte(value), 0600)
		if got, err := op.latest(context.Background()); err == nil {
			t.Errorf("accepted mismatched VERSION %q as %q", value, got)
		}
	}
	for _, value := range []string{`{`, `[]`, `[{"name":"../bad"}]`} {
		os.WriteFile(tags, []byte(value), 0600)
		if got, err := op.latest(context.Background()); err == nil {
			t.Errorf("accepted tags %q as %q", value, got)
		}
	}
}

func TestVersionAvailableCLI(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	t.Setenv("SELFISHELL_RELEASE_ROOT", "file://"+root)
	os.MkdirAll(root+"/latest/download", 0700)
	os.WriteFile(root+"/latest/download/VERSION", []byte("1.2.3\n"), 0600)
	var out, stderr bytes.Buffer
	c := CLI{Root: root, Out: &out, Err: &stderr}
	if code := c.Run([]string{"version", "--available"}); code != 0 || out.String() != "1.2.3\n" {
		t.Fatalf("code %d, stdout %q, stderr %q", code, out.String(), stderr.String())
	}
	os.WriteFile(root+"/latest/download/VERSION", []byte("unsafe\n"), 0600)
	out.Reset()
	stderr.Reset()
	if code := c.Run([]string{"version", "--available"}); code != 1 || out.Len() != 0 || !strings.Contains(stderr.String(), "Unable to determine the latest Selfishell release") {
		t.Fatal(fmt.Sprintf("code %d, stdout %q, stderr %q", code, out.String(), stderr.String()))
	}
}

func releaseFixture(t *testing.T) (releaseOperation, string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	share := home + "/share/selfishell"
	releases := share + "/releases"
	root := releases + "/1.0.0"
	if err := os.MkdirAll(root+"/bin", 0700); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(root+"/VERSION", []byte("1.0.0\n"), 0600)
	os.WriteFile(root+"/bin/selfishell", []byte("#!/bin/sh\n"), 0755)
	if err := os.Symlink("releases/1.0.0", share+"/current"); err != nil {
		t.Fatal(err)
	}
	return releaseOperation{Root: root, Process: Process{Err: &bytes.Buffer{}}}, share, releases
}

func publishReleaseFixture(t *testing.T, version string, members ...archiveMember) string {
	t.Helper()
	remote := t.TempDir()
	name, err := releaseArchiveName(version)
	if err != nil {
		t.Fatal(err)
	}
	dir := remote + "/download/v" + version
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	archive := testArchive(t, members...)
	data, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/"+name, data, 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	if err := os.WriteFile(dir+"/SHA256SUMS", []byte(hex.EncodeToString(digest[:])+"  "+name+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SELFISHELL_RELEASE_ROOT", "file://"+remote)
	return dir
}

func TestReleaseInstallExactAndRetainsRollback(t *testing.T) {
	op, share, releases := releaseFixture(t)
	dir := publishReleaseFixture(t, "2.0.0", archiveMember{"VERSION", "", 0, "2.0.0\n", 0644}, archiveMember{"bin/selfishell", "", 0, "#!/bin/sh\n", 0755}, archiveMember{"bin/sfs", "selfishell", tar.TypeSymlink, "", 0})
	checksum := dir + "/SHA256SUMS"
	original, _ := os.ReadFile(checksum)
	os.WriteFile(checksum, append(original, original...), 0600)
	os.MkdirAll(releases+"/0.9.0/bin", 0700)
	os.WriteFile(releases+"/0.9.0/VERSION", []byte("0.9.0\n"), 0600)
	os.WriteFile(releases+"/0.9.0/bin/selfishell", []byte("#!/bin/sh\n"), 0755)
	got, err := op.install(context.Background(), "2.0.0")
	if err != nil || filepath.Base(got) != "2.0.0" {
		t.Fatalf("install %q, %v", got, err)
	}
	current, _ := os.Readlink(share + "/current")
	previous, _ := os.Readlink(share + "/previous")
	if current != "releases/2.0.0" || previous != "releases/1.0.0" {
		t.Fatalf("links %q, %q", current, previous)
	}
	if _, err := os.Stat(releases + "/0.9.0"); !os.IsNotExist(err) {
		t.Errorf("obsolete release retained: %v", err)
	}
	if _, err := os.Lstat(releases + "/1.0.0"); err != nil {
		t.Fatal(err)
	}
	if link, err := os.Readlink(got + "/bin/sfs"); err != nil || link != "selfishell" {
		t.Fatalf("sfs %q %v", link, err)
	}
}

func TestReleaseInstallRejectsConflictBeforeActivation(t *testing.T) {
	op, share, releases := releaseFixture(t)
	dir := publishReleaseFixture(t, "2.0.0", archiveMember{"VERSION", "", 0, "2.0.0\n", 0644}, archiveMember{"bin/selfishell", "", 0, "#!/bin/sh\n", 0755})
	name, _ := releaseArchiveName("2.0.0")
	checksum := dir + "/SHA256SUMS"
	os.WriteFile(checksum, []byte(strings.Repeat("0", 64)+"  "+name+"\n"), 0600)
	if _, err := op.install(context.Background(), "2.0.0"); err == nil {
		t.Fatal("accepted checksum mismatch")
	}
	os.WriteFile(checksum, []byte("bad\n"), 0600)
	if _, err := op.install(context.Background(), "2.0.0"); err == nil {
		t.Fatal("accepted missing checksum")
	}
	valid, _ := os.ReadFile(dir + "/" + name)
	digest := sha256.Sum256(valid)
	os.WriteFile(checksum, []byte(hex.EncodeToString(digest[:])+"  "+name+"\n"+strings.Repeat("0", 64)+"  "+name+"\n"), 0600)
	if _, err := op.install(context.Background(), "2.0.0"); err == nil {
		t.Fatal("accepted conflicting checksum")
	}
	current, _ := os.Readlink(share + "/current")
	if current != "releases/1.0.0" {
		t.Fatal(current)
	}
	if _, err := os.Lstat(releases + "/2.0.0"); !os.IsNotExist(err) {
		t.Fatalf("partial target: %v", err)
	}
}

func TestReleaseInstallUsesExactVersionURL(t *testing.T) {
	op, share, releases := releaseFixture(t)
	remote := t.TempDir()
	t.Setenv("SELFISHELL_RELEASE_ROOT", "file://"+remote)
	os.MkdirAll(remote+"/latest/download", 0700)
	os.WriteFile(remote+"/latest/download/VERSION", []byte("2.0.0\n"), 0600)
	if _, err := op.install(context.Background(), "9.9.9"); err == nil {
		t.Fatal("fell back to latest")
	}
	current, _ := os.Readlink(share + "/current")
	if current != "releases/1.0.0" {
		t.Fatal(current)
	}
	if _, err := os.Lstat(releases + "/9.9.9"); !os.IsNotExist(err) {
		t.Fatalf("created missing exact version: %v", err)
	}
}

func TestReleaseInstallPreflightsOccupiedAndForeignLinks(t *testing.T) {
	for _, tc := range []struct{ link, kind, want string }{
		{"previous", "file", "occupied release link"},
		{"previous", "link", "foreign release link"},
		{"current", "file", "versioned Selfishell installation"},
		{"current", "link", "foreign release link"},
	} {
		t.Run(tc.link+"-"+tc.kind, func(t *testing.T) {
			op, share, releases := releaseFixture(t)
			path := share + "/" + tc.link
			os.Remove(path)
			if tc.kind == "file" {
				writeTestFile(t, path, "user data\n", 0600)
			} else if err := os.Symlink("/personal", path); err != nil {
				t.Fatal(err)
			}
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
			defer server.Close()
			t.Setenv("SELFISHELL_RELEASE_ROOT", server.URL)
			if _, err := op.install(context.Background(), "2.0.0"); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("%s %s was not rejected: %v", tc.kind, tc.link, err)
			}
			if got := requests.Load(); got != 0 {
				t.Fatalf("contacted release source before link preflight: %d requests", got)
			}
			if tc.kind == "file" {
				if data, _ := os.ReadFile(path); string(data) != "user data\n" {
					t.Fatalf("%s changed: %q", tc.link, data)
				}
			} else if link, _ := os.Readlink(path); link != "/personal" {
				t.Fatalf("%s changed: %q", tc.link, link)
			}
			if current, _ := os.Readlink(share + "/current"); tc.link == "previous" && current != "releases/1.0.0" {
				t.Fatalf("current changed: %q", current)
			}
			if _, err := validReleaseDirectory(releases, "1.0.0"); err != nil {
				t.Fatalf("old active release removed: %v", err)
			}
			if _, err := os.Lstat(releases + "/2.0.0"); !os.IsNotExist(err) {
				t.Fatalf("target created before link preflight: %v", err)
			}
		})
	}
}

func TestReleaseInstallKeepsForeignReleaseEntries(t *testing.T) {
	op, _, releases := releaseFixture(t)
	publishReleaseFixture(t, "2.0.0", archiveMember{"VERSION", "", 0, "2.0.0\n", 0644}, archiveMember{"bin/selfishell", "", 0, "#!/bin/sh\n", 0755})
	writeTestFile(t, op.Root+"/VERSION", "wrong\n", 0600)
	if _, err := op.install(context.Background(), "2.0.0"); err == nil {
		t.Fatal("accepted corrupt running root")
	}
	writeTestFile(t, op.Root+"/VERSION", "1.0.0\n", 0600)
	writeTestFile(t, releases+"/0.9.0/personal", "foreign\n", 0600)
	linkTarget := t.TempDir()
	os.Symlink(linkTarget, releases+"/0.8.0")
	stale := releases + "/.9.9.9.tmp.stale"
	fresh := releases + "/.9.9.9.tmp.fresh"
	foreignStage := releases + "/.personal.tmp.docs"
	os.Mkdir(stale, 0700)
	os.Mkdir(fresh, 0700)
	os.Mkdir(foreignStage, 0700)
	old := time.Now().Add(-25 * time.Hour)
	os.Chtimes(stale, old, old)
	os.Chtimes(foreignStage, old, old)
	if _, err := op.install(context.Background(), "2.0.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(releases + "/0.9.0/personal"); err != nil {
		t.Fatal(err)
	}
	if got, err := os.Readlink(releases + "/0.8.0"); err != nil || got != linkTarget {
		t.Fatalf("foreign symlink changed: %q, %v", got, err)
	}
	if _, err := os.Lstat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale stage retained: %v", err)
	}
	if _, err := os.Lstat(fresh); err != nil {
		t.Fatalf("fresh stage deleted: %v", err)
	}
	if _, err := os.Lstat(foreignStage); err != nil {
		t.Fatalf("foreign directory deleted: %v", err)
	}
}

func TestReleaseInstallRestoresPreviousAfterActivationFailure(t *testing.T) {
	op, share, _ := releaseFixture(t)
	publishReleaseFixture(t, "2.0.0", archiveMember{"VERSION", "", 0, "2.0.0\n", 0644}, archiveMember{"bin/selfishell", "", 0, "#!/bin/sh\n", 0755})
	if err := os.Symlink("releases/0.9.0", share+"/previous"); err != nil {
		t.Fatal(err)
	}
	op.link = func(target, path string) error {
		if target == "releases/2.0.0" {
			return fmt.Errorf("activation failed")
		}
		return atomicReleaseLink(target, path)
	}
	if _, err := op.install(context.Background(), "2.0.0"); err == nil {
		t.Fatal("reported activation success")
	}
	if got, _ := os.Readlink(share + "/previous"); got != "releases/0.9.0" {
		t.Fatalf("previous=%q", got)
	}
	if got, _ := os.Readlink(share + "/current"); got != "releases/1.0.0" {
		t.Fatalf("current=%q", got)
	}
}

func TestReleaseInstallRejectsExistingIncompleteAndSymlinkTargets(t *testing.T) {
	op, share, releases := releaseFixture(t)
	publishReleaseFixture(t, "2.0.0", archiveMember{"VERSION", "", 0, "2.0.0\n", 0644}, archiveMember{"bin/selfishell", "", 0, "#!/bin/sh\n", 0755})
	target := releases + "/2.0.0"
	os.MkdirAll(target+"/bin", 0700)
	os.WriteFile(target+"/bin/selfishell", []byte("not executable"), 0600)
	if _, err := op.install(context.Background(), "2.0.0"); err == nil {
		t.Fatal("accepted incomplete existing release")
	}
	os.RemoveAll(target)
	os.Symlink(releases+"/1.0.0", target)
	if _, err := op.install(context.Background(), "2.0.0"); err == nil {
		t.Fatal("accepted symlinked existing release")
	}
	link, _ := os.Readlink(target)
	if link != releases+"/1.0.0" {
		t.Fatalf("replaced symlink: %q", link)
	}
	current, _ := os.Readlink(share + "/current")
	if current != "releases/1.0.0" {
		t.Fatal(current)
	}
	os.Remove(target)
	os.MkdirAll(target+"/bin", 0700)
	os.WriteFile(target+"/VERSION", []byte("9.9.9\n"), 0600)
	os.WriteFile(target+"/bin/selfishell", []byte("#!/bin/sh\n"), 0755)
	if _, err := op.install(context.Background(), "2.0.0"); err == nil {
		t.Fatal("accepted wrong VERSION in existing release")
	}
}

func TestReleaseInstallSameVersionPreservesPreviousLink(t *testing.T) {
	op, share, _ := releaseFixture(t)
	publishReleaseFixture(t, "1.0.0", archiveMember{"VERSION", "", 0, "1.0.0\n", 0644}, archiveMember{"bin/selfishell", "", 0, "#!/bin/sh\n", 0755})
	os.Symlink("releases/0.9.0", share+"/previous")
	if _, err := op.install(context.Background(), "1.0.0"); err != nil {
		t.Fatal(err)
	}
	previous, _ := os.Readlink(share + "/previous")
	if previous != "releases/0.9.0" {
		t.Fatal(previous)
	}
}

func TestReleaseInstallPreflightsWholeArchiveBeforeFilesystemMutation(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprintf("existing=%t", existing), func(t *testing.T) {
			op, share, releases := releaseFixture(t)
			publishReleaseFixture(t, "2.0.0", archiveMember{"VERSION", "", 0, "2.0.0\n", 0644}, archiveMember{"bin/selfishell", "", 0, "#!/bin/sh\n", 0755}, archiveMember{"../sentinel", "", 0, "overwrite", 0600})
			if existing {
				writeTestFile(t, releases+"/2.0.0/VERSION", "2.0.0\n", 0600)
				writeTestFile(t, releases+"/2.0.0/bin/selfishell", "winner binary\n", 0755)
			}
			downloads := t.TempDir()
			t.Setenv("TMPDIR", downloads)
			sentinel := filepath.Dir(share) + "/sentinel"
			os.WriteFile(sentinel, []byte("original\n"), 0600)
			os.Symlink("releases/0.9.0", share+"/previous")
			if _, err := op.install(context.Background(), "2.0.0"); err == nil {
				t.Fatal("accepted traversal archive")
			}
			data, _ := os.ReadFile(sentinel)
			if string(data) != "original\n" {
				t.Fatalf("outside sentinel changed: %q", data)
			}
			current, _ := os.Readlink(share + "/current")
			previous, _ := os.Readlink(share + "/previous")
			if current != "releases/1.0.0" || previous != "releases/0.9.0" {
				t.Fatalf("links changed: %q, %q", current, previous)
			}
			if _, err := os.Lstat(releases + "/2.0.0"); !existing && !os.IsNotExist(err) {
				t.Fatalf("created invalid target: %v", err)
			}

			if existing && readTestFile(t, releases+"/2.0.0/bin/selfishell") != "winner binary\n" {
				t.Fatal("replaced existing release")
			}
			entries, err := os.ReadDir(releases)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.Contains(entry.Name(), ".tmp.") {
					t.Fatalf("left staging path: %s", entry.Name())
				}
			}
			entries, err = os.ReadDir(downloads)
			if err != nil || len(entries) != 0 {
				t.Fatalf("left download paths: %v %v", entries, err)
			}
		})
	}
}

func TestReleaseInstallActivationFailureDoesNotClaimSuccess(t *testing.T) {
	op, share, releases := releaseFixture(t)
	isolateMiseForHome(t, os.Getenv("HOME"))
	publishReleaseFixture(t, "2.0.0", archiveMember{"VERSION", "", 0, "2.0.0\n", 0644}, archiveMember{"bin/selfishell", "", 0, "#!/bin/sh\n", 0755})
	if err := os.Chmod(share, 0500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(share, 0700)
	if _, err := op.install(context.Background(), "2.0.0"); err == nil {
		t.Fatal("reported activation success without write access")
	}
	current, _ := os.Readlink(share + "/current")
	if current != "releases/1.0.0" {
		t.Fatalf("current changed: %s", current)
	}
	if _, err := os.Stat(releases + "/2.0.0/bin/selfishell"); err != nil {
		t.Fatalf("promoted release cannot be reused: %v", err)
	}
	code, out, stderr := commandResult(op.Root, "update", "--cli-only", "--version", "2.0.0", "--yes")
	if code == 0 || strings.Contains(out, "Selfishell updated") || stderr == "" {
		t.Fatalf("CLI claimed failed activation: code %d, stdout %q, stderr %q", code, out, stderr)
	}
	if err := os.Chmod(share, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := op.install(context.Background(), "2.0.0"); err != nil {
		t.Fatalf("activation retry: %v", err)
	}
	current, _ = os.Readlink(share + "/current")
	previous, _ := os.Readlink(share + "/previous")
	if current != "releases/2.0.0" || previous != "releases/1.0.0" {
		t.Fatalf("retry links: %q, %q", current, previous)
	}
}

func TestReleaseFetchUsesMetadataDeadlineOnlyForMetadata(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SELFISHELL_RELEASE_ROOT", "file://"+home+"/remote")
	t.Setenv("SELFISHELL_CURL_METADATA_MAX_TIME", "7")
	fake := home + "/bin"
	os.Mkdir(fake, 0700)
	log := home + "/curl-args"
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" >\"$CURL_LOG\"\nwhile [ \"$#\" -gt 0 ]; do\n  if [ \"$1\" = '-o' ]; then shift; printf '1.2.3\\n' >\"$1\"; fi\n  shift\ndone\n"
	os.WriteFile(fake+"/curl", []byte(script), 0755)
	op := releaseOperation{Process: Process{Env: append(os.Environ(), "PATH="+fake+":/usr/bin:/bin", "CURL_LOG="+log)}}
	if got, err := op.latest(context.Background()); err != nil || got != "1.2.3" {
		t.Fatalf("metadata %q, %v", got, err)
	}
	args, _ := os.ReadFile(log)
	if !strings.Contains(string(args), "--max-time\n7\n") {
		t.Fatalf("metadata omitted deadline: %q", args)
	}
	if err := op.fetch(context.Background(), "transfer", "file://fixture", home+"/transfer"); err != nil {
		t.Fatal(err)
	}
	args, _ = os.ReadFile(log)
	if strings.Contains(string(args), "--max-time") {
		t.Fatalf("transfer inherited metadata deadline: %q", args)
	}
}
