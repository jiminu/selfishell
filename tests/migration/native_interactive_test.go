package migration_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func interactiveSource() string {
	return filepath.Join(repoRoot(), "config/shared/zsh/interactive.zsh")
}
func interactiveRun(t *testing.T, home, code string, env ...string) capture {
	t.Helper()
	return nativeRun(t, home, `_selfishell_command_path() { command -v "$1"; }; source "$SELFISHELL_SOURCE"; `+code, append([]string{"PATH=" + interactiveBin(t, home), "SELFISHELL_SOURCE=" + interactiveSource(), "SELFISHELL_COMMON_DIR=" + filepath.Join(repoRoot(), "config/shared/zsh")}, env...)...)
}
func interactiveCache(t *testing.T, home string) string {
	t.Helper()
	dir := filepath.Join(home, ".cache/selfishell")
	mustFS(t, os.MkdirAll(dir, 0700))
	return dir
}
func cacheEqual(t *testing.T, path, want string) {
	t.Helper()
	if got := nativeRead(t, path); got != want {
		t.Fatalf("%s: got %q want %q", path, got, want)
	}
}
func fakeExecutable(t *testing.T, home, name, body string) string {
	t.Helper()
	bin := interactiveBin(t, home)
	p := filepath.Join(bin, name)
	if info, err := os.Lstat(p); err == nil && info.Mode()&os.ModeSymlink != 0 {
		mustFS(t, os.Remove(p))
	} else if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	nativeWrite(t, p, "#!/bin/sh\n"+body, 0700)
	return bin
}

func interactiveBin(t *testing.T, home string) string {
	t.Helper()
	bin := filepath.Join(home, "bin")
	mustFS(t, os.MkdirAll(bin, 0700))
	for _, name := range []string{"mkdir", "rm", "mv", "zsh"} {
		path := filepath.Join(bin, name)
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			mustFS(t, os.Symlink(filepath.Join("/bin", name), path))
		} else if err != nil {
			t.Fatal(err)
		}
	}
	return bin
}

