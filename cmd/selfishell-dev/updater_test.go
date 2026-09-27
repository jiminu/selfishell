package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const oldC = "1111111111111111111111111111111111111111"
const newC = "2222222222222222222222222222222222222222"
const oldF = "3333333333333333333333333333333333333333"
const newF = "4444444444444444444444444444444444444444"
const oldA = "5555555555555555555555555555555555555555"
const completion = "if [[ -s \"$ZINIT_HOME/zinit.zsh\" ]]; then\n  source \"$ZINIT_HOME/zinit.zsh\"\n  zinit ice blockf atpull'zinit creinstall -q .' ver'" + oldC + "'\n  zinit light zsh-users/zsh-completions\nfi\n"
const interactive = "if (($+functions[zinit])); then\n  if command -v fzf >/dev/null 2>&1; then\n    zinit ice ver'" + oldF + "'\n    zinit light Aloxaf/fzf-tab\n  fi\n  zinit ice wait'0' lucid ver'" + oldA + "'\n  zinit light zsh-users/zsh-autosuggestions\nfi\n"
const mise = "[tools]\nstarship = \"1.25.0\"\nfzf = \"0.74.3\"\nzoxide = \"0.9.8\"\nripgrep = \"15.1.0\"\neza = \"0.23.4\"\nbat = \"0.26.0\"\njq = \"1.8.1\"\nnode = \"24.18.0\"\npython = \"3.13.14\"\nneovim = \"0.12.4\"\ntree-sitter = \"0.26.11\"\nuv = \"0.5.21\"\nlazygit = \"0.65.0\"\n\n[settings]\nnot_found_auto_install = false\n"

func zshLine(repo, sha string) string {
	return "zsh-plugin " + repo + " " + sha + " all all https://github.com/" + repo + ".git - - -\n"
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
	name, manifest, metadata, completion, interactive, mise string
	wantManifest, wantCompletion, wantInteractive, wantMise string
	failure, rerun                                          bool
}

