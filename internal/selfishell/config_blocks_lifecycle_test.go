package selfishell

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func blockHome(t *testing.T, platform string) (string, string, Paths) {
	t.Helper()
	root := testRelease(t)
	home := t.TempDir()
	isolateHome(t, home)
	for _, key := range []string{"MISE_DATA_DIR", "MISE_STATE_DIR", "MISE_CACHE_DIR", "MISE_CONFIG_DIR"} {
		t.Setenv(key, filepath.Join(home, strings.ToLower(key)))
	}
	tools := filepath.Join(home, "tools")
	if err := os.Mkdir(tools, 0700); err != nil {
		t.Fatal(err)
	}
	for name, source := range map[string]string{"cksum": "/usr/bin/cksum", "date": "/bin/date"} {
		if err := os.Symlink(source, filepath.Join(tools, name)); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", tools)
	t.Setenv("MISE_GLOBAL_CONFIG_FILE", filepath.Join(home, "mise/global.toml"))
	t.Setenv("MISE_DEFAULT_CONFIG_FILENAME", "")
	t.Setenv("MISE_OVERRIDE_CONFIG_FILENAMES", "")
	t.Setenv("MISE_TRUSTED_CONFIG_PATHS", home)
	t.Setenv("MISE_IGNORED_CONFIG_PATHS", "")
	t.Setenv("MISE_OFFLINE", "1")
	if err := os.Mkdir(filepath.Join(home, "tmp"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", filepath.Join(home, "tmp"))
	t.Setenv("SHELL", "/bin/zsh")
	if platform == "macos" {
		t.Setenv("SELFISHELL_TEST_SYSTEM_NAME", "Darwin")
	} else {
		t.Setenv("SELFISHELL_TEST_SYSTEM_NAME", "Linux")
		release := filepath.Join(home, "os-release")
		if err := os.WriteFile(release, []byte("ID=ubuntu\n"), 0600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("SELFISHELL_TEST_OS_RELEASE_FILE", release)
	}
	paths, err := UserPaths()
	if err != nil {
		t.Fatal(err)
	}
	return root, home, paths
}
func blockRun(t *testing.T, root string, input string, args ...string) (int, string, string) {
	t.Helper()
	var out, stderr bytes.Buffer
	c := CLI{Root: root, In: strings.NewReader(input), Out: &out, Err: &stderr}
	code := c.Run(args)
	return code, out.String(), stderr.String()
}
func blockOK(t *testing.T, root string, args ...string) string {
	t.Helper()
	code, out, stderr := blockRun(t, root, "", args...)
	if code != 0 {
		t.Fatalf("%v: %d %s %s", args, code, out, stderr)
	}
	return out
}
func blockRead(t *testing.T, path string) []byte {
	t.Helper()
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func blockWrite(t *testing.T, path string, b []byte) {
	t.Helper()
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(path, b, 0640); e != nil {
		t.Fatal(e)
	}
}
func blockState(t *testing.T, paths Paths, name string) State {
	t.Helper()
	s, e := ReadState(paths.Resources + "/" + name + ".state")
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func blockEqual(t *testing.T, path string, want []byte) {
	t.Helper()
	if got := blockRead(t, path); !bytes.Equal(got, want) {
		t.Fatalf("%s: got %q want %q", path, got, want)
	}
}

func TestManagedBlockAtEOFWithoutNewline(t *testing.T) {
	for _, name := range []string{"user-zshrc", "user-zprofile", "user-vimrc", "user-ghostty", "user-zshenv"} {
		for _, update := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/update=%t", name, update), func(t *testing.T) {
				platform := "macos"
				if name == "user-zshenv" {
					platform = "ubuntu"
				}
				root, _, paths := blockHome(t, platform)
				blockOK(t, root, "install", "--skip-packages", "--yes")
				r := failureResource(t, root, name)
				content, err := blockContent(name, paths.Config)
				if err != nil {
					t.Fatal(err)
				}
				personal := []byte("# personal content\n")
				blockWrite(t, r.Target, append(bytes.Clone(personal), bytes.TrimSuffix(content, []byte("\n"))...))
				if update {
					blockOK(t, root, "update", "--tools-only", "--skip-packages", "--yes")
				}
				blockOK(t, root, "uninstall", "--yes")
				blockEqual(t, r.Target, personal)
			})
		}
	}
}

func TestMissingActiveUserBlockCanBeReinstalledAndUninstalled(t *testing.T) {
	for _, tc := range []struct {
		name    string
		removed bool
		command []string
	}{
		{"install edited rc", false, []string{"install", "--skip-packages", "--yes"}},
		{"tools update edited rc", false, []string{"update", "--tools-only", "--skip-packages", "--yes"}},
		{"install deleted rc", true, []string{"install", "--skip-packages", "--yes"}},
		{"tools update deleted rc", true, []string{"update", "--tools-only", "--skip-packages", "--yes"}},
		{"uninstall edited rc", false, []string{"uninstall", "--yes"}},
		{"uninstall deleted rc", true, []string{"uninstall", "--yes"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, home, paths := blockHome(t, "macos")
			blockOK(t, root, "install", "--skip-packages", "--yes")
			target := home + "/.zshrc"
			personal := []byte("alias personal='yes'\n")
			if tc.removed {
				if err := os.Remove(target); err != nil {
					t.Fatal(err)
				}
			} else {
				blockWrite(t, target, personal)
			}
			blockOK(t, root, tc.command...)
			if tc.command[0] == "uninstall" {
				if tc.removed {
					if _, err := os.Lstat(target); !os.IsNotExist(err) {
						t.Fatalf("recreated deleted rc: %v", err)
					}
				} else {
					blockEqual(t, target, personal)
				}
				if _, err := os.Lstat(paths.Resources + "/user-zshrc.state"); !os.IsNotExist(err) {
					t.Fatalf("retained state: %v", err)
				}
				return
			}
			data := blockRead(t, target)
			view, err := inspectBlock("user-zshrc", data)
			if err != nil || view.status != "intact" {
				t.Fatalf("block not restored: %s %v", view.status, err)
			}
			if !tc.removed && !bytes.Contains(data, personal) {
				t.Fatalf("personal content lost: %q", data)
			}
			if blockState(t, paths, "user-zshrc").Status != "active" {
				t.Fatal("active state not restored")
			}
		})
	}
}

func TestStatusDoesNotBlockOnReplacedUserRCFIFO(t *testing.T) {
	root, home, _ := blockHome(t, "macos")
	blockOK(t, root, "install", "--skip-packages", "--yes")
	target := home + "/.zshrc"
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(target, 0600); err != nil {
		t.Fatal(err)
	}
	done := make(chan int, 1)
	go func() { code, _, _ := blockRun(t, root, "", "status"); done <- code }()
	select {
	case code := <-done:
		if code != 1 {
			t.Fatalf("status accepted FIFO: %d", code)
		}
	case <-time.After(2 * time.Second):
		// Release the old implementation's blocking reader before failing.
		fd, err := syscall.Open(target, syscall.O_WRONLY|syscall.O_NONBLOCK, 0)
		if err == nil {
			syscall.Close(fd)
		}
		<-done
		t.Fatal("status blocked reading a user rc FIFO")
	}
}

func TestBlockHomeExcludesAmbientMiseAndGlobalConfig(t *testing.T) {
	ambient := t.TempDir()
	bin := filepath.Join(ambient, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(ambient, "mise-invoked")
	global := filepath.Join(ambient, "global.toml")
	blockWrite(t, global, []byte("private sentinel\n"))
	blockWrite(t, filepath.Join(bin, "mise"), []byte("#!/bin/sh\nprintf invoked >\"$MISE_SENTINEL_MARKER\"\ncat \"$MISE_GLOBAL_CONFIG_FILE\" >>\"$MISE_SENTINEL_MARKER\"\n"))
	if err := os.Chmod(filepath.Join(bin, "mise"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	t.Setenv("MISE_GLOBAL_CONFIG_FILE", global)
	t.Setenv("MISE_SENTINEL_MARKER", marker)
	root, home, _ := blockHome(t, "macos")
	if _, err := exec.LookPath("mise"); err == nil {
		t.Fatal("ambient mise remained on fixture PATH")
	}
	if got := os.Getenv("MISE_GLOBAL_CONFIG_FILE"); got != filepath.Join(home, "mise/global.toml") {
		t.Fatalf("ambient global config remained: %q", got)
	}
	blockOK(t, root, "install", "--skip-packages", "--yes")
	blockOK(t, root, "update", "--tools-only", "--skip-packages", "--yes")
	if _, err := os.Lstat(marker); !os.IsNotExist(err) {
		t.Fatalf("ambient mise was invoked: %v", err)
	}
	blockEqual(t, global, []byte("private sentinel\n"))
}

func TestCurrentNeovimSourcesAreDeclaredOnce(t *testing.T) {
	root, _, _ := blockHome(t, "macos")
	resources, err := ManagedResources(root)
	if err != nil {
		t.Fatal(err)
	}
	count := map[string]int{}
	for _, r := range resources {
		count[r.Source]++
	}
	base := filepath.Join(root, "config/shared/nvim")
	err = filepath.WalkDir(base, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if !d.IsDir() && count[path] != 1 {
			t.Errorf("current Neovim source %s declared %d times", path, count[path])
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
func TestFreshUserBlockPreflightRejectsForeignPaths(t *testing.T) {
	for _, tc := range []struct{ platform, name, path, kind, diagnostic string }{
		{"macos", "zshrc-link", ".zshrc", "link", "Refusing to modify symbolic link"},
		{"macos", "zprofile-link", ".zprofile", "link", "Refusing to modify symbolic link"},
		{"macos", "vimrc-link", ".vimrc", "link", "Refusing to modify symbolic link"},
		{"ubuntu", "zshenv-link", ".zshenv", "link", "Refusing to modify symbolic link"},
		{"macos", "ghostty-link", ".config/ghostty/config.ghostty", "link", "Refusing to modify symbolic link"},
		{"macos", "ghostty-directory", ".config/ghostty/config.ghostty", "directory", "Refusing to modify non-regular block path"},
		{"macos", "zshrc-malformed", ".zshrc", "malformed", "Cannot manage"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, home, paths := blockHome(t, tc.platform)
			target := filepath.Join(home, tc.path)
			if e := os.MkdirAll(filepath.Dir(target), 0700); e != nil {
				t.Fatal(e)
			}
			foreign := filepath.Join(home, "personal-reference")
			switch tc.kind {
			case "link":
				blockWrite(t, foreign, []byte("personal\x00bytes\n"))
				if e := os.Symlink(foreign, target); e != nil {
					t.Fatal(e)
				}
			case "directory":
				if e := os.MkdirAll(target+"/keep", 0700); e != nil {
					t.Fatal(e)
				}
			case "malformed":
				blockWrite(t, target, []byte("# >>> Selfishell initialize >>>\nuser content\n"))
			}
			targetInfo, _ := os.Lstat(target)
			before := []byte(nil)
			if tc.kind != "directory" {
				before = blockRead(t, target)
			}
			code, _, stderr := blockRun(t, root, "", "install", "--skip-packages", "--yes")
			if code != 1 || !strings.Contains(stderr, tc.diagnostic) {
				t.Fatalf("%d %s", code, stderr)
			}
			info, e := os.Lstat(target)
			if e != nil || info.Mode() != targetInfo.Mode() {
				t.Fatalf("target changed: %v %v", info, e)
			}
			if tc.kind != "directory" {
				blockEqual(t, target, before)
			} else if _, e := os.Stat(target + "/keep"); e != nil {
				t.Fatal(e)
			}
			if tc.kind == "link" {
				link, e := os.Readlink(target)
				if e != nil || link != foreign {
					t.Fatalf("link %q %v", link, e)
				}
				blockEqual(t, foreign, []byte("personal\x00bytes\n"))
			}
			for _, p := range []string{paths.Config, paths.State} {
				if _, e := os.Lstat(p); !os.IsNotExist(e) {
					t.Fatalf("preflight created %s: %v", p, e)
				}
			}
		})
	}
}
func TestManagedBlockStateIdentityMismatch(t *testing.T) {
	root, home, paths := blockHome(t, "macos")
	blockOK(t, root, "install", "--skip-packages", "--yes")
	target := home + "/.zprofile"
	statePath := paths.Resources + "/user-zprofile.state"
	original := blockRead(t, target)
	for _, tc := range []struct{ name, kind, target string }{{"kind", "file", target}, {"target", "block", target + ".other"}} {
		t.Run(tc.name, func(t *testing.T) {
			state := State{Kind: tc.kind, Status: "active", Target: tc.target, Reference: "-", Backup: "-", Checksum: "bogus-checksum"}
			if e := WriteState(statePath, state); e != nil {
				t.Fatal(e)
			}
			before := blockRead(t, statePath)
			code, _, stderr := blockRun(t, root, "", "install", "--skip-packages", "--yes")
			if code != 1 || !strings.Contains(stderr, "State conflict for managed block") {
				t.Fatalf("%d %s", code, stderr)
			}
			blockEqual(t, target, original)
			blockEqual(t, statePath, before)
		})
	}
}
func TestOutdatedUserBlocksUpgradeInPlace(t *testing.T) {
	for _, tc := range []struct{ name, platform, path, old string }{
		{"zprofile", "macos", ".zprofile", "# >>> Selfishell mise shims >>>\nif command -v mise >/dev/null 2>&1; then\n  eval \"$(command mise activate zsh --shims)\"\nfi\n# <<< Selfishell mise shims <<<\n"},
		{"ghostty", "macos", ".config/ghostty/config.ghostty", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, home, paths := blockHome(t, tc.platform)
			blockOK(t, root, "install", "--skip-packages", "--yes")
			target := filepath.Join(home, tc.path)
			old := []byte(tc.old)
			if tc.name == "ghostty" {
				old = []byte("# >>> Selfishell ghostty >>>\nconfig-file = " + paths.Config + "/ghostty/config.ghostty\n# <<< Selfishell ghostty <<<\n")
			}
			current, e := blockContent("user-"+tc.name, paths.Config)
			if e != nil {
				t.Fatal(e)
			}
			surrounding := []byte("before=한글\r\n")
			suffix := []byte("after=\x00bytes")
			blockWrite(t, target, append(append(append([]byte{}, surrounding...), old...), suffix...))
			checksum, e := checksumBytes(old)
			if e != nil {
				t.Fatal(e)
			}
			if e = WriteState(paths.Resources+"/user-"+tc.name+".state", State{"block", "active", target, "-", "-", checksum}); e != nil {
				t.Fatal(e)
			}
			blockOK(t, root, "install", "--skip-packages", "--yes")
			want := append(append(append([]byte{}, surrounding...), current...), suffix...)
			blockEqual(t, target, want)
			active := blockState(t, paths, "user-"+tc.name)
			if active.Status != "active" || active.Checksum == checksum {
				t.Fatalf("state %+v", active)
			}
			blockOK(t, root, "install", "--skip-packages", "--yes")
			blockEqual(t, target, want)
		})
	}
}
func TestModifiedZprofileBlockDecisions(t *testing.T) {
	for _, decision := range []string{"overwrite", "skip", "yes"} {
		t.Run(decision, func(t *testing.T) {
			root, home, paths := blockHome(t, "macos")
			blockOK(t, root, "install", "--skip-packages", "--yes")
			target := home + "/.zprofile"
			clean := blockRead(t, target)
			clean = append([]byte("export BEFORE=1\r\n"), clean...)
			clean = append(clean, []byte("export AFTER=1")...)
			modified := bytes.Replace(clean, []byte("command mise activate zsh"), []byte("command mise activate --user-edited zsh"), 1)
			if bytes.Equal(modified, clean) {
				t.Fatal("fixture did not edit block")
			}
			blockWrite(t, target, modified)
			statePath := paths.Resources + "/user-zprofile.state"
			oldState := blockRead(t, statePath)
			later := filepath.Dir(paths.Config) + "/starship.toml"
			laterTarget := paths.Config + "/starship.toml"
			if decision == "skip" {
				if err := os.Remove(later); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("SELFISHELL_TEST_TTY", "1")
			args := []string{"install", "--skip-packages"}
			input := "y\ny\n"
			if decision == "skip" {
				input = "y\nn\n"
			}
			if decision == "yes" {
				args = append(args, "--yes")
				input = ""
			}
			code, out, stderr := blockRun(t, root, input, args...)
			if decision == "yes" {
				if code != 1 || !strings.Contains(stderr, "Managed block was modified; preserving it") {
					t.Fatalf("%d %s", code, stderr)
				}
			} else if code != 0 {
				t.Fatalf("%d %s %s", code, out, stderr)
			}
			if decision == "overwrite" {
				blockEqual(t, target, clean)
				backups, e := filepath.Glob(paths.State + "/backups/user-zprofile.backup.*")
				if e != nil || len(backups) != 1 {
					t.Fatalf("backups %v %v", backups, e)
				}
				blockEqual(t, backups[0], modified)
				if strings.Count(out, "Managed block was modified:") != 1 {
					t.Fatalf("prompt count: %s", out)
				}
			} else {
				blockEqual(t, target, modified)
				blockEqual(t, statePath, oldState)
				if _, e := os.Lstat(paths.State + "/backups"); !os.IsNotExist(e) {
					t.Fatalf("unexpected backup %v", e)
				}
				if decision == "skip" {
					if !strings.Contains(out, "Skipped modified managed block:") {
						t.Fatal(out)
					}
					link, err := os.Readlink(later)
					if err != nil || link != laterTarget {
						t.Fatalf("skip stopped later resource: link %q, error %v", link, err)
					}
					blockEqual(t, later, blockRead(t, root+"/config/shared/starship.toml"))
				}
			}
		})
	}
}

func TestUserBlocksAndGhosttyLifecycle(t *testing.T) {
	for _, platform := range []string{"macos", "ubuntu"} {
		t.Run(platform, func(t *testing.T) {
			root, home, paths := blockHome(t, platform)
			originals := map[string][]byte{
				".zshrc":    []byte("export BEFORE=한글\r\nalias tail=true"),
				".zprofile": []byte("export USER_ZPROFILE=kept"),
				".vimrc":    []byte("set background=dark\nset nocompatible\n"),
			}
			if platform == "ubuntu" {
				originals[".zshenv"] = []byte(`. "$HOME/.cargo/env"` + "\n")
			} else {
				originals[".zshenv"] = []byte("macOS personal zshenv\x00")
			}
			for name, data := range originals {
				blockWrite(t, home+"/"+name, data)
			}
			ghostty := filepath.Dir(paths.Config) + "/ghostty/config.ghostty"
			if platform == "macos" {
				blockWrite(t, ghostty, []byte("font-size = 14\r\ncursor-style = bar"))
			}
			blockOK(t, root, "install", "--skip-packages", "--yes")
			for name, original := range originals {
				target := home + "/" + name
				info, err := os.Lstat(target)
				if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0640 {
					t.Fatalf("%s user type/mode: %v %v", name, info, err)
				}
				got := blockRead(t, target)
				if platform == "macos" && name == ".zshenv" {
					blockEqual(t, target, original)
					continue
				}
				if !bytes.HasSuffix(got, original) {
					t.Fatalf("%s original suffix lost: %q", name, got)
				}
				marker := map[string]string{".zshrc": "# >>> Selfishell initialize >>>", ".zprofile": "# >>> Selfishell mise shims >>>", ".vimrc": "\" >>> Selfishell vimrc >>>", ".zshenv": "# >>> Selfishell zshenv >>>"}[name]
				if bytes.Count(got, []byte(marker)) != 1 {
					t.Fatalf("%s marker count: %q", name, got)
				}
				if name == ".zshenv" && !bytes.Contains(got, []byte("skip_global_compinit=1")) {
					t.Fatal("zshenv body missing")
				}
				state := blockState(t, paths, map[string]string{".zshrc": "user-zshrc", ".zprofile": "user-zprofile", ".vimrc": "user-vimrc", ".zshenv": "user-zshenv"}[name])
				if state.Kind != "block" {
					t.Fatalf("%s state %+v", name, state)
				}
			}
			if platform == "macos" {
				got := blockRead(t, ghostty)
				defaults := []byte("config-file = " + paths.Config + "/ghostty/config.ghostty")
				override := []byte("config-file = ?user.ghostty")
				if bytes.Index(got, defaults) < 0 || bytes.Index(got, override) <= bytes.Index(got, defaults) || !bytes.HasSuffix(got, []byte("font-size = 14\r\ncursor-style = bar")) {
					t.Fatalf("ghostty order/bytes: %q", got)
				}
				blockEqual(t, paths.State+"/ghostty", []byte("1\n"))
				if _, e := os.Lstat(paths.Config + "/../ghostty/user.ghostty"); !os.IsNotExist(e) {
					t.Fatalf("created user override: %v", e)
				}
				if _, e := os.Lstat(paths.Resources + "/user-zshenv.state"); !os.IsNotExist(e) {
					t.Fatalf("macOS managed zshenv: %v", e)
				}
			}
			// Personal edits before and after the loader must survive dry-run, repeat, sync, and uninstall.
			zshrc := home + "/.zshrc"
			edited := append([]byte("alias PREFIX=true\n"), blockRead(t, zshrc)...)
			edited = append(edited, []byte("\nexport AFTER=1")...)
			blockWrite(t, zshrc, edited)
			blockOK(t, root, "update", "--tools-only", "--skip-packages", "--dry-run", "--yes")
			blockEqual(t, zshrc, edited)
			blockOK(t, root, "install", "--skip-packages", "--yes")
			blockOK(t, root, "update", "--tools-only", "--skip-packages", "--yes")
			blockEqual(t, zshrc, edited)
			blockOK(t, root, "uninstall", "--yes")
			wantZsh := append([]byte("alias PREFIX=true\n"), originals[".zshrc"]...)
			wantZsh = append(wantZsh, []byte("\nexport AFTER=1")...)
			blockEqual(t, zshrc, wantZsh)
			for name, data := range originals {
				if name != ".zshrc" {
					blockEqual(t, home+"/"+name, data)
				}
			}
			if platform == "macos" {
				info, err := os.Lstat(ghostty)
				if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0640 {
					t.Fatalf("Ghostty user type/mode: %v %v", info, err)
				}
				blockEqual(t, ghostty, []byte("font-size = 14\r\ncursor-style = bar"))
			}
		})
	}
}
func TestUserGhosttyOverrideIsUntouched(t *testing.T) {
	for _, kind := range []string{"regular", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			root, _, paths := blockHome(t, "macos")
			override := filepath.Dir(paths.Config) + "/ghostty/user.ghostty"
			referent := filepath.Join(filepath.Dir(paths.Config), "personal-ghostty")
			original := []byte("theme = Catppuccin Mocha\nfont-size = 15\x00")
			if kind == "regular" {
				blockWrite(t, override, original)
			} else {
				blockWrite(t, referent, original)
				if e := os.MkdirAll(filepath.Dir(override), 0700); e != nil {
					t.Fatal(e)
				}
				if e := os.Symlink(referent, override); e != nil {
					t.Fatal(e)
				}
			}
			check := func() {
				t.Helper()
				if kind == "regular" {
					info, e := os.Lstat(override)
					if e != nil || !info.Mode().IsRegular() {
						t.Fatalf("regular override %v %v", info, e)
					}
					blockEqual(t, override, original)
				} else {
					link, e := os.Readlink(override)
					if e != nil || link != referent {
						t.Fatalf("override link %q %v", link, e)
					}
					blockEqual(t, referent, original)
				}
			}
			blockOK(t, root, "install", "--skip-packages", "--yes")
			check()
			blockOK(t, root, "update", "--tools-only", "--skip-packages", "--yes")
			check()
			blockOK(t, root, "uninstall", "--yes")
			check()
		})
	}
}
func TestRepeatedInstallKeepsStateInodesAndBackups(t *testing.T) {
	root, home, paths := blockHome(t, "macos")
	blockWrite(t, home+"/.zshrc", []byte("original zshrc"))
	blockOK(t, root, "install", "--skip-packages", "--yes")
	entries, e := os.ReadDir(paths.Resources)
	if e != nil {
		t.Fatal(e)
	}
	before := map[string]os.FileInfo{}
	for _, entry := range entries {
		info, e := os.Stat(paths.Resources + "/" + entry.Name())
		if e != nil {
			t.Fatal(e)
		}
		before[entry.Name()] = info
	}
	backups := blockBackupNames(t, home)
	blockOK(t, root, "install", "--skip-packages", "--yes")
	after, e := os.ReadDir(paths.Resources)
	if e != nil || len(after) != len(entries) {
		t.Fatalf("state count %v %v", after, e)
	}
	for name, info := range before {
		next, e := os.Stat(paths.Resources + "/" + name)
		if e != nil || !os.SameFile(info, next) {
			t.Fatalf("state inode changed %s: %v", name, e)
		}
	}
	now := blockBackupNames(t, home)
	if len(now) != len(backups) {
		t.Fatalf("backup count changed %v -> %v", backups, now)
	}
	if bytes.Count(blockRead(t, home+"/.zshrc"), []byte("# >>> Selfishell initialize >>>")) != 1 {
		t.Fatal("duplicated zshrc loader")
	}
}
func TestSavedDeclinedGhosttyChoiceStaysExact(t *testing.T) {
	root, _, paths := blockHome(t, "macos")
	blockWrite(t, paths.State+"/ghostty", []byte("0\n"))
	blockOK(t, root, "install", "--skip-packages", "--yes")
	blockEqual(t, paths.State+"/ghostty", []byte("0\n"))
	for _, p := range []string{paths.Config + "/ghostty/config.ghostty", filepath.Dir(paths.Config) + "/ghostty/config.ghostty", paths.Resources + "/user-ghostty.state"} {
		if _, e := os.Lstat(p); !os.IsNotExist(e) {
			t.Fatalf("declined Ghostty path %s: %v", p, e)
		}
	}
}
func TestZprofileActivatesPrivateMise(t *testing.T) {
	if _, e := os.Stat("/bin/zsh"); e != nil {
		t.Skip("native zsh unavailable")
	}
	for _, kind := range []string{"path", "private"} {
		t.Run(kind, func(t *testing.T) {
			root, home, _ := blockHome(t, "macos")
			blockWrite(t, home+"/.zprofile", []byte("export USER_ZPROFILE=kept"))
			blockOK(t, root, "install", "--skip-packages", "--yes")
			blockOK(t, root, "install", "--skip-packages", "--yes")
			marker := []byte("# >>> Selfishell mise shims >>>")
			if bytes.Count(blockRead(t, home+"/.zprofile"), marker) != 1 {
				t.Fatal("duplicate zprofile block")
			}
			bin := filepath.Join(home, "fakebin")
			if kind == "private" {
				bin = home + "/.local/bin"
			}
			if e := os.MkdirAll(bin, 0700); e != nil {
				t.Fatal(e)
			}
			script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >\"$HOME/mise-args\"\nprintf '%s\\n' 'export SELFISHELL_MISE_SHIMS_TEST=loaded'\n"
			blockWrite(t, bin+"/mise", []byte(script))
			if e := os.Chmod(bin+"/mise", 0700); e != nil {
				t.Fatal(e)
			}
			path := "/usr/bin:/bin"
			if kind == "path" {
				path = bin + ":" + path
			}
			out, e := runNativeZprofile(home, path, 5*time.Second)
			if e != nil || strings.TrimSpace(string(out)) != "loaded" {
				t.Fatalf("mise activation: %q %v", out, e)
			}
			blockEqual(t, home+"/mise-args", []byte("activate zsh --shims\n"))
			blockOK(t, root, "uninstall", "--yes")
			blockEqual(t, home+"/.zprofile", []byte("export USER_ZPROFILE=kept"))
		})
	}
}

func runNativeZprofile(home, path string, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/zsh", "-dfc", `source "$HOME/.zprofile"; print "${SELFISHELL_MISE_SHIMS_TEST-}"`)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 500 * time.Millisecond
	cmd.Env = []string{"HOME=" + home, "PATH=" + path, "TMPDIR=" + home, "MISE_DATA_DIR=" + home + "/mise/data"}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	out, err := cmd.CombinedOutput()
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	if ctx.Err() != nil {
		return out, ctx.Err()
	}
	return out, err
}

func TestNativeZprofileTimeoutKillsMiseDescendant(t *testing.T) {
	if _, err := os.Stat("/bin/zsh"); err != nil {
		t.Skip("native zsh unavailable")
	}
	root, home, _ := blockHome(t, "macos")
	blockOK(t, root, "install", "--skip-packages", "--yes")
	bin := filepath.Join(home, "fakebin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	blockWrite(t, filepath.Join(bin, "mise"), []byte("#!/bin/sh\nprintf '%s\\n' \"$$\" >\"$HOME/hung-mise.pid\"\nexec /bin/sleep 30\n"))
	if err := os.Chmod(filepath.Join(bin, "mise"), 0700); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err := runNativeZprofile(home, bin+":/usr/bin:/bin", time.Second)
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 4*time.Second {
		t.Fatalf("native profile was not bounded: %v, elapsed %s", err, time.Since(start))
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(blockRead(t, home+"/hung-mise.pid"))))
	if err != nil || pid <= 0 {
		t.Fatalf("invalid private fake PID: %d %v", pid, err)
	}
	cleanup := true
	t.Cleanup(func() {
		if cleanup && syscall.Kill(pid, 0) == nil {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	})
	// SIGKILL delivery is asynchronous, and waiting for Zsh does not reap its
	// grandchild. Give that private descendant a bounded chance to stop.
	deadline := time.Now().Add(time.Second)
	for {
		probeErr := syscall.Kill(pid, 0)
		if probeErr == nil && runtime.GOOS == "linux" {
			// Container init may leave a killed grandchild as a non-running zombie.
			stat, readErr := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
			if readErr == nil {
				at := bytes.LastIndex(stat, []byte(") "))
				if at >= 0 && len(stat) > at+2 && stat[at+2] == 'Z' {
					probeErr = syscall.ESRCH
				}
			}
		}
		if errors.Is(probeErr, syscall.ESRCH) {
			cleanup = false
			break
		}
		if probeErr != nil {
			t.Fatal(probeErr)
		}
		if time.Now().After(deadline) {
			t.Fatalf("private fake mise descendant %d survived timeout", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestModifiedBlockBackupFailurePreservesTargetAndState(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("backup copy permission failure requires unprivileged process")
	}
	root, home, paths := blockHome(t, "macos")
	blockOK(t, root, "install", "--skip-packages", "--yes")
	target := home + "/.zprofile"
	modified := bytes.Replace(blockRead(t, target), []byte("command mise activate zsh"), []byte("command mise activate --user-edited zsh"), 1)
	if !bytes.Contains(modified, []byte("--user-edited")) {
		t.Fatal("bad conflict fixture")
	}
	blockWrite(t, target, modified)
	statePath := paths.Resources + "/user-zprofile.state"
	originalState := blockRead(t, statePath)
	backupDir := paths.State + "/backups"
	if err := os.Mkdir(backupDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(backupDir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(backupDir, 0700) })
	t.Setenv("SELFISHELL_TEST_TTY", "1")
	code, out, stderr := blockRun(t, root, "y\ny\n", "install", "--skip-packages")
	if code != 1 || strings.Contains(out, "Updated Selfishell block:") || stderr == "" {
		t.Fatalf("backup failure accepted: %d %q %q", code, out, stderr)
	}
	blockEqual(t, target, modified)
	blockEqual(t, statePath, originalState)
	entries, err := os.ReadDir(backupDir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed backup left files: %v %v", entries, err)
	}
}
func TestModifiedBlockReplaceFailureIsRetryable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("filesystem permission failure requires unprivileged process")
	}
	root, home, paths := blockHome(t, "macos")
	blockOK(t, root, "install", "--skip-packages", "--yes")
	target := home + "/.zprofile"
	clean := blockRead(t, target)
	modified := bytes.Replace(clean, []byte("command mise activate zsh"), []byte("command mise activate --user-edited zsh"), 1)
	if !bytes.Contains(modified, []byte("--user-edited")) {
		t.Fatal("bad conflict fixture")
	}
	blockWrite(t, target, modified)
	t.Setenv("SELFISHELL_TEST_TTY", "1")
	if e := os.Chmod(home, 0500); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = os.Chmod(home, 0700) })
	code, out, stderr := blockRun(t, root, "y\ny\n", "install", "--skip-packages")
	if e := os.Chmod(home, 0700); e != nil {
		t.Fatal(e)
	}
	if code != 1 || strings.Contains(out, "Updated Selfishell block:") || stderr == "" {
		t.Fatalf("replace failure accepted: %d %q %q", code, out, stderr)
	}
	blockEqual(t, target, modified)
	if state := blockState(t, paths, "user-zprofile"); state.Status != "pending" {
		t.Fatalf("state %+v", state)
	}
	temps, e := filepath.Glob(target + ".tmp.*")
	if e != nil || len(temps) != 0 {
		t.Fatalf("temporary files %v %v", temps, e)
	}
	blockOK(t, root, "install", "--skip-packages", "--yes")
	blockEqual(t, target, clean)
	if state := blockState(t, paths, "user-zprofile"); state.Status != "active" {
		t.Fatalf("state %+v", state)
	}
}

func TestGhosttyPersonalBytesSurviveDryRunReinstallAndUninstall(t *testing.T) {
	root, _, paths := blockHome(t, "macos")
	target := filepath.Dir(paths.Config) + "/ghostty/config.ghostty"
	original := []byte("font-size = 14\r\n")
	blockWrite(t, target, original)
	blockOK(t, root, "install", "--skip-packages", "--yes")
	installed := blockRead(t, target)
	edited := append([]byte("cursor-style = bar\n"), installed...)
	edited = append(edited, []byte("font-family = 한글\x00")...)
	blockWrite(t, target, edited)
	blockOK(t, root, "install", "--skip-packages", "--dry-run", "--yes")
	blockEqual(t, target, edited)
	blockOK(t, root, "install", "--skip-packages", "--yes")
	blockEqual(t, target, edited)
	blockOK(t, root, "update", "--tools-only", "--skip-packages", "--yes")
	blockEqual(t, target, edited)
	blockOK(t, root, "uninstall", "--yes")
	want := append([]byte("cursor-style = bar\n"), original...)
	want = append(want, []byte("font-family = 한글\x00")...)
	blockEqual(t, target, want)
}

func blockBackupNames(t *testing.T, home string) []string {
	t.Helper()
	var names []string
	err := filepath.WalkDir(home, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if strings.Contains(d.Name(), ".backup.") {
			names = append(names, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return names
}
