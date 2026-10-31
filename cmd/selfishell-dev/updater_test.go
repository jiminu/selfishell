package main

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jiminu/selfishell/internal/selfishell"
	"github.com/jiminu/selfishell/internal/testutil"
)

const oldC = "1111111111111111111111111111111111111111"
const newC = "2222222222222222222222222222222222222222"
const oldF = "3333333333333333333333333333333333333333"
const newF = "4444444444444444444444444444444444444444"
const mise = "[tools]\nstarship = \"1.25.0\"\nfzf = \"0.74.3\"\nzoxide = \"0.9.8\"\nripgrep = \"15.1.0\"\neza = \"0.23.4\"\nbat = \"0.26.0\"\njq = \"1.8.1\"\nnode = \"24.18.0\"\npython = \"3.13.14\"\nneovim = \"0.12.4\"\ntree-sitter = \"0.26.11\"\nuv = \"0.5.21\"\nlazygit = \"0.65.0\"\n\n[settings]\nnot_found_auto_install = false\n"

// gitLine is a git record that follows its default branch, pinned by commit.
func gitLine(name, sha string) string {
	return "git " + name + " " + sha + " all all https://github.com/test/" + name + ".git " + sha + " .local/share/selfishell/zsh-plugins/" + name + " " + name + ".plugin.zsh\n"
}
func replace(s, old, next string) string { return strings.Replace(s, old, next, 1) }
func put(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
func exact(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Fatalf("%s: got %q (%v), want %q", path, got, err, want)
	}
}

type updaterCase struct {
	name, manifest, metadata, mise string
	wantManifest, wantMise         string
	failure                        bool
}

