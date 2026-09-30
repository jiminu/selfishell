package integration_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jiminu/selfishell/internal/testutil"
)

const nativeZsh = "/bin/zsh"
const nativePath = "/usr/bin:/bin"

func nativeHome(t *testing.T) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	mustFS(t, os.MkdirAll(home, 0700))
	return home
}
func nativeWrite(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	mustFS(t, os.MkdirAll(filepath.Dir(path), 0700))
	mustFS(t, testutil.WriteFile(path, []byte(body), mode))
}
func nativeRun(t *testing.T, home, code string, env ...string) capture {
	t.Helper()
	result, err := runCommand(home, []string{nativeZsh, "-f", "-c", code, "zsh"}, nil, append([]string{"PATH=" + nativePath, "MISE_DATA_DIR=" + filepath.Join(home, ".local/share/mise"), "ZDOTDIR=", "WSL_DISTRO_NAME="}, env...), 10*time.Second)
	if err != nil || result.Status != 0 {
		t.Fatalf("native zsh: %+v: %v", result, err)
	}
	return result
}
func nativeQuiet(t *testing.T, result capture) {
	t.Helper()
	if len(result.Stdout) != 0 || len(result.Stderr) != 0 {
		t.Fatalf("unexpected output: stdout=%q stderr=%q", result.Stdout, result.Stderr)
	}
}
func nativeCommon() string     { return filepath.Join(repoRoot(), "config/shared/zsh/common.zsh") }
func nativeCompletion() string { return filepath.Join(repoRoot(), "config/shared/zsh/completion.zsh") }

func TestNativeGitCompletionInitializesWithoutZinit(t *testing.T) {
	t.Parallel()
	home := nativeHome(t)
	r := nativeRun(t, home, `source "$SELFISHELL_SOURCE"; (( $+functions[_git] )) || exit 10; [[ -s "$HOME/.zcompdump" ]] || exit 11`, "SELFISHELL_SOURCE="+nativeCommon())
	nativeQuiet(t, r)
	data, err := os.ReadFile(filepath.Join(home, ".zcompdump"))
	if err != nil || len(data) == 0 {
		t.Fatalf("dump: %v %d bytes", err, len(data))
	}
}

