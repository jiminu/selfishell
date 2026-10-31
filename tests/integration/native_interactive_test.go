package integration_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jiminu/selfishell/internal/pty"
)

func interactiveSource() string {
	return filepath.Join(repoRoot(), "config/shared/zsh/interactive.zsh")
}
func interactiveFzfSource(t *testing.T, home string) string {
	t.Helper()
	// Keep the system fallback fixture private, regardless of installed fzf.
	source := filepath.Join(home, "interactive.zsh")
	nativeWrite(t, source, strings.ReplaceAll(nativeRead(t, interactiveSource()), "/usr/share/doc/fzf/examples/key-bindings.zsh", `"$HOME/key-bindings.zsh"`), 0600)
	return source
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
	t.Parallel()
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
}
func TestNativeStarshipInitOnce(t *testing.T) {
	t.Parallel()
	home := nativeHome(t)
	bin := fakeExecutable(t, home, "starship", "printf 'prompt_starship_precmd() { :; }\\n(( ++starship_inits ))\\n'\n")
	r := nativeRun(t, home, `_selfishell_command_path() { command -v "$1"; }; typeset -gi starship_inits=0; source "$SELFISHELL_SOURCE"; source "$SELFISHELL_SOURCE"; print -r -- "$starship_inits"`, "SELFISHELL_SOURCE="+interactiveSource(), "SELFISHELL_COMMON_DIR="+filepath.Join(repoRoot(), "config/shared/zsh"), "PATH="+bin)
	if string(r.Stdout) != "1\n" {
		t.Fatalf("inits: %q", r.Stdout)
	}
}
func TestNativePluginLoadOrder(t *testing.T) {
	t.Parallel()
	home := nativeHome(t)
	bin := fakeExecutable(t, home, "fzf", "printf ':\\n'\n")
	nativePlugins(t, home, "fzf-tab", "zsh-autosuggestions", "fast-syntax-highlighting")
	log := filepath.Join(filepath.Dir(home), "plugin-log")
	r := nativeRun(t, home, `_selfishell_command_path() { command -v "$1"; }; SELFISHELL_ZSH_PLUGINS="$HOME/.local/share/selfishell/zsh-plugins"; source "$SELFISHELL_SOURCE"; print -r -- "manual=$ZSH_AUTOSUGGEST_MANUAL_REBIND handler=$(zle -F)"`, "SELFISHELL_SOURCE="+interactiveSource(), "SELFISHELL_COMMON_DIR="+filepath.Join(repoRoot(), "config/shared/zsh"), "PATH="+bin+":"+nativePath, "SELFISHELL_TEST_PLUGIN_LOG="+log)
	if !regexp.MustCompile(`^manual=1 handler=zle -F [0-9]+ _selfishell_deferred_plugins\n$`).Match(r.Stdout) {
		t.Fatalf("deferred registration: %q", r.Stdout)
	}
	// fzf-tab loads synchronously; the deferred pair waits for the line editor.
	if data, err := os.ReadFile(log); err != nil || string(data) != "fzf-tab\n" {
		t.Fatalf("sourced plugins: %q %v", data, err)
	}
}

// The deferred pair loads once, after zle draws the first prompt and starts
// reading, so neither plugin delays the prompt.
func TestNativeDeferredPluginsLoadAfterFirstPrompt(t *testing.T) {
	t.Parallel()
	// Sourcing the configuration twice before the first prompt must still leave
	// exactly one pending handler; a second one would spin on /dev/null.
	// Sourcing it again after the prompt must not load any plugin a second
	// time; each load wraps every widget once more.
	for _, tc := range []struct {
		name     string
		sources  int
		resource bool
	}{{"sourced-1", 1, false}, {"sourced-2", 2, false}, {"resourced-after-prompt", 1, true}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			testDeferredPluginsLoadAfterFirstPrompt(t, tc.sources, tc.resource)
		})
	}
}