func TestUpdaterCases(t *testing.T) {
	cLine := zshLine("zsh-users/zsh-completions", oldC)
	fLine := zshLine("Aloxaf/fzf-tab", oldF)
	aLine := zshLine("zsh-users/zsh-autosuggestions", oldA)
	base := []updaterCase{
		{name: "test_updates_only_matching_manifest_fields", manifest: "# type name version platform architecture source checksum target marker\ndownload mise 1.0.0 linux amd64 https://old/mise-amd64 oldsum .local/bin/mise raw\ndownload mise 1.0.0 linux arm64 https://old/mise oldmise .local/bin/mise raw\ngit zinit v0.1.0 all all https://github.com/zdharma-continuum/zinit.git - .local/share/zinit/zinit.git zinit.zsh\nnvim-plugin folke/lazy.nvim " + oldC + " all all https://github.com/folke/lazy.nvim.git - - -\n" + cLine, metadata: "download mise 2.0.0 linux amd64 https://new/mise-amd64 newsum\ndownload mise 2.0.0 linux arm64 https://new/mise newmise\ngit zinit v0.2.0 " + oldF + "\nnvim-plugin folke/lazy.nvim " + newC + "\nzsh-plugin zsh-users/zsh-completions " + newC + "\n", wantManifest: "# type name version platform architecture source checksum target marker\ndownload mise 2.0.0 linux amd64 https://new/mise-amd64 newsum .local/bin/mise raw\ndownload mise 2.0.0 linux arm64 https://new/mise newmise .local/bin/mise raw\ngit zinit v0.2.0 all all https://github.com/zdharma-continuum/zinit.git " + oldF + " .local/share/zinit/zinit.git zinit.zsh\nnvim-plugin folke/lazy.nvim " + newC + " all all https://github.com/folke/lazy.nvim.git - - -\n" + zshLine("zsh-users/zsh-completions", newC), wantCompletion: replace(completion, oldC, newC)},
		{name: "test_rejects_metadata_without_manifest_entry", manifest: "git zinit v0.1.0 all all https://example.invalid/zinit.git - .zinit zinit.zsh\n", metadata: "git missing v1.0.0\n", failure: true},
		{name: "test_zsh_plugin_update_rewrites_manifest_and_pin_file", manifest: cLine + fLine + aLine, metadata: "zsh-plugin zsh-users/zsh-completions " + newC + "\nzsh-plugin Aloxaf/fzf-tab " + newF + "\n", wantManifest: zshLine("zsh-users/zsh-completions", newC) + zshLine("Aloxaf/fzf-tab", newF) + aLine, wantCompletion: replace(completion, oldC, newC), wantInteractive: replace(interactive, oldF, newF)},
		{name: "test_zsh_plugin_update_fails_when_target_pin_missing", manifest: zshLine("zsh-users/zsh-completions", strings.Repeat("9", 40)), metadata: "zsh-plugin zsh-users/zsh-completions " + newC + "\n", failure: true},
		{name: "test_zsh_plugin_update_fails_when_manifest_entry_missing", metadata: "zsh-plugin zsh-users/zsh-completions " + newC + "\n", failure: true},
		{name: "test_zsh_plugin_update_rejects_invalid_commit_format", manifest: cLine, metadata: "zsh-plugin zsh-users/zsh-completions NOT-A-VALID-SHA\n", failure: true},
		{name: "test_zsh_plugin_update_rejects_duplicate_pin_matches", manifest: cLine, metadata: "zsh-plugin zsh-users/zsh-completions " + newC + "\n", completion: "zinit ice blockf ver'" + oldC + "'\nzinit light zsh-users/zsh-completions\n# Accidentally duplicated pin comment: ver'" + oldC + "'\n", failure: true},
		{name: "test_zsh_plugin_update_is_all_or_nothing", manifest: cLine + zshLine("Aloxaf/fzf-tab", strings.Repeat("9", 40)), metadata: "zsh-plugin zsh-users/zsh-completions " + newC + "\nzsh-plugin Aloxaf/fzf-tab " + newF + "\n", failure: true},
		{name: "test_mise_tool_update_bumps_pin_in_mise_toml_only", metadata: "mise-tool neovim 0.12.5\n", wantMise: replace(mise, "neovim = \"0.12.4\"", "neovim = \"0.12.5\"")},
		{name: "test_mise_tool_update_bumps_moved_cli_pin", metadata: "mise-tool fzf 0.74.4\nmise-tool starship 1.26.0\nmise-tool lazygit 0.65.1\n", wantMise: replace(replace(replace(mise, "fzf = \"0.74.3\"", "fzf = \"0.74.4\""), "starship = \"1.25.0\"", "starship = \"1.26.0\""), "lazygit = \"0.65.0\"", "lazygit = \"0.65.1\"")},
		{name: "test_mise_tool_update_compares_versions_numerically", metadata: "mise-tool uv 0.12.3\n", wantMise: replace(mise, "uv = \"0.5.21\"", "uv = \"0.12.3\"")},
		{name: "test_mise_tool_update_skips_same_or_older_candidate", metadata: "mise-tool neovim 0.12.4\nmise-tool uv 0.5.20\n"},
		{name: "test_mise_tool_update_rejects_non_stable_candidate_format", metadata: "mise-tool neovim nightly\n", failure: true},
		{name: "test_mise_tool_update_is_idempotent_on_rerun", metadata: "mise-tool neovim 0.12.5\n", wantMise: replace(mise, "neovim = \"0.12.4\"", "neovim = \"0.12.5\""), rerun: true},
	}
	for _, tc := range base {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			home := filepath.Join(root, "home")
			os.MkdirAll(home, 0700)
			manifest := filepath.Join(root, "dependencies.conf")
			metadata := filepath.Join(root, "metadata")
			zroot := filepath.Join(root, "zsh-root")
			cp := filepath.Join(zroot, "config/shared/zsh/completion.zsh")
			ip := filepath.Join(zroot, "config/shared/zsh/interactive.zsh")
			mp := filepath.Join(zroot, "config/shared/mise.toml")
			other := filepath.Join(zroot, "config/shared/zsh/other.zsh")
			originalC := completion
			if tc.completion != "" {
				originalC = tc.completion
			}
			originalI := interactive
			if tc.interactive != "" {
				originalI = tc.interactive
			}
			originalM := mise
			if tc.mise != "" {
				originalM = tc.mise
			}
			put(t, manifest, tc.manifest)
			put(t, metadata, tc.metadata)
			put(t, cp, originalC)
			put(t, ip, originalI)
			put(t, mp, originalM)
			put(t, other, "unrelated fixture content\n")
			wantManifest := tc.manifest
			if tc.wantManifest != "" {
				wantManifest = tc.wantManifest
			}
			wantC := originalC
			if tc.wantCompletion != "" {
				wantC = tc.wantCompletion
			}
			wantI := originalI
			if tc.wantInteractive != "" {
				wantI = tc.wantInteractive
			}
			wantM := originalM
			if tc.wantMise != "" {
				wantM = tc.wantMise
			}
			run := func() {
				t.Helper()
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, "bash", "../../scripts/update-dependencies.sh", "--manifest", manifest, "--metadata", metadata, "--zsh-root", zroot)
				cmd.Env = []string{"HOME=" + home, "XDG_CONFIG_HOME=" + filepath.Join(home, "config"), "XDG_DATA_HOME=" + filepath.Join(home, "data"), "XDG_STATE_HOME=" + filepath.Join(home, "state"), "XDG_CACHE_HOME=" + filepath.Join(home, "cache"), "MISE_DATA_DIR=" + filepath.Join(home, "mise-data"), "TMPDIR=" + root, "PATH=/usr/bin:/bin:/usr/sbin:/sbin"}
				out, err := cmd.CombinedOutput()
				if (err != nil) != tc.failure {
					t.Fatalf("failure=%v err=%v output=%s", tc.failure, err, out)
				}
				if ctx.Err() != nil {
					t.Fatal(ctx.Err())
				}
			}
			run()
			exact(t, manifest, wantManifest)
			exact(t, cp, wantC)
			exact(t, ip, wantI)
			exact(t, mp, wantM)
			exact(t, other, "unrelated fixture content\n")
			if tc.rerun {
				run()
				exact(t, manifest, wantManifest)
				exact(t, cp, wantC)
				exact(t, ip, wantI)
				exact(t, mp, wantM)
			}
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
out=""; url=""
while (($#)); do
  if [[ "$1" == -o ]]; then shift; out="$1"; else url="$1"; fi
  shift
done
if [[ -n "$out" ]]; then printf archive-bytes >"$out"; exit 0; fi
case "$url" in
  */jqlang/jq/releases/latest) printf '{"tag_name":"jq-2.0.0"}\n' ;;
  */zdharma-continuum/zinit/releases/latest) printf '{"tag_name":"v1.2.3"}\n' ;;
  *) printf '{"tag_name":"v1.2.3"}\n' ;;