// mise skips the first prompt's full hook-env only when PATH is unchanged
// since activation, so Zinit's $ZPFX/bin must be added before it.
func TestNativeZinitLoadsBeforeMiseActivation(t *testing.T) {
	t.Parallel()
	home := nativeHome(t)
	nativeWrite(t, filepath.Join(home, ".local/share/zinit/zinit.git/zinit.zsh"), `path=("$HOME/zpfx/bin" $path)
zinit() { return 0; }
`, 0600)
	bin := filepath.Join(filepath.Dir(home), "bin")
	nativeWrite(t, filepath.Join(bin, "mise"), `#!/bin/sh
[ "$*" = 'activate zsh' ] || exit 1
printf '%s\n' 'typeset -g SELFISHELL_TEST_ACTIVATION_PATH="$PATH"'
`, 0700)
	r := nativeRun(t, home, `source "$SELFISHELL_SOURCE"; [[ -n "$SELFISHELL_TEST_ACTIVATION_PATH" ]] || exit 10; (( ${path[(I)$HOME/zpfx/bin]} )) || exit 11; [[ "$PATH" == "$SELFISHELL_TEST_ACTIVATION_PATH" ]] || exit 12`,
		"PATH="+bin+":"+nativePath, "SELFISHELL_SOURCE="+nativeCommon())
	nativeQuiet(t, r)
}
func TestNativeZinitStartup(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		zinit    bool
		complete bool
		want     int
	}{
		{"missing_zinit", false, false, 0}, {"missing_plugins", true, false, 0}, {"preprovisioned", true, true, 4}, {"incomplete_checkout", true, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := nativeHome(t)
			root := filepath.Dir(home)
			log := filepath.Join(root, "zinit-calls")
			if tc.zinit {
				zinit := filepath.Join(home, ".local/share/zinit/zinit.git/zinit.zsh")
				nativeWrite(t, zinit, `typeset -gA ZINIT
ZINIT[PLUGINS_DIR]="${XDG_DATA_HOME:-$HOME/.local/share}/zinit/plugins"
zinit() {
  print -r -- "$*" >>"$SELFISHELL_TEST_ZINIT_LOG"
  if [[ "$1" == ice ]]; then
    SELFISHELL_TEST_BLOCK_FPATH=${@[(I)blockf]}
  elif [[ "$1" == light && "$2" == zsh-users/zsh-completions && "$SELFISHELL_TEST_BLOCK_FPATH" == 0 ]]; then
    source "$ZINIT[PLUGINS_DIR]/zsh-users---zsh-completions/zsh-completions.plugin.zsh"
  fi
  return 0
}
`, 0600)
				for _, p := range []string{"zsh-users---zsh-completions", "Aloxaf---fzf-tab", "zsh-users---zsh-autosuggestions", "zdharma-continuum---fast-syntax-highlighting"} {
					path := filepath.Join(home, ".local/share/zinit/plugins", p)
					if tc.complete {
						path = filepath.Join(path, ".git")
					}
					if tc.complete || tc.name == "incomplete_checkout" && p != "Aloxaf---fzf-tab" {
						mustFS(t, os.MkdirAll(path, 0700))
					}
				}
				if tc.complete {
					plugin := filepath.Join(home, ".local/share/zinit/plugins/zsh-users---zsh-completions")
					nativeWrite(t, filepath.Join(plugin, "zsh-completions.plugin.zsh"), `fpath+="${0:A:h}/src"`+"\n", 0600)
					nativeWrite(t, filepath.Join(plugin, "src/_selfishell_completion_probe"), "#compdef selfishell-completion-probe\n", 0600)
				}
			}
			fakeBin := filepath.Join(root, "bin")
			if tc.complete {
				nativeWrite(t, filepath.Join(fakeBin, "fzf"), "#!/bin/sh\nprintf ':\\n'\n", 0700)
			}
			path := nativePath
			if tc.complete {
				path = fakeBin + ":" + path
			}
			probe := `source "$SELFISHELL_SOURCE"`
			if tc.complete {
				probe += `; [[ "${_comps[selfishell-completion-probe]}" == _selfishell_completion_probe ]] || exit 20`
			}
			r := nativeRun(t, home, probe, "PATH="+path, "SELFISHELL_SOURCE="+nativeCommon(), "SELFISHELL_TEST_ZINIT_LOG="+log)
			nativeQuiet(t, r)
			data, err := os.ReadFile(log)
			if os.IsNotExist(err) {
				data = nil
				err = nil
			}
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
				if strings.HasPrefix(line, "light ") {
					count++
				}
			}
			if count != tc.want {
				t.Fatalf("light calls: got %d want %d; log=%q", count, tc.want, data)
			}
		})
	}
}