func TestNativeShellToolCacheGeneration(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		home := nativeHome(t)
		dir := interactiveCache(t, home)
		r := interactiveRun(t, home, `_selfishell_generate_zsh_cache "$SELFISHELL_TEST_CACHE" echo "print ok" || exit 10`, "SELFISHELL_TEST_CACHE="+filepath.Join(dir, "cache.zsh"))
		nativeQuiet(t, r)
		cacheEqual(t, filepath.Join(dir, "cache.zsh"), "\nprint ok\n")
		nativeAssertNoTemp(t, dir)
	})
	for _, tc := range []struct{ name, fn string }{{"nonzero-exit", `fake_tool() { print partial; return 1; }`}, {"empty-output", `fake_tool() { :; }`}, {"invalid-syntax", `fake_tool() { print 'if [[ not valid zsh'; }`}} {
		t.Run(tc.name, func(t *testing.T) {
			home := nativeHome(t)
			dir := interactiveCache(t, home)
			p := filepath.Join(dir, "cache.zsh")
			nativeWrite(t, p, "# preexisting cache\n", 0600)
			r := interactiveRun(t, home, tc.fn+`; _selfishell_generate_zsh_cache "$SELFISHELL_TEST_CACHE" fake_tool && exit 10; exit 0`, "SELFISHELL_TEST_CACHE="+p)
			nativeQuiet(t, r)
			cacheEqual(t, p, "# preexisting cache\n")
			nativeAssertNoTemp(t, dir)
		})
	}
	t.Run("newer-binary", func(t *testing.T) {
		home := nativeHome(t)
		dir := interactiveCache(t, home)
		p := filepath.Join(dir, "zoxide-init.zsh")
		nativeWrite(t, p, "# stale cache\n", 0600)
		nativeOldTime(t, p)
		bin := fakeExecutable(t, home, "zoxide", "printf 'print regenerated\\n'\n")
		r := interactiveRun(t, home, "", "PATH="+bin)
		if string(r.Stdout) != "regenerated\n" {
			t.Fatalf("output %q", r.Stdout)
		}
		if !strings.Contains(nativeRead(t, p), "print regenerated") {
			t.Fatal("stale cache was not replaced")
		}
	})
}
func TestNativeStarshipInitOnce(t *testing.T) {
	home := nativeHome(t)
	bin := fakeExecutable(t, home, "starship", "printf 'prompt_starship_precmd() { :; }\\n(( ++starship_inits ))\\n'\n")
	r := nativeRun(t, home, `_selfishell_command_path() { command -v "$1"; }; typeset -gi starship_inits=0; source "$SELFISHELL_SOURCE"; source "$SELFISHELL_SOURCE"; print -r -- "$starship_inits"`, "SELFISHELL_SOURCE="+interactiveSource(), "SELFISHELL_COMMON_DIR="+filepath.Join(repoRoot(), "config/shared/zsh"), "PATH="+bin)
	if string(r.Stdout) != "1\n" {
		t.Fatalf("inits: %q", r.Stdout)
	}
}
func TestNativeAutosuggestionsOrderAndPin(t *testing.T) {
	home := nativeHome(t)
	r := nativeRun(t, home, `_selfishell_command_path() { command -v "$1"; }; _selfishell_zinit_plugin_ready() { return 0; }; zinit() { [[ "$1" == ice ]] && print -r -- "ice: ${(j: :)@[2,-1]}"; [[ "$1" == light ]] && print -r -- "light: $2"; }; source "$SELFISHELL_SOURCE"; print -r -- "manual=$ZSH_AUTOSUGGEST_MANUAL_REBIND"`, "SELFISHELL_SOURCE="+interactiveSource(), "SELFISHELL_COMMON_DIR="+filepath.Join(repoRoot(), "config/shared/zsh"))
	out := string(r.Stdout)
	for _, s := range []string{"manual=1", "ver4672ad5dd9ad68a7effc1476d65afb7c584ce2b3 atload", "|| _zsh_autosuggest_bind_widgets"} {
		if !strings.Contains(out, s) {
			t.Fatalf("missing %q: %q", s, out)
		}
	}
	a := strings.Index(out, "light: zsh-users/zsh-autosuggestions\n")
	b := strings.Index(out, "light: zdharma-continuum/fast-syntax-highlighting")
	if a < 0 || b < a {
		t.Fatalf("light order: %q", out)
	}
}
func TestNativeShellToolCacheReplacement(t *testing.T) {
	for _, tool := range []string{"fzf", "zoxide", "starship"} {
		for _, replacement := range []string{"preserved-mtime", "older-mtime", "symlink"} {
			t.Run(tool+"/"+replacement, func(t *testing.T) {
				home := nativeHome(t)
				bin := filepath.Join(home, "bin")
				args := "init zsh"
				if tool == "fzf" {
					args = "--zsh"
				}
				body := `[ "$*" = "$SELFISHELL_TEST_INIT_ARGS" ] || exit 1
printf 'called\n' >>"$HOME/generations"
printf 'print old\n'
`
				fakeExecutable(t, home, tool, body)
				p := filepath.Join(bin, tool)
				old := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
				mustFS(t, os.Chtimes(p, old, old))
				run := func(want string, count int) {
					t.Helper()
					r := interactiveRun(t, home, "", "PATH="+bin, "SELFISHELL_TEST_INIT_ARGS="+args)
					if got := string(r.Stdout); got != want+"\n" {
						t.Fatalf("output %q want %s", got, want)
					}
					if got := strings.Count(nativeRead(t, filepath.Join(home, "generations")), "called\n"); got != count {
						t.Fatalf("generations %d want %d", got, count)
					}
				}
				run("old", 1)
				run("old", 1)
				replacementPath := filepath.Join(bin, "replacement")
				nativeWrite(t, replacementPath, "#!/bin/sh\n"+strings.Replace(body, "print old", "print new", 1), 0700)
				mustFS(t, os.Chtimes(replacementPath, old, old))
				if replacement == "older-mtime" {
					mustFS(t, os.Chtimes(replacementPath, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)))
				}
				if replacement == "symlink" {
					mustFS(t, os.Remove(p))
					mustFS(t, os.Symlink(replacementPath, p))
				} else {
					mustFS(t, os.Rename(replacementPath, p))
				}
				run("new", 2)
				run("new", 2)
			})
		}
	}
}
func TestNativeShellToolCacheFailureCleanup(t *testing.T) {
	for _, kind := range []string{"write", "mv"} {
		t.Run(kind, func(t *testing.T) {
			home := nativeHome(t)
			dir := interactiveCache(t, home)
			p := filepath.Join(dir, "cache.zsh")
			nativeWrite(t, p, "# preexisting cache\n", 0600)
			env := []string{"SELFISHELL_TEST_CACHE=" + p}
			if kind == "write" {
				if os.Geteuid() == 0 {
					t.Skip("root bypasses cache directory permissions")
				}
				mustFS(t, os.Chmod(dir, 0555))
				t.Cleanup(func() { os.Chmod(dir, 0700) })
			} else {
				bin := fakeExecutable(t, home, "mv", `printf 'mv-called\n' >>"$HOME/mv-calls"
exit 1
`)
				env = append(env, "PATH="+bin)
			}
			r := interactiveRun(t, home, `_selfishell_generate_zsh_cache "$SELFISHELL_TEST_CACHE" echo "print ok" && exit 10; exit 0`, env...)
			if kind == "write" && !strings.Contains(string(r.Stderr), "permission denied") {
				t.Fatalf("cache write failure not reached: stderr=%q", r.Stderr)
			}
			if kind == "mv" {
				nativeQuiet(t, r)
				if !strings.Contains(nativeRead(t, filepath.Join(home, "mv-calls")), "mv-called\n") {
					t.Fatal("final mv not reached")
				}
			}
			cacheEqual(t, p, "# preexisting cache\n")
			nativeAssertNoTemp(t, dir)
		})
	}
}
func TestNativeFzfCacheGeneration(t *testing.T) {
	for _, kind := range []string{"success", "invalid-syntax", "mv-failure"} {
		t.Run(kind, func(t *testing.T) {
			home := nativeHome(t)
			dir := interactiveCache(t, home)
			p := filepath.Join(dir, "cache.zsh")
			bin := fakeExecutable(t, home, "fzf", `printf 'bindkey -M emacs "^R" fzf-history-widget\n'`)
			if kind == "invalid-syntax" {
				nativeWrite(t, filepath.Join(bin, "fzf"), "#!/bin/sh\nprintf 'if [[ not valid zsh\\n'\n", 0700)
			}
			if kind == "mv-failure" {
				fakeExecutable(t, home, "mv", `printf 'mv-called\n' >>"$HOME/mv-calls"
exit 1`)
			}
			if kind != "success" {
				nativeWrite(t, p, "# preexisting fzf cache\n", 0600)
			}
			r := interactiveRun(t, home, `if _selfishell_generate_fzf_cache "$SELFISHELL_TEST_CACHE"; then [[ "$SELFISHELL_TEST_KIND" == success ]] || exit 10; else [[ "$SELFISHELL_TEST_KIND" != success ]] || exit 11; fi`, "SELFISHELL_TEST_KIND="+kind, "PATH="+bin, "SELFISHELL_TEST_CACHE="+p)
			nativeQuiet(t, r)
			if kind == "success" {
				if !strings.Contains(nativeRead(t, p), "fzf-history-widget") {
					t.Fatal("missing binding")
				}
			} else {
				cacheEqual(t, p, "# preexisting fzf cache\n")
			}
			nativeAssertNoTemp(t, dir)
			if kind == "mv-failure" {
				if !strings.Contains(nativeRead(t, filepath.Join(home, "mv-calls")), "mv-called\n") {
					t.Fatal("final mv not reached")
				}
			}
		})
	}
}
func TestNativeFzfFallbackCopy(t *testing.T) {
	if _, err := os.Stat("/usr/share/doc/fzf/examples/key-bindings.zsh"); err != nil {
		t.Skip("system fzf key bindings absent")
	}
	home := nativeHome(t)
	dir := interactiveCache(t, home)
	bin := filepath.Join(home, "bin")
	mustFS(t, os.MkdirAll(bin, 0700))
	for _, name := range []string{"mkdir", "rm", "mv", "zsh", "cp"} {
		path, err := exec.LookPath(name)
		if err != nil {
			t.Fatal(err)
		}
		mustFS(t, os.Symlink(path, filepath.Join(bin, name)))
	}
	p := filepath.Join(dir, "fallback-cache.zsh")
	r := interactiveRun(t, home, `_selfishell_generate_fzf_cache "$SELFISHELL_TEST_CACHE" || exit 10`, "PATH="+bin, "SELFISHELL_TEST_CACHE="+p)
	nativeQuiet(t, r)
	if len(nativeRead(t, p)) == 0 {
		t.Fatal("empty fallback")
	}
	nativeAssertNoTemp(t, dir)
	mustFS(t, os.Remove(filepath.Join(bin, "cp")))
	fakeExecutable(t, home, "cp", `printf 'cp-called\n' >>"$HOME/cp-calls"
exit 1`)
	nativeWrite(t, p, "# preexisting fallback cache\n", 0600)
	r = interactiveRun(t, home, `_selfishell_generate_fzf_cache "$SELFISHELL_TEST_CACHE" && exit 10; exit 0`, "PATH="+bin, "SELFISHELL_TEST_CACHE="+p)
	nativeQuiet(t, r)
	cacheEqual(t, filepath.Join(home, "cp-calls"), "cp-called\n")
	cacheEqual(t, p, "# preexisting fallback cache\n")
	nativeAssertNoTemp(t, dir)
}
func TestNativeInteractiveAliases(t *testing.T) {
	home := nativeHome(t)
	bin := fakeExecutable(t, home, "eza", "exit 0")
	fakeExecutable(t, home, "nvim", "exit 0")
	r := interactiveRun(t, home, `print -rl -- "ls=${aliases[ls]}" "vim=${aliases[vim]}"; for name in tf k kg kd g; do (( ${+aliases[$name]} )) && exit 10; done; exit 0`, "PATH="+bin)
	if got := string(r.Stdout); got != "ls=eza --group-directories-first\nvim=nvim\n" {
		t.Fatalf("aliases %q", got)
	}
}
func TestNativeKubectlCanonicalCompletion(t *testing.T) {
	home := nativeHome(t)
	bin := fakeExecutable(t, home, "kubectl", `[ "$*" = "completion zsh" ] || exit 1
printf '_kubectl() { return 0; }\n'`)
	r := nativeRun(t, home, `_selfishell_command_path() { command -v "$1"; }; source "$SELFISHELL_SOURCE"; print -rl -- "before=${_comps[kubectl]}" "k=${+_comps[k]}"; _selfishell_kubectl_completion || exit 10; print -rl -- "after=${_comps[kubectl]}" "k=${+_comps[k]}"`, "PATH="+bin, "SELFISHELL_SOURCE="+nativeCompletion())
	if got := string(r.Stdout); got != "before=_selfishell_kubectl_completion\nk=0\nafter=_kubectl\nk=0\n" {
		t.Fatalf("completion %q stderr %q", got, r.Stderr)
	}
}
func TestNativeEditorAliasesAndExports(t *testing.T) {
	home := nativeHome(t)
	bin := fakeExecutable(t, home, "nvim", "exit 0")
	source := filepath.Join(repoRoot(), "config/shared/zsh/aliases.zsh")
	r := nativeRun(t, home, `_selfishell_command_path() { command -v "$1"; }; unset EDITOR VISUAL; source "$SELFISHELL_SOURCE"; print -rl -- "alias=${aliases[vim]}" "first=$EDITOR|$VISUAL|${(t)EDITOR}|${(t)VISUAL}"; EDITOR='code --wait'; unset VISUAL; source "$SELFISHELL_SOURCE"; print -r -- "second=$EDITOR|$VISUAL"; unset EDITOR; VISUAL='emacsclient -c'; source "$SELFISHELL_SOURCE"; print -r -- "third=$EDITOR|$VISUAL"; EDITOR=nano; source "$SELFISHELL_SOURCE"; print -r -- "fourth=$EDITOR|$VISUAL"`, "PATH="+bin, "SELFISHELL_SOURCE="+source)
	want := "alias=nvim\nfirst=nvim|nvim|scalar-export|scalar-export\nsecond=code --wait|code --wait\nthird=nvim|emacsclient -c\nfourth=nano|emacsclient -c\n"
	if string(r.Stdout) != want {
		t.Fatalf("editor: %q want %q", r.Stdout, want)
	}
}
func TestNativeMissingNeovimAndGDS(t *testing.T) {
	home := nativeHome(t)
	empty := filepath.Join(home, "empty-bin")
	mustFS(t, os.MkdirAll(empty, 0700))
	source := filepath.Join(repoRoot(), "config/shared/zsh/aliases.zsh")
	r := nativeRun(t, home, `_selfishell_command_path() { command -v "$1"; }; unset EDITOR VISUAL; source "$SELFISHELL_SOURCE"; print -rl -- "vim=${+aliases[vim]}" "editor=${+EDITOR}" "visual=${+VISUAL}" "gds=${aliases[gds]}"`, "PATH="+empty, "SELFISHELL_SOURCE="+source)
	want := "vim=0\neditor=0\nvisual=0\ngds=git diff --staged\n"
	if string(r.Stdout) != want {
		t.Fatalf("aliases: %q", r.Stdout)
	}
}
func TestNativeZshPluginPins(t *testing.T) {
	data := nativeRead(t, filepath.Join(repoRoot(), "dependencies.conf"))
	for _, tc := range []struct{ repo, file string }{{"zsh-users/zsh-completions", "completion.zsh"}, {"Aloxaf/fzf-tab", "interactive.zsh"}, {"zsh-users/zsh-autosuggestions", "interactive.zsh"}, {"zdharma-continuum/fast-syntax-highlighting", "interactive.zsh"}} {
		t.Run(tc.repo, func(t *testing.T) {
			var pin string
			for _, line := range strings.Split(data, "\n") {
				f := strings.Fields(line)
				if len(f) >= 3 && f[0] == "zsh-plugin" && f[1] == tc.repo {
					pin = f[2]
				}
			}
			if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(pin) {
				t.Fatalf("missing approved pin %q", pin)
			}
			file := nativeRead(t, filepath.Join(repoRoot(), "config/shared/zsh", tc.file))
			if !strings.Contains(file, "ver'"+pin+"'") {
				t.Fatalf("%s lacks %s pin", tc.file, tc.repo)
			}
		})
	}
}
func TestNativeFzfPaletteAndUserOptions(t *testing.T) {
	for _, userOpts := range []string{"", "--with-nth=2.. --bind=ctrl-a:select-all"} {
		t.Run(fmt.Sprintf("opts=%q", userOpts), func(t *testing.T) {
			home := nativeHome(t)
			fakeExecutable(t, home, "fzf", "printf ':\\n'")
			nativeWrite(t, filepath.Join(home, ".local/share/zinit/zinit.git/zinit.zsh"), `typeset -gA ZINIT
ZINIT[PLUGINS_DIR]="$HOME/.local/share/zinit/plugins"
zinit() { :; }
`, 0600)
			mustFS(t, os.MkdirAll(filepath.Join(home, ".local/share/zinit/plugins/Aloxaf---fzf-tab/.git"), 0700))
			r := nativeRun(t, home, `source "$SELFISHELL_SOURCE"; local -a flags; zstyle -a ':fzf-tab:complete:cd:x' fzf-flags flags || exit 10; print -rl -- "opts=$FZF_DEFAULT_OPTS" "type=${(t)FZF_DEFAULT_OPTS}" "cd=${(j: :)flags}"; zstyle -a ':fzf-tab:complete:kill:argument-rest' fzf-flags flags || exit 11; print -r -- "kill=${(j: :)flags}"; local follows; zstyle -s ':fzf-tab:x' use-fzf-default-opts follows && exit 12; print -r -- 'follows=unset'`, "PATH="+filepath.Join(home, "bin")+":"+nativePath, "SELFISHELL_SOURCE="+nativeCommon(), "FZF_DEFAULT_OPTS="+userOpts)
			out := string(r.Stdout)
			want := userOpts
			if want == "" {
				want = "--color=16"
			}
			if !strings.Contains(out, "opts="+want+"\n") || !strings.Contains(out, "type=scalar-export\n") || !strings.Contains(out, "cd=--color=16\n") || !strings.Contains(out, "kill=--color=16 --preview-window=down:4:wrap\n") || !strings.Contains(out, "follows=unset\n") {
				t.Fatalf("fzf settings: %q", out)
			}
			if strings.Contains(out, "cd=--with-nth") || strings.Contains(out, "cd=--bind") {
				t.Fatalf("user opts reached fzf-tab: %q", out)
			}
		})
	}
}