func TestUpdaterCases(t *testing.T) {
	cLine := gitLine("zsh-completions", oldC)
	fLine := gitLine("fzf-tab", oldF)
	base := []updaterCase{
		{name: "empty_metadata_preserves_manifest", manifest: "# retained comment\n" + cLine},
		{name: "pin_replacement_is_literal", metadata: "mise-tool uv 0.12.3\n", mise: mise + "# uv = \"0x5x21\"\n", wantMise: replace(mise, "uv = \"0.5.21\"", "uv = \"0.12.3\"") + "# uv = \"0x5x21\"\n"},
		{name: "invalid_mise_bump_preserves_staged_plugin", manifest: cLine, metadata: "git zsh-completions " + newC + " " + newC + "\nmise-tool neovim nightly\n", failure: true},
		{name: "unknown_metadata", manifest: cLine, metadata: "unknown item value\n", failure: true},
		{name: "duplicate_metadata", manifest: cLine, metadata: "git zsh-completions " + newC + " " + newC + "\ngit zsh-completions " + newC + " " + newC + "\n", failure: true},
		{name: "short_download_metadata", metadata: "download mise 1.2.3 linux amd64\n", failure: true},
		{name: "malformed_download_checksum", manifest: "download mise 1.0.0 linux amd64 https://old.invalid/mise oldsum .local/bin/mise raw\n", metadata: "download mise 2.0.0 linux amd64 https://new.invalid/mise not-a-sha256\n", failure: true},
		{name: "missing_tool_pin", metadata: "mise-tool missing 1.2.3\n", failure: true},
		{name: "large_numeric_version", metadata: "mise-tool uv 999999999999999999999.1\n", wantMise: replace(mise, "uv = \"0.5.21\"", "uv = \"999999999999999999999.1\"")},
		{name: "test_updates_only_matching_manifest_fields", manifest: "# type name version platform architecture source checksum target marker\ndownload mise 1.0.0 linux amd64 https://old/mise-amd64 oldsum .local/bin/mise raw\ndownload mise 1.0.0 linux arm64 https://old/mise oldmise .local/bin/mise raw\ngit tool v0.1.0 all all https://github.com/test/tool.git - .local/share/tool tool.zsh\nnvim-plugin folke/lazy.nvim " + oldC + " all all https://github.com/folke/lazy.nvim.git - - -\n" + cLine, metadata: "download mise 2.0.0 linux amd64 https://new/mise-amd64 7777777777777777777777777777777777777777777777777777777777777777\ndownload mise 2.0.0 linux arm64 https://new/mise 8888888888888888888888888888888888888888888888888888888888888888\ngit tool v0.2.0 " + oldF + "\nnvim-plugin folke/lazy.nvim " + newC + "\n", wantManifest: "# type name version platform architecture source checksum target marker\ndownload mise 2.0.0 linux amd64 https://new/mise-amd64 7777777777777777777777777777777777777777777777777777777777777777 .local/bin/mise raw\ndownload mise 2.0.0 linux arm64 https://new/mise 8888888888888888888888888888888888888888888888888888888888888888 .local/bin/mise raw\ngit tool v0.2.0 all all https://github.com/test/tool.git " + oldF + " .local/share/tool tool.zsh\nnvim-plugin folke/lazy.nvim " + newC + " all all https://github.com/folke/lazy.nvim.git - - -\n" + cLine},
		{name: "test_rejects_metadata_without_manifest_entry", manifest: "git tool v0.1.0 all all https://example.invalid/tool.git - .tool tool.zsh\n", metadata: "git missing v1.0.0\n", failure: true},
		{name: "test_git_update_following_head_rewrites_version_and_checksum", manifest: cLine + fLine, metadata: "git zsh-completions " + newC + " " + newC + "\n", wantManifest: gitLine("zsh-completions", newC) + fLine},
		{name: "test_git_update_rejects_invalid_commit_format", manifest: cLine, metadata: "git zsh-completions " + newC + " NOT-A-VALID-SHA\n", failure: true},
		{name: "test_git_update_is_all_or_nothing", manifest: cLine, metadata: "git zsh-completions " + newC + " " + newC + "\ngit fzf-tab " + newF + " " + newF + "\n", failure: true},
		{name: "test_mise_tool_update_bumps_moved_cli_pin", metadata: "mise-tool fzf 0.74.4\nmise-tool starship 1.26.0\nmise-tool lazygit 0.65.1\n", wantMise: replace(replace(replace(mise, "fzf = \"0.74.3\"", "fzf = \"0.74.4\""), "starship = \"1.25.0\"", "starship = \"1.26.0\""), "lazygit = \"0.65.0\"", "lazygit = \"0.65.1\"")},
		{name: "test_mise_tool_update_skips_same_or_older_candidate", metadata: "mise-tool neovim 0.12.4\nmise-tool uv 0.5.20\n"},
	}
	for _, tc := range base {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			home := filepath.Join(root, "home")
			os.MkdirAll(home, 0700)
			manifest := filepath.Join(root, "dependencies.conf")
			metadata := filepath.Join(root, "metadata")
			zroot := filepath.Join(root, "zsh-root")
			mp := filepath.Join(zroot, "config/shared/mise.toml")
			other := filepath.Join(zroot, "config/shared/zsh/other.zsh")
			originalM := mise
			if tc.mise != "" {
				originalM = tc.mise
			}
			put(t, manifest, tc.manifest)
			put(t, metadata, tc.metadata)
			put(t, mp, originalM)
			put(t, other, "unrelated fixture content\n")
			wantManifest := tc.manifest
			if tc.wantManifest != "" {
				wantManifest = tc.wantManifest
			}
			wantM := originalM
			if tc.wantMise != "" {
				wantM = tc.wantMise
			}
			run := func() {
				t.Helper()
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				var output bytes.Buffer
				p := selfishell.Process{Out: &output, Err: &output, Env: []string{"HOME=" + home, "PATH=" + filepath.Join(root, "no-tools")}}
				status := runDependencyUpdate(ctx, root, []string{"--manifest", manifest, "--metadata", metadata, "--zsh-root", zroot}, p)
				if (status != 0) != tc.failure {
					t.Fatalf("failure=%v status=%d output=%s", tc.failure, status, output.String())
				}
				if ctx.Err() != nil {
					t.Fatal(ctx.Err())
				}
			}
			run()
			exact(t, manifest, wantManifest)
			exact(t, mp, wantM)
			exact(t, other, "unrelated fixture content\n")
		})
	}
}