func nativeCopyCompletionFunctions(t *testing.T, home string) string {
	t.Helper()
	dir := filepath.Join(home, "completion-functions")
	mustFS(t, os.MkdirAll(dir, 0700))
	r := nativeRun(t, home, `for name in compinit compaudit compdump compinstall _git; do files=(${^fpath}/$name(N)); (( ${#files} )) || exit 12; command cp "$files[1]" "$SELFISHELL_FUNCTIONS/$name" || exit 13; done`, "SELFISHELL_FUNCTIONS="+dir)
	nativeQuiet(t, r)
	return dir
}
func nativeAuditCount(t *testing.T, home, functions string) int {
	t.Helper()
	r, err := runCommand(home, []string{nativeZsh, "-f", "-i", "-c", `fpath=("$SELFISHELL_FUNCTIONS"); _compdir=""; _selfishell_command_path() { return 1; }; zmodload zsh/zprof; source "$SELFISHELL_SOURCE"; zprof`, "zsh"}, nil, []string{"PATH=" + filepath.Join(home, "bin"), "ZDOTDIR=" + home, "SELFISHELL_FUNCTIONS=" + functions, "SELFISHELL_SOURCE=" + nativeCompletion()}, 10*time.Second)
	if err != nil || r.Status != 0 || len(r.Stderr) != 0 {
		t.Fatalf("audit probe: %+v %v", r, err)
	}
	return strings.Count(string(r.Stdout), "compaudit")
}
func nativeOldTime(t *testing.T, path string) {
	t.Helper()
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	mustFS(t, os.Chtimes(path, old, old))
}
func nativeMarker(t *testing.T, home, kind string) func() {
	t.Helper()
	root := filepath.Dir(home)
	marker := filepath.Join(home, ".zcompdump.audit")
	target := filepath.Join(root, "marker-target")
	missing := filepath.Join(root, "missing-target")
	mustFS(t, os.RemoveAll(marker))
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	switch kind {
	case "file":
		nativeWrite(t, marker, "personal data\n", 0600)
	case "symlink", "empty-symlink":
		body := "personal data\n"
		if kind == "empty-symlink" {
			body = ""
		}
		nativeWrite(t, target, body, 0600)
		mustFS(t, os.Symlink(target, marker))
		mustFS(t, os.Chtimes(target, old, old))
	case "dangling":
		mustFS(t, os.Symlink(missing, marker))
	case "directory":
		mustFS(t, os.Mkdir(marker, 0700))
		nativeWrite(t, filepath.Join(marker, "personal"), "personal data\n", 0600)
	}
	if kind == "file" || kind == "directory" {
		mustFS(t, os.Chtimes(marker, old, old))
	}
	before := mustSnapshot(t, marker)
	var targetBefore []byte
	if kind == "symlink" || kind == "empty-symlink" {
		targetBefore = mustSnapshot(t, target)
	}
	return func() {
		after := mustSnapshot(t, marker)
		if !bytes.Equal(before, after) {
			t.Fatalf("foreign %s marker changed: before %s after %s", kind, before, after)
		}
		if kind != "dangling" {
			p := marker
			if kind == "symlink" || kind == "empty-symlink" {
				p = target
			}
			info, err := os.Stat(p)
			if err != nil || !info.ModTime().Equal(old) {
				t.Fatalf("foreign %s mtime changed: %v %v", kind, info, err)
			}
			if targetBefore != nil && !bytes.Equal(targetBefore, mustSnapshot(t, target)) {
				t.Fatalf("foreign %s target changed", kind)
			}
		} else if _, err := os.Lstat(missing); !os.IsNotExist(err) {
			t.Fatalf("dangling marker target created: %v", err)
		}
	}
}
func TestNativeCompletionAudit(t *testing.T) {
	t.Parallel()
	home := nativeHome(t)
	functions := nativeCopyCompletionFunctions(t, home)
	bin := filepath.Join(home, "bin")
	mustFS(t, os.Mkdir(bin, 0700))
	for name, target := range map[string]string{"mv": "/bin/mv", "rm": "/bin/rm", "touch": "/usr/bin/touch"} {
		mustFS(t, os.Symlink(target, filepath.Join(bin, name)))
	}
	nativeAuditCount(t, home, functions)
	if got := nativeAuditCount(t, home, functions); got != 0 {
		t.Fatalf("fresh dump audited: %d", got)
	}
	for _, p := range []string{".zcompdump", ".zcompdump.audit"} {
		nativeOldTime(t, filepath.Join(home, p))
	}
	if got := nativeAuditCount(t, home, functions); got == 0 {
		t.Fatal("stale dump not audited")
	}
	if got := nativeAuditCount(t, home, functions); got != 0 {
		t.Fatalf("daily audit repeated: %d", got)
	}
	for _, p := range []string{".zcompdump", ".zcompdump.zwc"} {
		nativeOldTime(t, filepath.Join(home, p))
	}
	nativeWrite(t, filepath.Join(functions, "_zzselfishell"), "#compdef zzselfishell\n", 0600)
	if got := nativeAuditCount(t, home, functions); got == 0 {
		t.Fatal("new completion did not rebuild")
	}
	dump, err := os.ReadFile(filepath.Join(home, ".zcompdump"))
	if err != nil || !bytes.Contains(dump, []byte("_zzselfishell")) {
		t.Fatalf("rebuilt dump: %v %q", err, dump)
	}
	if got := nativeAuditCount(t, home, functions); got != 0 {
		t.Fatalf("unchanged completion rebuilt: %d", got)
	}
	for _, kind := range []string{"file", "symlink", "empty-symlink", "dangling", "directory"} {
		t.Run(kind, func(t *testing.T) {
			check := nativeMarker(t, home, kind)
			mustFS(t, os.Remove(filepath.Join(home, ".zcompdump")))
			nativeAuditCount(t, home, functions)
			check()
			if got := nativeAuditCount(t, home, functions); got == 0 {
				t.Fatal("foreign marker bypassed audit")
			}
			check()
		})
	}
}