esac
`
	git := `#!/bin/bash
printf '%s\n' "$*" >>"$GIT_LOG"
case "$*" in
  *zdharma-continuum/zinit.git*) printf 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\trefs/tags/v1.2.3\nbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\trefs/tags/v1.2.3^{}\n' ;;
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
	manifest := filepath.Join(root, "dependencies.conf")
	zroot := filepath.Join(root, "zsh-root")
	var lines strings.Builder
	lines.WriteString("git zinit v0.1.0 all all https://github.com/zdharma-continuum/zinit.git - .local/share/zinit/zinit.git zinit.zsh\n")
	lines.WriteString("nvim-plugin folke/lazy.nvim " + oldC + " all all https://github.com/folke/lazy.nvim.git - - -\n")
	lines.WriteString(zshLine("zsh-users/zsh-completions", oldC))
	for _, platform := range []string{"linux", "macos"} {
		for _, arch := range []string{"amd64", "arm64"} {
			lines.WriteString("download mise 0.1.0 " + platform + " " + arch + " https://old.invalid/mise oldsum .local/bin/mise raw\n")
		}
	}
	put(t, manifest, lines.String())
	put(t, filepath.Join(zroot, "config/shared/zsh/completion.zsh"), completion)
	put(t, filepath.Join(zroot, "config/shared/mise.toml"), replace(mise, "lazygit = \"0.65.0\"", "lazygit = \"0.65.0\"\ngh = \"2.0.0\""))
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "../../scripts/update-dependencies.sh", "--manifest", manifest, "--zsh-root", zroot)
	cmd.Env = []string{"HOME=" + home, "XDG_CONFIG_HOME=" + filepath.Join(home, "config"), "XDG_DATA_HOME=" + filepath.Join(home, "data"), "XDG_STATE_HOME=" + filepath.Join(home, "state"), "XDG_CACHE_HOME=" + filepath.Join(home, "cache"), "MISE_DATA_DIR=" + filepath.Join(home, "mise-data"), "TMPDIR=" + root, "PATH=" + fakebin + ":" + os.Getenv("PATH"), "GOTOOLCHAIN=local", "GOPROXY=off", "GOSUMDB=off", "GOFLAGS=-buildvcs=false", "DISCOVERY_LOG=" + log, "GIT_LOG=" + filepath.Join(root, "git.log"), "GH_TOKEN=private-secret"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("discovery: %v: %s", err, out)
	}
	result, _ := os.ReadFile(manifest)
	got := string(result)
	for _, want := range []string{"git zinit v1.2.3 all all https://github.com/zdharma-continuum/zinit.git bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "nvim-plugin folke/lazy.nvim cccccccccccccccccccccccccccccccccccccccc", "zsh-plugin zsh-users/zsh-completions cccccccccccccccccccccccccccccccccccccccc", "download mise 1.2.3 linux amd64 https://github.com/jdx/mise/releases/download/v1.2.3/mise-v1.2.3-linux-x64"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %s", want, got)
		}
	}
	gitlog, _ := os.ReadFile(filepath.Join(root, "git.log"))
	if !strings.Contains(string(gitlog), "refs/tags/v1.2.3^{}") {
		t.Fatalf("annotated tag not peeled: %s", gitlog)
	}
	calls, _ := os.ReadFile(log)
	if !strings.Contains(string(calls), "Authorization: Bearer private-secret") || !strings.Contains(string(calls), "Accept: application/vnd.github+json") || !strings.Contains(string(calls), "X-GitHub-Api-Version: 2022-11-28") || !strings.Contains(string(calls), "--max-time 15") || !strings.Contains(string(calls), "--speed-limit 1024") {
		t.Fatalf("transport policy missing: %s", calls)
	}
	if strings.Contains(string(out), "private-secret") {
		t.Fatal("secret in diagnostics")
	}
	exact(t, filepath.Join(zroot, "config/shared/zsh/completion.zsh"), replace(completion, oldC, strings.Repeat("c", 40)))
}
