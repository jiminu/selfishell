package selfishell

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCurrentManifestMembershipAndDryRun(t *testing.T) {
	root := testRelease(t)
	packages, err := ReadPackages(filepath.Join(root, "packages.conf"))
	if err != nil {
		t.Fatal(err)
	}
	groups := map[string][]string{}
	for _, p := range packages {
		groups[p.Platform+":"+p.Requirement+":"+p.Manager] = append(groups[p.Platform+":"+p.Requirement+":"+p.Manager], p.Name)
	}
	for key, want := range map[string]string{
		"all:required:direct":    "mise",
		"all:required:mise":      "fzf gh jq lazygit neovim node python ripgrep starship tree-sitter uv zoxide",
		"all:optional:mise":      "bat eza",
		"macos:required:formula": "git vim",
		"macos:required:direct":  "zinit",
		"macos:optional:cask":    "font-meslo-lg-nerd-font font-noto-sans-cjk-kr",
		"ubuntu:required:apt":    "build-essential ca-certificates curl git vim zsh",
		"ubuntu:required:direct": "zinit",
	} {
		actual := groups[key]
		slices.Sort(actual)
		if strings.Join(actual, " ") != want {
			t.Fatalf("%s: %q want %q", key, actual, want)
		}
		delete(groups, key)
	}
	if len(groups) != 0 {
		t.Fatalf("unexpected current package groups: %v", groups)
	}
	data, err := os.ReadFile(filepath.Join(root, "config/shared/mise.toml"))
	if err != nil {
		t.Fatal(err)
	}
	var pinned []string
	inTools := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			inTools = line == "[tools]"
		} else if inTools && strings.Contains(line, "=") {
			name, _, _ := strings.Cut(line, "=")
			pinned = append(pinned, strings.TrimSpace(name))
		}
	}
	slices.Sort(pinned)
	if got := strings.Join(pinned, " "); got != "bat eza fzf gh jq lazygit neovim node python ripgrep starship tree-sitter uv zoxide" {
		t.Fatalf("mise.toml membership: %q", got)
	}
	for _, scenario := range []string{"Darwin", "Linux", "LinuxWSL"} {
		t.Run(scenario, func(t *testing.T) {
			home := t.TempDir()
			isolateHome(t, home)
			t.Setenv("TMPDIR", t.TempDir())
			for _, name := range []string{"MISE_DATA_DIR", "MISE_CACHE_DIR", "MISE_STATE_DIR", "MISE_CONFIG_DIR"} {
				t.Setenv(name, filepath.Join(home, "mise", name))
			}
			bin := filepath.Join(home, "safe-bin")
			if err := os.Mkdir(bin, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("/usr/bin/cksum", filepath.Join(bin, "cksum")); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin)
			system := scenario
			if scenario == "LinuxWSL" {
				system = "Linux"
			}
			t.Setenv("SELFISHELL_TEST_SYSTEM_NAME", system)
			t.Setenv("SELFISHELL_TEST_MACHINE_ARCH", "arm64")
			t.Setenv("SELFISHELL_TEST_OS_RELEASE_FILE", filepath.Join(home, "os-release"))
			t.Setenv("SELFISHELL_TEST_PROC_VERSION_FILE", filepath.Join(home, "proc-version"))
			if err := os.WriteFile(filepath.Join(home, "os-release"), []byte("ID=ubuntu\n"), 0600); err != nil {
				t.Fatal(err)
			}
			proc := "Linux version\n"
			if scenario == "LinuxWSL" {
				proc = "Linux microsoft WSL2\n"
			}
			if err := os.WriteFile(filepath.Join(home, "proc-version"), []byte(proc), 0600); err != nil {
				t.Fatal(err)
			}
			if system == "Darwin" {
				state := filepath.Join(home, ".local/state/selfishell")
				if err := os.MkdirAll(state, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(state, "ghostty"), []byte("1\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			var out, stderr bytes.Buffer
			code := (CLI{Root: root, In: strings.NewReader(""), Out: &out, Err: &stderr}).Run([]string{"install", "--dry-run"})
			if code != 0 {
				t.Fatalf("dry-run %d: %s", code, stderr.String())
			}
			plan := out.String()
			for _, tc := range []struct{ prefix, want string }{
				{"Would sync required mise tools: ", "fzf gh jq lazygit neovim node python ripgrep starship tree-sitter uv zoxide"},
				{"Would sync optional mise tools: ", "bat eza"},
			} {
				var names []string
				for _, line := range strings.Split(plan, "\n") {
					if strings.HasPrefix(line, tc.prefix) {
						for _, pin := range strings.Fields(strings.TrimPrefix(line, tc.prefix)) {
							name, _, ok := strings.Cut(pin, "@")
							if !ok {
								t.Fatalf("unversioned mise plan pin %q", pin)
							}
							names = append(names, name)
						}
					}
				}
				slices.Sort(names)
				if got := strings.Join(names, " "); got != tc.want {
					t.Fatalf("%s: %q want %q", tc.prefix, got, tc.want)
				}
			}
			for _, text := range []string{"Would sync required direct package: mise", "Would sync required direct package: zinit", "Would sync required mise tools: starship@", "Would sync optional mise tools: eza@", " bat@", "Would sync declared Neovim plugins."} {
				if !strings.Contains(plan, text) {
					t.Fatalf("missing %q in %s", text, plan)
				}
			}
			if system == "Darwin" {
				for _, text := range []string{"Would install optional Homebrew cask: font-meslo-lg-nerd-font font-noto-sans-cjk-kr", "Would install optional Homebrew cask: ghostty"} {
					if !strings.Contains(plan, text) {
						t.Fatalf("missing %q in %s", text, plan)
					}
				}
			} else if !strings.Contains(plan, "Would install required apt packages: zsh git curl ca-certificates vim build-essential") {
				t.Fatal(plan)
			}
			if _, err := os.Lstat(filepath.Join(home, "mise")); !os.IsNotExist(err) {
				t.Fatalf("dry-run invoked mise: %v", err)
			}
		})
	}
}