func nativeCompletionProbe(t *testing.T, home, dir, mode, compile string) capture {
	t.Helper()
	if mode == "" {
		mode = "-i"
	}
	r, err := runCommand(home, []string{nativeZsh, "-f", mode, "-c", `[[ -z "$SELFISHELL_TEST_COMPLETION_DIR" ]] || fpath=("$SELFISHELL_TEST_COMPLETION_DIR" "${SELFISHELL_TEST_COMPLETION_DIR:h}/secure-completions" $fpath)
if [[ "$SELFISHELL_TEST_FAIL_COMPILE" == compile-failure ]]; then zcompile() { return 1; }; fi
source "$SELFISHELL_SOURCE"
(( ! ${+_comps[selfishell-insecure-probe]} )) || print INSECURE_REGISTERED
(( ! ${+_comps[selfishell-safe-probe]} )) || _selfishell_safe_probe
print STARTUP_COMPLETE`, "zsh"}, nil, []string{"PATH=" + nativePath, "ZDOTDIR=" + home, "SELFISHELL_SOURCE=" + nativeCommon(), "SELFISHELL_TEST_COMPLETION_DIR=" + dir, "SELFISHELL_TEST_FAIL_COMPILE=" + compile}, 10*time.Second)
	if err != nil || r.Status != 0 {
		t.Fatalf("completion startup: %+v %v", r, err)
	}
	return r
}
func nativeAssertCompletionProbe(t *testing.T, r capture, warn bool) {
	t.Helper()
	s := string(r.Stdout) + string(r.Stderr)
	for _, token := range []string{"STARTUP_COMPLETE", "SAFE_COMPLETION"} {
		if !strings.Contains(s, token) {
			t.Fatalf("missing %s: %+v", token, r)
		}
	}
	for _, token := range []string{"INSECURE_REGISTERED", "INSECURE_LOADED"} {
		if strings.Contains(s, token) {
			t.Fatalf("unsafe %s: %+v", token, r)
		}
	}
	if warn && !strings.Contains(s, "insecure completion directories detected") {
		t.Fatalf("missing insecure warning: %+v", r)
	}
}
func TestNativeInsecureCompletionDirectory(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"missing", "noninteractive", "removed", "compile-failure", "expired", "foreign-file", "foreign-symlink", "foreign-empty-symlink", "foreign-dangling", "foreign-directory"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			home := nativeHome(t)
			root := filepath.Dir(home)
			insecure := filepath.Join(root, "insecure-completions")
			secure := filepath.Join(root, "secure-completions")
			mustFS(t, os.MkdirAll(insecure, 0777))
			mustFS(t, os.Chmod(insecure, 0777))
			mustFS(t, os.Mkdir(secure, 0755))
			nativeWrite(t, filepath.Join(insecure, "_selfishell_insecure_probe"), "#compdef selfishell-insecure-probe\n", 0600)
			nativeWrite(t, filepath.Join(secure, "_selfishell_safe_probe"), "#compdef selfishell-safe-probe\nprint SAFE_COMPLETION\n", 0600)
			nativeWrite(t, filepath.Join(insecure, "_selfishell_safe_probe"), "#compdef selfishell-safe-probe\nprint INSECURE_LOADED\n", 0600)
			var check func()
			if strings.HasPrefix(scenario, "foreign-") {
				check = nativeMarker(t, home, strings.TrimPrefix(scenario, "foreign-"))
			}
			switch scenario {
			case "noninteractive":
				nativeCompletionProbe(t, home, insecure, "+i", "")
				for _, name := range []string{"_added_one", "_added_two"} {
					nativeWrite(t, filepath.Join(secure, name), "", 0600)
				}
			case "removed", "compile-failure":
				nativeCompletionProbe(t, home, insecure, "", "")
				nativeWrite(t, filepath.Join(home, ".zcompdump.audit"), "", 0600)
				mustFS(t, os.Remove(filepath.Join(home, ".zcompdump")))
			case "expired":
				nativeCompletionProbe(t, home, insecure, "", "")
				nativeWrite(t, filepath.Join(home, ".zcompdump.audit"), "", 0600)
				nativeOldTime(t, filepath.Join(home, ".zcompdump.audit"))
			}
			nativeAssertCompletionProbe(t, nativeCompletionProbe(t, home, insecure, "", scenario), true)
			nativeAssertCompletionProbe(t, nativeCompletionProbe(t, home, insecure, "", ""), false)
			if check != nil {
				check()
			}
		})
	}
}
func TestNativeSecureCompletionDirectory(t *testing.T) {
	t.Parallel()
	home := nativeHome(t)
	root := filepath.Dir(home)
	secure := filepath.Join(root, "secure-completions")
	mustFS(t, os.Mkdir(secure, 0755))
	for _, p := range []string{".zcompdump", ".zcompdump.audit"} {
		nativeWrite(t, filepath.Join(home, p), "", 0600)
		nativeOldTime(t, filepath.Join(home, p))
	}
	with := nativeCompletionProbe(t, home, secure, "", "")
	for _, p := range []string{".zcompdump", ".zcompdump.audit"} {
		path := filepath.Join(home, p)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			nativeWrite(t, path, "", 0600)
		}
		nativeOldTime(t, path)
	}
	without := nativeCompletionProbe(t, home, "", "", "")
	if !strings.Contains(string(with.Stdout), "STARTUP_COMPLETE") || !strings.Contains(string(without.Stdout), "STARTUP_COMPLETE") {
		t.Fatalf("startup did not complete: %+v %+v", with, without)
	}
	w := strings.Contains(string(with.Stderr), "insecure completion directories detected")
	b := strings.Contains(string(without.Stderr), "insecure completion directories detected")
	if w != b {
		t.Fatalf("secure directory changed warning: with=%+v without=%+v", with, without)
	}
}
func TestNativeMacOSPathPrefix(t *testing.T) {
	t.Parallel()
	home := nativeHome(t)
	root := filepath.Dir(home)
	bin := filepath.Join(root, "bin")
	nativeWrite(t, filepath.Join(bin, "brew"), "#!/bin/sh\nexit 0\n", 0700)
	nativeWrite(t, filepath.Join(home, ".config/selfishell/zsh/common.zsh"), ":\n", 0600)
	r := nativeRun(t, home, `source "$SELFISHELL_SOURCE"; print -l -r -- "$path[1]" "$path[2]"`, "PATH="+bin+":"+nativePath, "HOMEBREW_PREFIX=", "SELFISHELL_SOURCE="+filepath.Join(repoRoot(), "config/macos/zshrc"))
	if string(r.Stdout) != home+"/.local/bin\n"+home+"/.rd/bin\n" || len(r.Stderr) != 0 {
		t.Fatalf("path order: %+v", r)
	}
}

