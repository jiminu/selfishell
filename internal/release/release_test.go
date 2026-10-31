package release

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

	"github.com/jiminu/selfishell/internal/process"
	"github.com/jiminu/selfishell/internal/testutil"
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
	t.Parallel()
	var env process.Environment
	_, share, releases := releaseFixture(t, &env)
	stale := releases + "/0.9.0"
	if err := os.MkdirAll(stale+"/bin", 0700); err != nil {
		t.Fatal(err)
	}
	if err := testutil.WriteFile(stale+"/VERSION", []byte("0.9.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := testutil.WriteFile(stale+"/bin/selfishell", []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(share + "/current"); err != nil {
		t.Fatal(err)
	}
	pruneInactiveReleases(Layout{current: share + "/current", Previous: share + "/previous", releases: releases})
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
		got := CompareVersions(fields[0], fields[1]) > 0
		if got != (fields[2] == "1") {
			t.Errorf("%s > %s: got %v, want %s", fields[0], fields[1], got, fields[2])
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseLatestMetadata(t *testing.T) {
	t.Parallel()
	var env process.Environment
	setEnv(&env, "HOME", t.TempDir())
	root := t.TempDir()
	setEnv(&env, "SELFISHELL_RELEASE_ROOT", "file://"+root)
	setEnv(&env, "SELFISHELL_RELEASE_TAGS_API_URL", "")
	if err := os.MkdirAll(root+"/latest/download", 0700); err != nil {
		t.Fatal(err)
	}
	op := Operation{Process: process.Process{Inherit: env, Err: &bytes.Buffer{}}}
	for _, tc := range []struct{ body, want string }{{"1.2.3\n", "1.2.3"}, {"v1.2.3\n", "1.2.3"}, {"../escape\n", ""}, {"1.2.3+build\n", ""}, {"\n", ""}} {
		if err := testutil.WriteFile(root+"/latest/download/VERSION", []byte(tc.body), 0600); err != nil {
			t.Fatal(err)
		}
		got, err := op.Latest(context.Background())
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
	t.Parallel()
	var env process.Environment
	setEnv(&env, "HOME", t.TempDir())
	root := t.TempDir()
	setEnv(&env, "SELFISHELL_RELEASE_ROOT", "file://"+root)
	tags := root + "/tags.json"
	setEnv(&env, "SELFISHELL_RELEASE_TAGS_API_URL", "file://"+tags)
	if err := os.MkdirAll(root+"/download/v0.3.0-beta.2", 0700); err != nil {
		t.Fatal(err)
	}
	testutil.WriteFile(tags, []byte(`[{"name":"v0.3.0-beta.2"}]`), 0600)
	published := root + "/download/v0.3.0-beta.2/VERSION"
	testutil.WriteFile(published, []byte("0.3.0-beta.2\n"), 0600)
	op := Operation{Process: process.Process{Inherit: env, Err: &bytes.Buffer{}}}
	got, err := op.Latest(context.Background())
	if err != nil || got != "0.3.0-beta.2" {
		t.Fatalf("got %q, %v", got, err)
	}
	for _, value := range []string{"9.9.9\n", "../bad\n"} {
		testutil.WriteFile(published, []byte(value), 0600)
		if got, err := op.Latest(context.Background()); err == nil {
			t.Errorf("accepted mismatched VERSION %q as %q", value, got)
		}
	}
	for _, value := range []string{`{`, `[]`, `[{"name":"../bad"}]`} {
		testutil.WriteFile(tags, []byte(value), 0600)
		if got, err := op.Latest(context.Background()); err == nil {
			t.Errorf("accepted tags %q as %q", value, got)
		}
	}
}

func releaseFixture(t *testing.T, env *process.Environment) (Operation, string, string) {
	t.Helper()
	home := t.TempDir()
	setEnv(env, "HOME", home)
	share := home + "/share/selfishell"
	releases := share + "/releases"
	root := releases + "/1.0.0"
	if err := os.MkdirAll(root+"/bin", 0700); err != nil {
		t.Fatal(err)
	}
	testutil.WriteFile(root+"/VERSION", []byte("1.0.0\n"), 0600)
	testutil.WriteFile(root+"/bin/selfishell", []byte("#!/bin/sh\n"), 0755)
	if err := os.Symlink("releases/1.0.0", share+"/current"); err != nil {
		t.Fatal(err)
	}
	return Operation{Root: root, Process: process.Process{Inherit: *env, Err: &bytes.Buffer{}}}, share, releases
}

func publishReleaseFixture(t *testing.T, env *process.Environment, version string, members ...testutil.ArchiveMember) string {
	t.Helper()
	remote := t.TempDir()
	name, err := ArchiveName(version)
	if err != nil {
		t.Fatal(err)
	}
	dir := remote + "/download/v" + version
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	archive := testutil.Archive(t, members...)
	data, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	if err := testutil.WriteFile(dir+"/"+name, data, 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	if err := testutil.WriteFile(dir+"/SHA256SUMS", []byte(hex.EncodeToString(digest[:])+"  "+name+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	setEnv(env, "SELFISHELL_RELEASE_ROOT", "file://"+remote)
	return dir
}

func TestReleaseInstallExactAndRetainsRollback(t *testing.T) {
	t.Parallel()
	var env process.Environment
	op, share, releases := releaseFixture(t, &env)
	dir := publishReleaseFixture(t, &env, "2.0.0", testutil.ArchiveMember{Name: "VERSION", Body: "2.0.0\n", Mode: 0644}, testutil.ArchiveMember{Name: "bin/selfishell", Body: "#!/bin/sh\n", Mode: 0755}, testutil.ArchiveMember{Name: "bin/sfs", Target: "selfishell", Kind: tar.TypeSymlink})
	op.Process.Inherit = env
	checksum := dir + "/SHA256SUMS"
	original, _ := os.ReadFile(checksum)
	testutil.WriteFile(checksum, append(original, original...), 0600)
	os.MkdirAll(releases+"/0.9.0/bin", 0700)
	testutil.WriteFile(releases+"/0.9.0/VERSION", []byte("0.9.0\n"), 0600)
	testutil.WriteFile(releases+"/0.9.0/bin/selfishell", []byte("#!/bin/sh\n"), 0755)
	got, err := op.Install(context.Background(), "2.0.0")
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
	t.Parallel()
	var env process.Environment
	op, share, releases := releaseFixture(t, &env)
	dir := publishReleaseFixture(t, &env, "2.0.0", testutil.ArchiveMember{Name: "VERSION", Body: "2.0.0\n", Mode: 0644}, testutil.ArchiveMember{Name: "bin/selfishell", Body: "#!/bin/sh\n", Mode: 0755})
	op.Process.Inherit = env
	name, _ := ArchiveName("2.0.0")
	checksum := dir + "/SHA256SUMS"
	testutil.WriteFile(checksum, []byte(strings.Repeat("0", 64)+"  "+name+"\n"), 0600)
	if _, err := op.Install(context.Background(), "2.0.0"); err == nil {
		t.Fatal("accepted checksum mismatch")
	}
	testutil.WriteFile(checksum, []byte("bad\n"), 0600)
	if _, err := op.Install(context.Background(), "2.0.0"); err == nil {
		t.Fatal("accepted missing checksum")
	}
	valid, _ := os.ReadFile(dir + "/" + name)
	digest := sha256.Sum256(valid)
	testutil.WriteFile(checksum, []byte(hex.EncodeToString(digest[:])+"  "+name+"\n"+strings.Repeat("0", 64)+"  "+name+"\n"), 0600)
	if _, err := op.Install(context.Background(), "2.0.0"); err == nil {
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
	t.Parallel()
	var env process.Environment
	op, share, releases := releaseFixture(t, &env)
	remote := t.TempDir()
	setEnv(&env, "SELFISHELL_RELEASE_ROOT", "file://"+remote)
	op.Process.Inherit = env
	os.MkdirAll(remote+"/latest/download", 0700)
	testutil.WriteFile(remote+"/latest/download/VERSION", []byte("2.0.0\n"), 0600)
	if _, err := op.Install(context.Background(), "9.9.9"); err == nil {
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
	t.Parallel()
	var env process.Environment
	for _, tc := range []struct{ link, kind, want string }{
		{"previous", "file", "occupied release link"},
		{"previous", "link", "foreign release link"},
		{"current", "file", "versioned Selfishell installation"},
		{"current", "link", "foreign release link"},
	} {
		t.Run(tc.link+"-"+tc.kind, func(t *testing.T) {
			t.Parallel()
			env := env
			op, share, releases := releaseFixture(t, &env)
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
			setEnv(&env, "SELFISHELL_RELEASE_ROOT", server.URL)
			op.Process.Inherit = env
			if _, err := op.Install(context.Background(), "2.0.0"); err == nil || !strings.Contains(err.Error(), tc.want) {
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
			if _, err := Directory(releases, "1.0.0"); err != nil {
				t.Fatalf("old active release removed: %v", err)
			}
			if _, err := os.Lstat(releases + "/2.0.0"); !os.IsNotExist(err) {
				t.Fatalf("target created before link preflight: %v", err)
			}
		})
	}
}

func TestReleaseInstallKeepsForeignReleaseEntries(t *testing.T) {
	t.Parallel()
	var env process.Environment
	op, _, releases := releaseFixture(t, &env)
	publishReleaseFixture(t, &env, "2.0.0", testutil.ArchiveMember{Name: "VERSION", Body: "2.0.0\n", Mode: 0644}, testutil.ArchiveMember{Name: "bin/selfishell", Body: "#!/bin/sh\n", Mode: 0755})
	op.Process.Inherit = env
	writeTestFile(t, op.Root+"/VERSION", "wrong\n", 0600)
	if _, err := op.Install(context.Background(), "2.0.0"); err == nil {
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
	if _, err := op.Install(context.Background(), "2.0.0"); err != nil {
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
	t.Parallel()
	var env process.Environment
	op, share, _ := releaseFixture(t, &env)
	publishReleaseFixture(t, &env, "2.0.0", testutil.ArchiveMember{Name: "VERSION", Body: "2.0.0\n", Mode: 0644}, testutil.ArchiveMember{Name: "bin/selfishell", Body: "#!/bin/sh\n", Mode: 0755})
	op.Process.Inherit = env
	if err := os.Symlink("releases/0.9.0", share+"/previous"); err != nil {
		t.Fatal(err)
	}
	op.link = func(target, path string) error {
		if target == "releases/2.0.0" {
			return fmt.Errorf("activation failed")
		}
		return atomicReleaseLink(target, path)
	}
	if _, err := op.Install(context.Background(), "2.0.0"); err == nil {
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
	t.Parallel()
	var env process.Environment
	op, share, releases := releaseFixture(t, &env)
	publishReleaseFixture(t, &env, "2.0.0", testutil.ArchiveMember{Name: "VERSION", Body: "2.0.0\n", Mode: 0644}, testutil.ArchiveMember{Name: "bin/selfishell", Body: "#!/bin/sh\n", Mode: 0755})
	op.Process.Inherit = env
	target := releases + "/2.0.0"
	os.MkdirAll(target+"/bin", 0700)
	testutil.WriteFile(target+"/bin/selfishell", []byte("not executable"), 0600)
	if _, err := op.Install(context.Background(), "2.0.0"); err == nil {
		t.Fatal("accepted incomplete existing release")
	}
	os.RemoveAll(target)
	os.Symlink(releases+"/1.0.0", target)
	if _, err := op.Install(context.Background(), "2.0.0"); err == nil {
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
	testutil.WriteFile(target+"/VERSION", []byte("9.9.9\n"), 0600)
	testutil.WriteFile(target+"/bin/selfishell", []byte("#!/bin/sh\n"), 0755)
	if _, err := op.Install(context.Background(), "2.0.0"); err == nil {
		t.Fatal("accepted wrong VERSION in existing release")
	}
}

func TestReleaseInstallSameVersionPreservesPreviousLink(t *testing.T) {
	t.Parallel()
	var env process.Environment
	op, share, _ := releaseFixture(t, &env)
	publishReleaseFixture(t, &env, "1.0.0", testutil.ArchiveMember{Name: "VERSION", Body: "1.0.0\n", Mode: 0644}, testutil.ArchiveMember{Name: "bin/selfishell", Body: "#!/bin/sh\n", Mode: 0755})
	op.Process.Inherit = env
	os.Symlink("releases/0.9.0", share+"/previous")
	if _, err := op.Install(context.Background(), "1.0.0"); err != nil {
		t.Fatal(err)
	}
	previous, _ := os.Readlink(share + "/previous")
	if previous != "releases/0.9.0" {
		t.Fatal(previous)
	}
}

func TestReleaseInstallPreflightsWholeArchiveBeforeFilesystemMutation(t *testing.T) {
	t.Parallel()
	var env process.Environment
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprintf("existing=%t", existing), func(t *testing.T) {
			t.Parallel()
			env := env
			op, share, releases := releaseFixture(t, &env)
			publishReleaseFixture(t, &env, "2.0.0", testutil.ArchiveMember{Name: "VERSION", Body: "2.0.0\n", Mode: 0644}, testutil.ArchiveMember{Name: "bin/selfishell", Body: "#!/bin/sh\n", Mode: 0755}, testutil.ArchiveMember{Name: "../sentinel", Body: "overwrite", Mode: 0600})
			if existing {
				writeTestFile(t, releases+"/2.0.0/VERSION", "2.0.0\n", 0600)
				writeTestFile(t, releases+"/2.0.0/bin/selfishell", "winner binary\n", 0755)
			}
			downloads := t.TempDir()
			setEnv(&env, "TMPDIR", downloads)
			op.Process.Inherit = env
			sentinel := filepath.Dir(share) + "/sentinel"
			testutil.WriteFile(sentinel, []byte("original\n"), 0600)
			os.Symlink("releases/0.9.0", share+"/previous")
			if _, err := op.Install(context.Background(), "2.0.0"); err == nil {
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

func TestReleaseFetchUsesMetadataDeadlineOnlyForMetadata(t *testing.T) {
	t.Parallel()
	var env process.Environment
	home := t.TempDir()
	setEnv(&env, "HOME", home)
	setEnv(&env, "SELFISHELL_RELEASE_ROOT", "file://"+home+"/remote")
	setEnv(&env, "SELFISHELL_CURL_METADATA_MAX_TIME", "7")
	fake := home + "/bin"
	os.Mkdir(fake, 0700)
	log := home + "/curl-args"
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" >\"$CURL_LOG\"\nwhile [ \"$#\" -gt 0 ]; do\n  if [ \"$1\" = '-o' ]; then shift; printf '1.2.3\\n' >\"$1\"; fi\n  shift\ndone\n"
	testutil.WriteFile(fake+"/curl", []byte(script), 0755)
	op := Operation{Process: process.Process{Env: append(env.List(), "PATH="+fake+":/usr/bin:/bin", "CURL_LOG="+log)}}
	if got, err := op.Latest(context.Background()); err != nil || got != "1.2.3" {
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