func TestUpdaterDiscoveryUsesPrivateTransportAndPeeledGitTag(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	os.MkdirAll(home, 0700)
	fakebin := filepath.Join(root, "fakebin")
	os.MkdirAll(fakebin, 0700)
	log := filepath.Join(root, "curl.log")
	curl := `#!/bin/bash
printf '%s\n' "$*" >>"$DISCOVERY_LOG"
if [[ "${DISCOVERY_FAILURE:-}" == curl ]]; then printf 'private-secret' >&2; exit 22; fi
out=""; url=""
while (($#)); do
  if [[ "$1" == -o ]]; then shift; out="$1"
  elif [[ "$1" == -H && "$2" == @- ]]; then cat >>"$AUTH_LOG"; shift
  else url="$1"; fi
  shift
done
case "$url" in
  */releases/latest) [[ -z "$out" ]] || exec >"$out" ;;
  *)
    [[ "${DISCOVERY_FAILURE:-}" != download ]] || exit 22
    if [[ "$url" == */registry.json.zip ]]; then
      [[ "${DISCOVERY_FAILURE:-}" != registry ]] || { printf broken >"$out"; exit 0; }
      cp "$REGISTRY_ZIP" "$out"; exit 0
    fi
    printf archive-bytes >"$out"; exit 0 ;;
esac
if [[ "${DISCOVERY_FAILURE:-}" == json ]]; then printf '{bad json'; exit 0; fi
case "$url" in
  */jqlang/jq/releases/latest) printf '{"tag_name":"jq-2.0.0"}\n' ;;
  */saghen/blink.cmp/releases/latest) printf '{"tag_name":"v4.5.6"}\n' ;;
  *) printf '{"tag_name":"v1.2.3"}\n' ;;
esac
`
	git := `#!/bin/bash
printf '%s\n' "$*" >>"$GIT_LOG"
if [[ "${DISCOVERY_FAILURE:-}" == git ]]; then printf 'invalid\tHEAD\n'; exit 0; fi
case "$*" in
  *saghen/blink.cmp.git*refs/tags/v4.5.6*) printf 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\trefs/tags/v4.5.6\ndddddddddddddddddddddddddddddddddddddddd\trefs/tags/v4.5.6^{}\n' ;;
  *) printf 'cccccccccccccccccccccccccccccccccccccccc\tHEAD\n' ;;
esac
`
	put(t, filepath.Join(fakebin, "curl"), curl)
	put(t, filepath.Join(fakebin, "git"), git)
	for _, name := range []string{"curl", "git"} {
		if err := os.Chmod(filepath.Join(fakebin, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"jq", "sha256sum", "shasum"} {
		path := filepath.Join(fakebin, name)
		put(t, path, "#!/bin/sh\nexit 99\n")
		if err := os.Chmod(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	manifest := filepath.Join(root, "dependencies.conf")
	zroot := filepath.Join(root, "zsh-root")
	var lines strings.Builder
	lines.WriteString("nvim-plugin folke/lazy.nvim " + oldC + " all all https://github.com/folke/lazy.nvim.git - - -\n")
	lines.WriteString("nvim-plugin saghen/blink.cmp " + oldC + " all all https://github.com/saghen/blink.cmp.git - - -\n")
	lines.WriteString(gitLine("zsh-completions", oldC))
	for _, platform := range []string{"linux", "macos"} {
		for _, arch := range []string{"amd64", "arm64"} {
			lines.WriteString("download mise 0.1.0 " + platform + " " + arch + " https://old.invalid/mise oldsum .local/bin/mise raw\n")
		}
	}
	put(t, manifest, lines.String())
	put(t, filepath.Join(zroot, "config/shared/mise.toml"), replace(mise, "lazygit = \"0.65.0\"", "lazygit = \"0.65.0\"\ngh = \"2.0.0\""))
	languages := "local M = {\n  lsp = {\n    \"lua_ls@3.19.1\",\n    \"tombi@v1.5.5\",\n  },\n}\nreturn M\n"
	languagePath := filepath.Join(zroot, "config/shared/nvim/lua/config/languages.lua")
	put(t, languagePath, languages)
	registryZip := filepath.Join(root, "registry.json.zip")
	writeRegistryZip(t, registryZip, `[{"name":"lua-language-server","source":{"id":"pkg:github/LuaLS/lua-language-server@3.20.0"},"neovim":{"lspconfig":"lua_ls"}},{"name":"tombi","source":{"id":"pkg:github/tombi-toml/tombi@v1.5.6"},"neovim":{"lspconfig":"tombi"}}]`)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "../../scripts/update-dependencies.sh", "--manifest", manifest, "--zsh-root", zroot)
	cmd.Env = []string{"HOME=" + home, "XDG_CONFIG_HOME=" + filepath.Join(home, "config"), "XDG_DATA_HOME=" + filepath.Join(home, "data"), "XDG_STATE_HOME=" + filepath.Join(home, "state"), "XDG_CACHE_HOME=" + filepath.Join(home, "cache"), "MISE_DATA_DIR=" + filepath.Join(home, "mise-data"), "TMPDIR=" + root, "PATH=" + fakebin + ":" + os.Getenv("PATH"), "GOTOOLCHAIN=local", "GOCACHE=" + testutil.GoCache(t), "GOPROXY=off", "GOSUMDB=off", "GOFLAGS=-buildvcs=false", "DISCOVERY_LOG=" + log, "AUTH_LOG=" + filepath.Join(root, "auth.log"), "GIT_LOG=" + filepath.Join(root, "git.log"), "REGISTRY_ZIP=" + registryZip, "GH_TOKEN=private-secret"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("discovery: %v: %s", err, out)
	}
	result, _ := os.ReadFile(manifest)
	got := string(result)
	if count := strings.Count(got, " 0c982986710a026635603031674053ca851fc0e3ea760094a34f59b84f7f6da6 .local/bin/mise raw"); count != 4 {
		t.Fatalf("expected verified checksums for all four downloads, got %d: %s", count, got)
	}
	for _, want := range []string{gitLine("zsh-completions", strings.Repeat("c", 40)), "nvim-plugin folke/lazy.nvim cccccccccccccccccccccccccccccccccccccccc", "nvim-plugin saghen/blink.cmp dddddddddddddddddddddddddddddddddddddddd", "download mise 1.2.3 linux amd64 https://github.com/jdx/mise/releases/download/v1.2.3/mise-v1.2.3-linux-x64"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %s", want, got)
		}
	}
	gitlog, _ := os.ReadFile(filepath.Join(root, "git.log"))
	if !strings.Contains(string(gitlog), "refs/tags/v4.5.6^{}") {
		t.Fatalf("annotated tag not peeled: %s", gitlog)
	}
	calls, _ := os.ReadFile(log)
	if strings.Contains(string(calls), "private-secret") {
		t.Fatal("GitHub token exposed in curl arguments")
	}
	auth, _ := os.ReadFile(filepath.Join(root, "auth.log"))
	if !strings.Contains(string(auth), "Authorization: Bearer private-secret\n") || !strings.Contains(string(calls), "Accept: application/vnd.github+json") || !strings.Contains(string(calls), "X-GitHub-Api-Version: 2022-11-28") || !strings.Contains(string(calls), "--max-time 15") || !strings.Contains(string(calls), "--speed-limit 1024") {
		t.Fatalf("transport policy missing: %s", calls)
	}
	if strings.Contains(string(out), "private-secret") {
		t.Fatal("secret in diagnostics")
	}
	exact(t, languagePath, strings.Replace(strings.Replace(languages, "lua_ls@3.19.1", "lua_ls@3.20.0", 1), "tombi@v1.5.5", "tombi@v1.5.6", 1))
	for _, failure := range []string{"curl", "json", "git", "download", "registry"} {
		t.Run(failure+" preserves files", func(t *testing.T) {
			paths := []string{manifest, filepath.Join(zroot, "config/shared/mise.toml"), languagePath}
			before := make(map[string]string)
			for _, path := range paths {
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				before[path] = string(data)
			}
			var output bytes.Buffer
			p := selfishell.Process{Out: &output, Err: &output, Env: append(append([]string{}, cmd.Env...), "DISCOVERY_FAILURE="+failure)}
			if status := runDependencyUpdate(ctx, root, []string{"--manifest", manifest, "--zsh-root", zroot}, p); status == 0 || strings.Contains(output.String(), "private-secret") {
				t.Fatalf("discovery failure status=%d output=%s", status, output.String())
			}
			for path, data := range before {
				exact(t, path, data)
			}
		})
	}
}

func TestUpdaterArgumentsBeforeEffects(t *testing.T) {
	for _, args := range [][]string{{"--unknown"}, {"--manifest"}, {"--metadata"}, {"--zsh-root"}, {"--metadata", ""}, {"--manifest", ""}, {"unexpected"}, {"--help"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			root := t.TempDir()
			var output bytes.Buffer
			status := runDependencyUpdate(context.Background(), root, args, selfishell.Process{Out: &output, Err: &output})
			want := 2
			if args[0] == "--help" {
				want = 0
			}
			if status != want {
				t.Fatalf("status=%d output=%s", status, output.String())
			}
			entries, err := os.ReadDir(root)
			if err != nil || len(entries) != 0 {
				t.Fatalf("argument validation caused effects: %v %v", entries, err)
			}
		})
	}
}

func TestUpdaterPreflightsTargetsAndCancellation(t *testing.T) {
	for _, mode := range []string{"missing", "symlink", "directory", "canceled", "success"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			manifest := filepath.Join(root, "dependencies.conf")
			metadata := filepath.Join(root, "metadata")
			pin := filepath.Join(root, "config/shared/mise.toml")
			original := gitLine("zsh-completions", oldC)
			put(t, manifest, original)
			put(t, metadata, "git zsh-completions "+newC+" "+newC+"\nmise-tool uv 0.12.3\n")
			put(t, pin, mise)
			wantMode := os.FileMode(0640)
			if err := os.Chmod(manifest, wantMode); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch mode {
			case "missing", "symlink", "directory":
				if err := os.Remove(pin); err != nil {
					t.Fatal(err)
				}
				if mode == "symlink" {
					outside := filepath.Join(t.TempDir(), "user-file")
					put(t, outside, mise)
					if err := os.Symlink(outside, pin); err != nil {
						t.Fatal(err)
					}
					defer exact(t, outside, mise)
				} else if mode == "directory" {
					if err := os.Mkdir(pin, 0700); err != nil {
						t.Fatal(err)
					}
				}
			case "canceled":
				cancel()
			}
			var output bytes.Buffer
			status := runDependencyUpdate(ctx, root, []string{"--metadata", metadata}, selfishell.Process{Out: &output, Err: &output})
			success := mode == "success"
			if (status == 0) != success {
				t.Fatalf("status=%d output=%s", status, output.String())
			}
			if success {
				exact(t, manifest, gitLine("zsh-completions", newC))
				exact(t, pin, replace(mise, "uv = \"0.5.21\"", "uv = \"0.12.3\""))
			} else {
				exact(t, manifest, original)
				if mode == "canceled" {
					exact(t, pin, mise)
				}
			}
			info, err := os.Stat(manifest)
			if err != nil || info.Mode() != wantMode {
				t.Fatalf("manifest mode changed: %v %v", info, err)
			}
			if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
				if err == nil && strings.HasPrefix(entry.Name(), ".selfishell-dependency-update-") {
					t.Errorf("staging file retained: %s", path)
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// A mise tool added to packages.conf without a discovery source would silently
// never receive update proposals.
func TestMiseToolSourcesCoverDeclaredTools(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "packages.conf"))
	if err != nil {
		t.Fatal(err)
	}
	declared := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		if f := strings.Fields(line); len(f) == 5 && f[0] == "package" && f[3] == "mise" && f[4] != "node" && f[4] != "python" {
			declared[f[4]] = true
		}
	}
	sources := map[string]bool{}
	for _, tool := range miseToolSources {
		if sources[tool[0]] {
			t.Errorf("duplicate discovery source: %s", tool[0])
		}
		sources[tool[0]] = true
		if !declared[tool[0]] {
			t.Errorf("discovery source for undeclared mise tool: %s", tool[0])
		}
	}
	for tool := range declared {
		if !sources[tool] {
			t.Errorf("declared mise tool has no discovery source: %s", tool)
		}
	}
	if len(declared) == 0 {
		t.Fatal("no mise tools parsed from packages.conf")
	}
}