func TestNativeMacOSInitializesHomebrewAlreadyOnPath(t *testing.T) {
	t.Parallel()
	home := nativeHome(t)
	prefix := filepath.Join(filepath.Dir(home), "brew prefix")
	bin := filepath.Join(prefix, "bin")
	nativeWrite(t, filepath.Join(bin, "brew"), `#!/bin/sh
[ "$*" = 'shellenv zsh' ] || exit 1
printf 'export HOMEBREW_PREFIX="%s"\n' "$SELFISHELL_TEST_BREW_PREFIX"
printf 'path=("%s/bin" "%s/sbin" $path)\n' "$SELFISHELL_TEST_BREW_PREFIX" "$SELFISHELL_TEST_BREW_PREFIX"
`, 0700)
	nativeWrite(t, filepath.Join(bin, "git"), "#!/bin/sh\nexit 0\n", 0700)
	nativeWrite(t, filepath.Join(home, ".config/selfishell/zsh/common.zsh"), ":\n", 0600)
	r := nativeRun(t, home, `source "$SELFISHELL_SOURCE"; source "$SELFISHELL_SOURCE"; print -rl -- "$HOMEBREW_PREFIX" "$(command -v git)" "$path[1]" "$path[2]"; unique_path=("${(@u)path}"); (( ${#path} == ${#unique_path} )) || exit 21`,
		"PATH="+nativePath+":"+bin, "HOMEBREW_PREFIX=", "SELFISHELL_TEST_BREW_PREFIX="+prefix,
		"SELFISHELL_SOURCE="+filepath.Join(repoRoot(), "config/macos/zshrc"))
	want := prefix + "\n" + filepath.Join(bin, "git") + "\n" + home + "/.local/bin\n" + home + "/.rd/bin\n"
	if string(r.Stdout) != want || len(r.Stderr) != 0 {
		t.Fatalf("Homebrew initialization: want %q got %+v", want, r)
	}
}
func TestNativeWSLDeferredPath(t *testing.T) {
	t.Parallel()
	home := nativeHome(t)
	nativeWrite(t, filepath.Join(home, ".config/selfishell/zsh/common.zsh"), `[[ ${path[(I)/mnt/[a-zA-Z]/*]} -eq 0 ]] || return 1
SELFISHELL_TEST_INITIALIZED=1
`, 0600)
	r := nativeRun(t, home, `source "$SELFISHELL_SOURCE" || exit 15; print -l -r -- "$SELFISHELL_TEST_INITIALIZED" "${(j.:.)path}"`, "PATH=/usr/bin:/mnt/c/Windows:/bin", "WSL_DISTRO_NAME=Ubuntu-24.04", "SELFISHELL_SOURCE="+filepath.Join(repoRoot(), "config/ubuntu/zshrc"))
	want := "1\n" + home + "/.local/bin:" + home + "/.rd/bin:/usr/bin:/bin:/mnt/c/Windows\n"
	if string(r.Stdout) != want || len(r.Stderr) != 0 {
		t.Fatalf("WSL path: want %q got %+v", want, r)
	}
}
func TestNativeCommandLookupPathSemantics(t *testing.T) {
	t.Parallel()
	home := nativeHome(t)
	root := filepath.Dir(home)
	work := filepath.Join(root, "work")
	bin := filepath.Join(root, "bin")
	rel := filepath.Join(root, "relative-bin")
	for _, p := range []string{filepath.Join(work, "cwd-probe"), filepath.Join(bin, "path-probe"), filepath.Join(rel, "relative-probe")} {
		nativeWrite(t, p, "#!/bin/sh\nexit 0\n", 0700)
	}
	r := nativeRun(t, home, `source "$SELFISHELL_SOURCE"; cd "$SELFISHELL_WORK" || exit 14; path=("" ../relative-bin "$SELFISHELL_BIN" /usr/bin /bin); print -l -r -- "current=$(_selfishell_command_path cwd-probe)" "relative=$(_selfishell_command_path relative-probe)" "path=$(_selfishell_command_path path-probe)"; if _selfishell_command_path missing-probe >/dev/null; then print missing=found; else print missing=absent; fi`, "SELFISHELL_SOURCE="+nativeCommon(), "SELFISHELL_WORK="+work, "SELFISHELL_BIN="+bin)
	want := fmt.Sprintf("current=cwd-probe\nrelative=../relative-bin/relative-probe\npath=%s/path-probe\nmissing=absent\n", bin)
	if string(r.Stdout) != want || len(r.Stderr) != 0 {
		t.Fatalf("lookup: want %q got %+v", want, r)
	}
}
