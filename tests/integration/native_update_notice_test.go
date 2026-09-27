package integration_test

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func nativeNotice() string { return filepath.Join(repoRoot(), "config/shared/zsh/update-notice.zsh") }
func nativeNoticeCLI(t *testing.T, home string) (string, string) {
	t.Helper()
	bin := filepath.Join(filepath.Dir(home), "bin")
	cache := filepath.Join(home, ".cache/selfishell")
	mustFS(t, os.MkdirAll(cache, 0700))
	nativeWrite(t, filepath.Join(bin, "selfishell"), `#!/bin/sh
[ "$1" = version ] || exit 1
if [ "${2:-}" = --available ]; then printf '1.1.0\n'; else printf 'selfishell 0.2.0\n'; fi
`, 0700)
	return bin, cache
}
func nativeNoticeRun(t *testing.T, home, code string, env ...string) capture {
	t.Helper()
	return nativeRun(t, home, code, append([]string{"SELFISHELL_SOURCE=" + nativeNotice()}, env...)...)
}
func nativeRead(t *testing.T, path string) string {
	t.Helper()
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}
func nativeAbsent(t *testing.T, path string) {
	t.Helper()
	if _, e := os.Lstat(path); !os.IsNotExist(e) {
		t.Fatalf("expected absent %s: %v", path, e)
	}
}
func TestNativeNoticeReadsInstalledVersionFile(t *testing.T) {
	home := nativeHome(t)
	release := filepath.Join(filepath.Dir(home), "releases/1.2.3")
	nativeWrite(t, filepath.Join(release, "VERSION"), "1.2.3\n", 0600)
	nativeWrite(t, filepath.Join(release, "bin/selfishell"), "#!/bin/sh\nprintf 'selfishell 9.9.9\\n'\n", 0700)
	r := nativeRun(t, home, `source "$SELFISHELL_SOURCE"; _selfishell_current_version`, "PATH="+filepath.Join(release, "bin")+":"+nativePath, "SELFISHELL_SOURCE="+nativeCommon())
	if string(r.Stdout) != "1.2.3\n" || len(r.Stderr) != 0 {
		t.Fatalf("installed VERSION ignored: %+v", r)
	}
}
func TestNativeNoticeDefersCurrentVersionLookup(t *testing.T) {
	home := nativeHome(t)
	bin, cache := nativeNoticeCLI(t, home)
	root := filepath.Dir(home)
	current := filepath.Join(root, "current-calls")
	refresh := filepath.Join(root, "refresh-calls")
	release := filepath.Join(root, "refresh-release")
	code := `_selfishell_command_path() { command -v "$1"; }
source "$SELFISHELL_SOURCE"
_selfishell_current_version() { print -r -- called >>"$SELFISHELL_CURRENT_CALLS"; print -r -- 1.0.0; }
_selfishell_update_notice_refresh() {
  # Expose the empty file until the observer acknowledges it.
  : >"$SELFISHELL_REFRESH_CALLS"
  while [[ ! -e "$SELFISHELL_REFRESH_RELEASE" ]]; do command sleep 0.05; done
  print -r -- scheduled >>"$SELFISHELL_REFRESH_CALLS"
}
SELFISHELL_UPDATE_CHECK_INTERVAL=0 _selfishell_update_notice
for attempt in {1..40}; do
  if [[ -r "$SELFISHELL_REFRESH_CALLS" ]]; then
    : >"$SELFISHELL_REFRESH_RELEASE"
    [[ "$(<"$SELFISHELL_REFRESH_CALLS")" == scheduled ]] && break
  fi
  command sleep 0.05
done
[[ -r "$SELFISHELL_REFRESH_CALLS" && "$(<"$SELFISHELL_REFRESH_CALLS")" == scheduled ]] || { print -u2 -r -- 'refresh notification not ready'; exit 1; }
print -r -- "stage1=${$(<"$SELFISHELL_REFRESH_CALLS"):-missing}"
print -r -- "current1=$([[ -e "$SELFISHELL_CURRENT_CALLS" ]] && print present || print absent)"
: >"$SELFISHELL_CACHE/available-version"
SELFISHELL_UPDATE_CHECK_INTERVAL=9999999999 _selfishell_update_notice
print -r -- "stage2=${$(<"$SELFISHELL_CACHE/available-version"):-empty}"
print -r -- "current2=$([[ -e "$SELFISHELL_CURRENT_CALLS" ]] && print present || print absent)"
print -r -- 1.1.0 >"$SELFISHELL_CACHE/available-version"
SELFISHELL_UPDATE_CHECK_INTERVAL=9999999999 _selfishell_update_notice
print -r -- "stage3=${$(<"$SELFISHELL_CACHE/available-version"):-missing}"
print -r -- "current3=$(command wc -l <"$SELFISHELL_CURRENT_CALLS" | command tr -d ' ')"
print -r -- 1.0.0 >"$SELFISHELL_CACHE/available-version"
SELFISHELL_UPDATE_CHECK_INTERVAL=9999999999 _selfishell_update_notice
print -r -- "stage4=$([[ -e "$SELFISHELL_CACHE/available-version" ]] && print present || print absent)"
print -r -- "current4=$(command wc -l <"$SELFISHELL_CURRENT_CALLS" | command tr -d ' ')"`
	r := nativeNoticeRun(t, home, code, "PATH="+bin+":"+nativePath, "SELFISHELL_CACHE="+cache, "SELFISHELL_CURRENT_CALLS="+current, "SELFISHELL_REFRESH_CALLS="+refresh, "SELFISHELL_REFRESH_RELEASE="+release)
	if string(r.Stdout) != "stage1=scheduled\ncurrent1=absent\nstage2=empty\ncurrent2=absent\nstage3=1.1.0\ncurrent3=1\nstage4=absent\ncurrent4=2\n" || string(r.Stderr) != "[Selfishell] 1.1.0 is available. Run: selfishell update\n" {
		t.Fatalf("deferred lookup: %+v", r)
	}
	if got := nativeRead(t, current); got != "called\ncalled\n" {
		t.Fatalf("current lookup calls: %q", got)
	}
	if got := nativeRead(t, refresh); got != "scheduled\n" {
		t.Fatalf("refresh calls: %q", got)
	}
	nativeAbsent(t, filepath.Join(cache, "available-version"))
}
func TestNativeNoticeSemanticVersionVectors(t *testing.T) {
	home := nativeHome(t)
	fixture := filepath.Join(repoRoot(), "tests/fixtures/version-precedence.txt")
	file, e := os.Open(fixture)
	if e != nil {
		t.Fatal(e)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	var input strings.Builder
	rows := 0
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 {
			t.Fatalf("fixture row %q", scanner.Text())
		}
		fmt.Fprintf(&input, "%s %s %s\n", fields[0], fields[1], fields[2])
		rows++
	}
	if e := scanner.Err(); e != nil {
		t.Fatal(e)
	}
	if rows != 21 {
		t.Fatalf("vector count: %d", rows)
	}
	r, e := runCommand(home, []string{nativeZsh, "-f", "-c", `source "$SELFISHELL_SOURCE"; while read -r candidate current expected; do actual=0; _selfishell_version_is_newer "$candidate" "$current" && actual=1; print -r -- "$actual"; done`, "zsh"}, []byte(input.String()), []string{"PATH=" + nativePath, "ZDOTDIR=", "SELFISHELL_SOURCE=" + nativeNotice()}, 10*time.Second)
	if e != nil || r.Status != 0 || len(r.Stderr) != 0 {
		t.Fatalf("comparison probe: %+v %v", r, e)
	}
	actual := strings.Fields(string(r.Stdout))
	expected := strings.Fields(input.String())
	if len(actual) != rows {
		t.Fatalf("comparison row count %d want %d", len(actual), rows)
	}
	for i := 0; i < rows; i++ {
		if actual[i] != expected[i*3+2] {
			t.Errorf("%s > %s: got %s want %s", expected[i*3], expected[i*3+1], actual[i], expected[i*3+2])
		}
	}
}
func TestNativeNoticeCacheAndRefresh(t *testing.T) {
	home := nativeHome(t)
	bin, cache := nativeNoticeCLI(t, home)
	nativeWrite(t, filepath.Join(cache, "available-version"), "1.1.0\n", 0600)
	nativeWrite(t, filepath.Join(cache, "update-checked-at"), strconv.FormatInt(time.Now().Unix(), 10)+"\n", 0600)
	r := nativeNoticeRun(t, home, `_selfishell_command_path() { command -v "$1"; }; source "$SELFISHELL_SOURCE"; _selfishell_update_notice`, "PATH="+bin+":"+nativePath)
	if len(r.Stdout) != 0 || string(r.Stderr) != "[Selfishell] 1.1.0 is available. Run: selfishell update\n" {
		t.Fatalf("cached notice: %+v", r)
	}
	r = nativeNoticeRun(t, home, `_selfishell_command_path() { command -v "$1"; }; source "$SELFISHELL_SOURCE"; SELFISHELL_UPDATE_NOTICE=0 _selfishell_update_notice`, "PATH="+bin+":"+nativePath)
	nativeQuiet(t, r)
	for _, name := range []string{"available-version", "update-checked-at"} {
		mustFS(t, os.Remove(filepath.Join(cache, name)))
	}
	nativeQuiet(t, nativeNoticeRun(t, home, `source "$SELFISHELL_SOURCE"; _selfishell_update_notice_refresh "$SELFISHELL_CACHE" 12345`, "PATH="+bin+":"+nativePath, "SELFISHELL_CACHE="+cache))
	if got := nativeRead(t, filepath.Join(cache, "available-version")); got != "1.1.0\n" {
		t.Fatalf("direct version: %q", got)
	}
	if got := nativeRead(t, filepath.Join(cache, "update-checked-at")); got != "12345\n" {
		t.Fatalf("direct time: %q", got)
	}
	for _, name := range []string{"available-version", "update-checked-at"} {
		mustFS(t, os.Remove(filepath.Join(cache, name)))
	}
	r = nativeNoticeRun(t, home, `_selfishell_command_path() { command -v "$1"; }; source "$SELFISHELL_SOURCE"; SELFISHELL_UPDATE_CHECK_INTERVAL=0 _selfishell_update_notice; for attempt in {1..40}; do [[ -r "$SELFISHELL_CACHE/available-version" && -r "$SELFISHELL_CACHE/update-checked-at" && ! -e "$SELFISHELL_CACHE/update-check.lock" ]] && break; command sleep 0.05; done`, "PATH="+bin+":"+nativePath, "SELFISHELL_CACHE="+cache)
	nativeQuiet(t, r)
	if got := nativeRead(t, filepath.Join(cache, "available-version")); got != "1.1.0\n" {
		t.Fatalf("background version: %q", got)
	}
	if got := nativeRead(t, filepath.Join(cache, "update-checked-at")); strings.TrimSpace(got) == "" {
		t.Fatal("empty background timestamp")
	}
	nativeAbsent(t, filepath.Join(cache, "update-check.lock"))
}
func TestNativeNoticeLockRecovery(t *testing.T) {
	rows := []struct{ meta, age, ttl, expected string }{
		{"timestamp", "-700", "600", "refreshed"}, {"timestamp", "0", "600", "held"}, {"absent", "stale", "600", "refreshed"}, {"absent", "fresh", "600", "held"}, {"pid", "stale", "600", "refreshed"}, {"corrupt", "stale", "600", "refreshed"}, {"corrupt", "fresh", "600", "held"}, {"zero", "stale", "600", "refreshed"}, {"zero", "fresh", "600", "held"}, {"unreadable", "stale", "600", "refreshed"}, {"timestamp", "100000", "600", "held"}, {"timestamp", "-700", "abc", "refreshed"}, {"timestamp", "-700", "-100", "refreshed"}, {"timestamp", "-700", "1.5", "refreshed"}, {"timestamp", "-700", "0", "refreshed"}, {"timestamp", "-700", "empty", "refreshed"}, {"timestamp", "-2", "0", "held"}, {"timestamp", "-5", "2", "refreshed"},
	}
	if len(rows) != 18 {
		t.Fatal("lock vector count")
	}
	for _, tc := range rows {
		t.Run(strings.Join([]string{tc.meta, tc.age, tc.ttl, tc.expected}, "_"), func(t *testing.T) {
			if tc.meta == "unreadable" && os.Geteuid() == 0 {
				t.Skip("root bypasses unreadable-file permission semantics")
			}
			home := nativeHome(t)
			bin, cache := nativeNoticeCLI(t, home)
			lock := filepath.Join(cache, "update-check.lock")
			mustFS(t, os.Mkdir(lock, 0700))
			now := time.Now().Unix()
			switch tc.meta {
			case "timestamp":
				delta, e := strconv.ParseInt(tc.age, 10, 64)
				if e != nil {
					t.Fatal(e)
				}
				nativeWrite(t, filepath.Join(lock, "created_at"), fmt.Sprintf("%d\n", now+delta), 0600)
			case "pid":
				nativeWrite(t, filepath.Join(lock, "pid"), "99999\n", 0600)
			case "corrupt":
				nativeWrite(t, filepath.Join(lock, "created_at"), "not-a-timestamp\n", 0600)
			case "zero":
				nativeWrite(t, filepath.Join(lock, "created_at"), "0\n", 0600)
			case "unreadable":
				nativeWrite(t, filepath.Join(lock, "created_at"), fmt.Sprintf("%d\n", now), 0000)
			}
			if tc.meta != "timestamp" && tc.age == "stale" {
				nativeOldTime(t, lock)
			}
			ttl := tc.ttl
			if ttl == "empty" {
				ttl = ""
			}
			nativeQuiet(t, nativeNoticeRun(t, home, `source "$SELFISHELL_SOURCE"; _selfishell_update_notice_refresh "$SELFISHELL_CACHE" 12345 || :`, "PATH="+bin+":"+nativePath, "SELFISHELL_CACHE="+cache, "SELFISHELL_UPDATE_LOCK_TTL="+ttl))
			if tc.expected == "refreshed" {
				nativeAbsent(t, lock)
				if got := nativeRead(t, filepath.Join(cache, "available-version")); got != "1.1.0\n" {
					t.Fatalf("version: %q", got)
				}
				if got := nativeRead(t, filepath.Join(cache, "update-checked-at")); got != "12345\n" {
					t.Fatalf("timestamp: %q", got)
				}
			} else {
				if info, e := os.Stat(lock); e != nil || !info.IsDir() {
					t.Fatalf("held lock: %v %v", info, e)
				}
				nativeAbsent(t, filepath.Join(cache, "available-version"))
				nativeAbsent(t, filepath.Join(cache, "update-checked-at"))
			}
		})
	}
}
func TestNativeNoticeFailedLookupClearsLock(t *testing.T) {
	home := nativeHome(t)
	cache := filepath.Join(home, ".cache/selfishell")
	mustFS(t, os.MkdirAll(cache, 0700))
	r := nativeNoticeRun(t, home, `source "$SELFISHELL_SOURCE"; _selfishell_update_notice_refresh "$SELFISHELL_CACHE" 12345; print -r -- "$([[ -e "$SELFISHELL_CACHE/update-check.lock" ]] && print LOCK_LEFT || print LOCK_CLEARED)"`, "SELFISHELL_CACHE="+cache)
	if string(r.Stdout) != "LOCK_CLEARED\n" {
		t.Fatalf("lookup failure: %+v", r)
	}
	nativeAbsent(t, filepath.Join(cache, "update-check.lock"))
}
func TestNativeNoticeUndatableLock(t *testing.T) {
	home := nativeHome(t)
	r := nativeNoticeRun(t, home, `source "$SELFISHELL_SOURCE"; if result="$(_selfishell_update_lock_stale_since "$SELFISHELL_LOCK" 600 99999999999)"; then print -r -- "DETERMINED:$result"; else print PRESERVED; fi`, "SELFISHELL_LOCK="+filepath.Join(home, "no-such-lock-dir"))
	if string(r.Stdout) != "PRESERVED\n" || len(r.Stderr) != 0 {
		t.Fatalf("undatable lock: %+v", r)
	}
}
func nativeAssertNoTemp(t *testing.T, cache string) {
	t.Helper()
	entries, e := os.ReadDir(cache)
	if e != nil {
		t.Fatal(e)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp.") {
			t.Fatalf("temporary file retained: %s", entry.Name())
		}
	}
}
func TestNativeNoticeWriteFailureCleansTemp(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses unwritable-directory permission semantics")
	}
	home := nativeHome(t)
	bin := filepath.Join(filepath.Dir(home), "bin")
	cache := filepath.Join(home, ".cache/selfishell")
	mustFS(t, os.MkdirAll(cache, 0700))
	lookup := filepath.Join(filepath.Dir(home), "available-lookup")
	cleanup := filepath.Join(filepath.Dir(home), "failed-write-cleanup")
	nativeWrite(t, filepath.Join(bin, "selfishell"), `#!/bin/sh
[ -d "$SELFISHELL_CACHE/update-check.lock" ] || exit 1
printf 'called\n' >"$SELFISHELL_LOOKUP_LOG"
/bin/chmod 0555 "$SELFISHELL_CACHE" || exit 1
printf '1.1.0\n'
`, 0700)
	nativeWrite(t, filepath.Join(bin, "rm"), `#!/bin/sh
if [ "$1" = -f ]; then
  case "$2" in
    "$SELFISHELL_CACHE"/available-version.tmp.*)
      printf 'called\n' >"$SELFISHELL_CLEANUP_LOG"
      /bin/chmod 0755 "$SELFISHELL_CACHE" || exit 1
      ;;
  esac
fi
exec /bin/rm "$@"
`, 0700)
	t.Cleanup(func() { os.Chmod(cache, 0755) })
	r := nativeNoticeRun(t, home, `source "$SELFISHELL_SOURCE"; _selfishell_update_notice_refresh "$SELFISHELL_CACHE" 12345 || :`, "PATH="+bin+":"+nativePath, "SELFISHELL_CACHE="+cache, "SELFISHELL_LOOKUP_LOG="+lookup, "SELFISHELL_CLEANUP_LOG="+cleanup)
	if len(r.Stdout) != 0 || !strings.Contains(string(r.Stderr), "permission denied") {
		t.Fatalf("temporary write failure was not observed: stdout=%q stderr=%q", r.Stdout, r.Stderr)
	}
	if got := nativeRead(t, lookup); got != "called\n" {
		t.Fatalf("available-version lookup: %q", got)
	}
	if got := nativeRead(t, cleanup); got != "called\n" {
		t.Fatalf("failed-write cleanup: %q", got)
	}
	nativeAbsent(t, filepath.Join(cache, "available-version"))
	if got := nativeRead(t, filepath.Join(cache, "update-checked-at")); got != "12345\n" {
		t.Fatalf("later checked-at write: %q", got)
	}
	nativeAbsent(t, filepath.Join(cache, "update-check.lock"))
	nativeAssertNoTemp(t, cache)
}
func TestNativeNoticeMoveFailureCleansTemp(t *testing.T) {
	home := nativeHome(t)
	bin, cache := nativeNoticeCLI(t, home)
	moves := filepath.Join(filepath.Dir(home), "move-calls")
	nativeWrite(t, filepath.Join(bin, "mv"), "#!/bin/sh\nprintf '%s\\n' \"$3\" >>\"$SELFISHELL_MOVE_LOG\"\nexit 1\n", 0700)
	nativeNoticeRun(t, home, `source "$SELFISHELL_SOURCE"; _selfishell_update_notice_refresh "$SELFISHELL_CACHE" 12345`, "PATH="+bin+":"+nativePath, "SELFISHELL_CACHE="+cache, "SELFISHELL_MOVE_LOG="+moves)
	if got, want := nativeRead(t, moves), filepath.Join(cache, "available-version")+"\n"+filepath.Join(cache, "update-checked-at")+"\n"; got != want {
		t.Fatalf("failed move calls: got %q want %q", got, want)
	}
	nativeAssertNoTemp(t, cache)
}