func testDeferredPluginsLoadAfterFirstPrompt(t *testing.T, sources int, resource bool) {
	home := nativeHome(t)
	root := filepath.Dir(home)
	log := filepath.Join(root, "plugin-log")
	plugins := filepath.Join(home, ".local/share/selfishell/zsh-plugins")
	// Each stub logs when it is sourced and defines the function Selfishell
	// checks to tell that the real plugin is already loaded.
	nativeWrite(t, filepath.Join(plugins, "fzf-tab/fzf-tab.plugin.zsh"), `print -r -- fzf-tab >>"$SELFISHELL_TEST_PLUGIN_LOG"
fzf-tab-complete() { :; }
`, 0600)
	nativeWrite(t, filepath.Join(plugins, "zsh-autosuggestions/zsh-autosuggestions.plugin.zsh"), `print -r -- zsh-autosuggestions >>"$SELFISHELL_TEST_PLUGIN_LOG"
_zsh_autosuggest_start() { :; }
_zsh_autosuggest_bind_widgets() { print -r -- bind >>"$SELFISHELL_TEST_PLUGIN_LOG"; }
`, 0600)
	nativeWrite(t, filepath.Join(plugins, "fast-syntax-highlighting/fast-syntax-highlighting.plugin.zsh"), `print -r -- fast-syntax-highlighting >>"$SELFISHELL_TEST_PLUGIN_LOG"
_zsh_highlight() { :; }
`, 0600)
	fzf := fakeExecutable(t, home, "fzf", "printf ':\\n'\n")
	// precmd also logs how many zle -F handlers are pending at that moment.
	nativeWrite(t, filepath.Join(home, ".zshrc"), strings.Repeat(`source "$SELFISHELL_SOURCE"
`, sources)+`precmd() { print -r -- "precmd $(zle -F | wc -l | tr -d ' ')" >>"$SELFISHELL_TEST_PLUGIN_LOG"; }
zle-line-init() { print -r -- line-init >>"$SELFISHELL_TEST_PLUGIN_LOG"; }
zle -N zle-line-init
`, 0600)
	master, slave, err := pty.Open()
	mustFS(t, err)
	defer master.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	// Only Selfishell's startup is under test: Ubuntu's /etc/zsh/zshrc runs its
	// own compinit, which prompts on insecure directories and never reaches
	// the first prompt on CI runners.
	cmd := exec.CommandContext(ctx, nativeZsh, "--no-globalrcs", "-i")
	cmd.Dir = home
	cmd.Env = withEnv(baseEnv(home, root), "PATH="+fzf+":"+nativePath, "ZDOTDIR="+home, "TERM=xterm", "WSL_DISTRO_NAME=", "SELFISHELL_UPDATE_NOTICE=0", "SELFISHELL_SOURCE="+nativeCommon(), "SELFISHELL_TEST_PLUGIN_LOG="+log)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	mustFS(t, cmd.Start())
	slave.Close()
	var screen bytes.Buffer
	drained := make(chan struct{})
	go func() { io.Copy(&screen, master); close(drained) }()
	defer func() {
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		cmd.Wait()
	}()
	waitLog := func(line string, count int) {
		t.Helper()
		for {
			data, _ := os.ReadFile(log)
			if strings.Count(string(data), line+"\n") >= count {
				return
			}
			if ctx.Err() != nil {
				// Stop the shell and drain the terminal so the screen is readable.
				syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
				cmd.Wait()
				master.Close()
				<-drained
				t.Fatalf("waiting for %s x%d: log %q screen %q", line, count, data, screen.String())
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	waitLog("bind", 1)
	_, err = master.Write([]byte(":\n"))
	mustFS(t, err)
	waitLog("line-init", 2)
	want := "fzf-tab\nprecmd 1\nline-init\nzsh-autosuggestions\nfast-syntax-highlighting\nbind\nprecmd 0\nline-init\n"
	if resource {
		_, err = master.Write([]byte("source \"$SELFISHELL_SOURCE\"\n"))
		mustFS(t, err)
		waitLog("line-init", 3)
		want += "precmd 0\nline-init\n"
	}
	_, err = master.Write([]byte("exit\n"))
	mustFS(t, err)
	if err := cmd.Wait(); err != nil {
		t.Fatalf("shell exit: %v", err)
	}
	master.Close()
	<-drained
	data, err := os.ReadFile(log)
	mustFS(t, err)
	if string(data) != want {
		t.Fatalf("load order: got %q want %q", data, want)
	}
	if strings.Contains(screen.String(), "_selfishell") || strings.Contains(screen.String(), "zle") {
		t.Fatalf("shell reported an error: %q", screen.String())
	}
}
func TestNativeShellToolCacheReplacement(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ tool, replacement string }{
		{"zoxide", "preserved-mtime"}, {"zoxide", "older-mtime"}, {"zoxide", "symlink"},
		{"fzf", "preserved-mtime"}, {"starship", "preserved-mtime"},
	} {
		tool, replacement := tc.tool, tc.replacement
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
func TestNativeStarshipContinuationCache(t *testing.T) {
	t.Parallel()
	body := `case "$*" in
"init zsh") printf 'called\n' >>"$HOME/generations"; printf '%s\n' "$SELFISHELL_TEST_INIT" ;;
"prompt --continuation") printf 'called\n' >>"$HOME/continuations"; style=
  { read -r style <"${STARSHIP_CONFIG:-$HOME/.config/starship.toml}"; } 2>/dev/null
  printf '%%{\033[90m%%}%s>%s%%{\033[0m%%} ' "$STARSHIP_SHELL" "$style" ;;
*) exit 1 ;;
esac
`
	calls := func(t *testing.T, path string) int {
		t.Helper()
		b, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			return 0
		}
		mustFS(t, err)
		return strings.Count(string(b), "called\n")
	}
	t.Run("folded", func(t *testing.T) {
		home := nativeHome(t)
		starship := filepath.Join(fakeExecutable(t, home, "starship", body), "starship")
		init := "export STARSHIP_SHELL=\"zsh\"\nPROMPT='$('" + starship + "' prompt)'\nPROMPT2=\"$(" + starship + " prompt --continuation)\""
		cache := filepath.Join(home, ".cache/selfishell/starship-init.zsh")
		run := func(config, style string, generations int) {
			t.Helper()
			r := interactiveRun(t, home, `print -r -- "$PROMPT2"`, "SELFISHELL_TEST_INIT="+init, "STARSHIP_CONFIG="+config)
			if want := "%{\x1b[90m%}zsh>" + style + "%{\x1b[0m%} \n"; string(r.Stdout) != want {
				t.Fatalf("PROMPT2 %q want %q", r.Stdout, want)
			}
			if g, c := calls(t, filepath.Join(home, "generations")), calls(t, filepath.Join(home, "continuations")); g != generations || c != generations {
				t.Fatalf("generations %d continuations %d want %d", g, c, generations)
			}
			lines := strings.Split(strings.TrimSuffix(nativeRead(t, cache), "\n"), "\n")
			if last := lines[len(lines)-1]; !strings.HasPrefix(last, "PROMPT2=") || strings.Contains(last, "$(") {
				t.Fatalf("cached PROMPT2 line %q", last)
			}
		}
		run("", "", 1)
		run("", "", 1)
		nativeWrite(t, filepath.Join(home, ".config/starship.toml"), "one\n", 0600)
		run("", "one", 2)
		run("", "one", 2)
		nativeWrite(t, filepath.Join(home, ".config/starship.toml"), "three\n", 0600)
		run("", "three", 3)
		other := filepath.Join(home, "other.toml")
		nativeWrite(t, other, "two\n", 0600)
		run(other, "two", 4)
		run(other, "two", 4)
	})
	t.Run("unchanged-init", func(t *testing.T) {
		home := nativeHome(t)
		fakeExecutable(t, home, "starship", body)
		init := "export STARSHIP_SHELL=\"zsh\"\nPROMPT2='> '"
		r := interactiveRun(t, home, `print -r -- "$PROMPT2"`, "SELFISHELL_TEST_INIT="+init)
		if string(r.Stdout) != "> \n" {
			t.Fatalf("PROMPT2 %q", r.Stdout)
		}
		if _, cached, _ := strings.Cut(nativeRead(t, filepath.Join(home, ".cache/selfishell/starship-init.zsh")), "\n"); cached != init+"\n" {
			t.Fatalf("cache %q want %q", cached, init+"\n")
		}
		if c := calls(t, filepath.Join(home, "continuations")); c != 0 {
			t.Fatalf("continuations %d", c)
		}
	})
}
func TestNativeFzfCacheGeneration(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"success", "nonzero-exit", "empty-output", "invalid-syntax"} {
		t.Run(kind, func(t *testing.T) {
			home := nativeHome(t)
			dir := interactiveCache(t, home)
			p := filepath.Join(dir, "cache.zsh")
			bin := fakeExecutable(t, home, "fzf", `printf 'bindkey -M emacs "^R" fzf-history-widget\n'`)
			if kind == "invalid-syntax" {
				nativeWrite(t, filepath.Join(bin, "fzf"), "#!/bin/sh\nprintf 'if [[ not valid zsh\\n'\n", 0700)
			} else if kind == "nonzero-exit" {
				nativeWrite(t, filepath.Join(bin, "fzf"), "#!/bin/sh\nprintf 'print partial\\n'\nexit 1\n", 0700)
			} else if kind == "empty-output" {
				nativeWrite(t, filepath.Join(bin, "fzf"), "#!/bin/sh\nexit 0\n", 0700)
			}
			if kind != "success" {
				nativeWrite(t, p, "# preexisting fzf cache\n", 0600)
			}
			r := interactiveRun(t, home, `if _selfishell_generate_fzf_cache "$SELFISHELL_TEST_CACHE"; then [[ "$SELFISHELL_TEST_KIND" == success ]] || exit 10; else [[ "$SELFISHELL_TEST_KIND" != success ]] || exit 11; fi`, "SELFISHELL_TEST_KIND="+kind, "PATH="+bin, "SELFISHELL_TEST_CACHE="+p, "SELFISHELL_SOURCE="+interactiveFzfSource(t, home))
			nativeQuiet(t, r)
			if kind == "success" {
				if !strings.Contains(nativeRead(t, p), "fzf-history-widget") {
					t.Fatal("missing binding")
				}
			} else {
				cacheEqual(t, p, "# preexisting fzf cache\n")
			}
			nativeAssertNoTemp(t, dir)
		})
	}
}
func TestNativeFzfFallbackCopy(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, fzf, fallback string
		valid               bool
	}{
		{"missing-fzf", "", "# fallback bindings\n", true},
		{"nonzero-exit", "printf 'print partial\\n'\nexit 1\n", "# fallback bindings\n", true},
		{"empty-output", "exit 0\n", "# fallback bindings\n", true},
		{"empty-fallback", "", "", false},
		{"invalid-fallback", "", "if [[ not valid zsh\n", false},
		{"invalid-output", "printf 'if [[ not valid zsh\\n'\n", "# fallback bindings\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := nativeHome(t)
			dir := interactiveCache(t, home)
			bin := interactiveBin(t, home)
			mustFS(t, os.Symlink("/bin/cat", filepath.Join(bin, "cat")))
			if tc.fzf != "" {
				fakeExecutable(t, home, "fzf", tc.fzf)
			}
			nativeWrite(t, filepath.Join(home, "key-bindings.zsh"), tc.fallback, 0600)
			p := filepath.Join(dir, "fallback-cache.zsh")
			nativeWrite(t, p, "# preexisting fzf cache\n", 0600)
			r := interactiveRun(t, home, `if _selfishell_generate_fzf_cache "$SELFISHELL_TEST_CACHE"; then [[ "$SELFISHELL_TEST_VALID" == true ]] || exit 10; else [[ "$SELFISHELL_TEST_VALID" == false ]] || exit 11; fi`, "SELFISHELL_TEST_CACHE="+p, "SELFISHELL_TEST_VALID="+fmt.Sprint(tc.valid), "SELFISHELL_SOURCE="+interactiveFzfSource(t, home))
			nativeQuiet(t, r)
			want := "# preexisting fzf cache\n"
			if tc.valid {
				want = "\n# fallback bindings\n"
			}
			cacheEqual(t, p, want)
			nativeAssertNoTemp(t, dir)
		})
	}
}
func TestNativeKubectlCanonicalCompletion(t *testing.T) {
	t.Parallel()
	home := nativeHome(t)
	bin := fakeExecutable(t, home, "kubectl", `[ "$*" = "completion zsh" ] || exit 1
printf '_kubectl() { return 0; }\n'`)
	r := nativeRun(t, home, `_selfishell_command_path() { command -v "$1"; }; source "$SELFISHELL_SOURCE"; print -rl -- "before=${_comps[kubectl]}" "k=${+_comps[k]}"; _selfishell_kubectl_completion || exit 10; print -rl -- "after=${_comps[kubectl]}" "k=${+_comps[k]}"`, "PATH="+bin, "SELFISHELL_SOURCE="+nativeCompletion())
	if got := string(r.Stdout); got != "before=_selfishell_kubectl_completion\nk=0\nafter=_kubectl\nk=0\n" {
		t.Fatalf("completion %q stderr %q", got, r.Stderr)
	}
}
func TestNativeEditorAliasesAndExports(t *testing.T) {
	t.Parallel()
	home := nativeHome(t)
	bin := fakeExecutable(t, home, "nvim", "exit 0")
	source := filepath.Join(repoRoot(), "config/shared/zsh/aliases.zsh")
	r := nativeRun(t, home, `_selfishell_command_path() { command -v "$1"; }; unset EDITOR VISUAL; source "$SELFISHELL_SOURCE"; print -rl -- "alias=${aliases[vim]}" "first=$EDITOR|$VISUAL|${(t)EDITOR}|${(t)VISUAL}"; EDITOR='code --wait'; unset VISUAL; source "$SELFISHELL_SOURCE"; print -r -- "second=$EDITOR|$VISUAL"; unset EDITOR; VISUAL='emacsclient -c'; source "$SELFISHELL_SOURCE"; print -r -- "third=$EDITOR|$VISUAL"; EDITOR=nano; source "$SELFISHELL_SOURCE"; print -r -- "fourth=$EDITOR|$VISUAL"`, "PATH="+bin, "SELFISHELL_SOURCE="+source)
	want := "alias=nvim\nfirst=nvim|nvim|scalar-export|scalar-export\nsecond=code --wait|code --wait\nthird=nvim|emacsclient -c\nfourth=nano|emacsclient -c\n"
	if string(r.Stdout) != want {
		t.Fatalf("editor: %q want %q", r.Stdout, want)
	}
}
func TestNativeMissingNeovim(t *testing.T) {
	t.Parallel()
	home := nativeHome(t)
	empty := filepath.Join(home, "empty-bin")
	mustFS(t, os.MkdirAll(empty, 0700))
	source := filepath.Join(repoRoot(), "config/shared/zsh/aliases.zsh")
	r := nativeRun(t, home, `_selfishell_command_path() { command -v "$1"; }; unset EDITOR VISUAL; source "$SELFISHELL_SOURCE"; print -rl -- "vim=${+aliases[vim]}" "editor=${+EDITOR}" "visual=${+VISUAL}"`, "PATH="+empty, "SELFISHELL_SOURCE="+source)
	want := "vim=0\neditor=0\nvisual=0\n"
	if string(r.Stdout) != want {
		t.Fatalf("aliases: %q", r.Stdout)
	}
}
func TestNativeFzfPaletteAndUserOptions(t *testing.T) {
	t.Parallel()
	for _, userOpts := range []string{"", "--with-nth=2.. --bind=ctrl-a:select-all"} {
		t.Run(fmt.Sprintf("opts=%q", userOpts), func(t *testing.T) {
			home := nativeHome(t)
			fakeExecutable(t, home, "fzf", "printf ':\\n'")
			nativePlugins(t, home, "fzf-tab")
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
