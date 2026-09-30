package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jiminu/selfishell/internal/selfishell"
)

func discoverDependencyUpdates(ctx context.Context, p selfishell.Process, dependencies []selfishell.Dependency, root string) ([][]string, error) {
	language, err := os.ReadFile(filepath.Join(root, "config/shared/nvim/lua/config/languages.lua"))
	if err != nil {
		return nil, err
	}
	pins, err := lspPins(string(language))
	if err != nil {
		return nil, err
	}
	temporary, err := os.MkdirTemp("", "selfishell-dependency-discovery-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(temporary)
	var metadata strings.Builder
	record := func(fields ...string) { fmt.Fprintln(&metadata, strings.Join(fields, " ")) }
	mod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return nil, err
	}
	currentGo, err := goToolchainVersion(string(mod))
	if err != nil {
		return nil, err
	}
	var goReleases bytes.Buffer
	goTransport := p
	goTransport.Out, goTransport.Err = &goReleases, io.Discard
	if code, err := goTransport.Curl(ctx, "metadata", "https://go.dev/dl/?mode=json"); err != nil {
		return nil, err
	} else if code != 0 {
		return nil, fmt.Errorf("Go release lookup failed (exit %d)", code)
	}
	goPatch, err := latestGoPatch(goReleases.Bytes(), currentGo)
	if err != nil {
		return nil, err
	}
	record("go-toolchain", "go", goPatch)
	latestTag := func(repository string) (string, error) {
		args := []string{"-H", "Accept: application/vnd.github+json", "-H", "X-GitHub-Api-Version: 2022-11-28"}
		transport := p
		authorized := false
		for _, value := range p.Env {
			if token, ok := strings.CutPrefix(value, "GH_TOKEN="); ok && token != "" {
				if strings.ContainsAny(token, "\r\n\x00") {
					return "", fmt.Errorf("invalid GitHub token")
				}
				// curl reads the private header from stdin; credentials stay out of argv.
				transport.In = strings.NewReader("Authorization: Bearer " + token + "\n")
				authorized = true
			}
		}
		if authorized {
			args = append(args, "-H", "@-")
		}
		args = append(args, "https://api.github.com/repos/"+repository+"/releases/latest")
		var output bytes.Buffer
		transport.Out, transport.Err = &output, io.Discard
		if code, err := transport.Curl(ctx, "metadata", args...); err != nil {
			return "", err
		} else if code != 0 {
			return "", fmt.Errorf("release lookup for %s failed (exit %d)", repository, code)
		}
		var release struct {
			Tag string `json:"tag_name"`
		}
		if err := json.Unmarshal(output.Bytes(), &release); err != nil {
			return "", fmt.Errorf("invalid release metadata for %s: %w", repository, err)
		}
		if release.Tag == "" || strings.ContainsAny(release.Tag, " \t\r\n\x00") {
			return "", fmt.Errorf("invalid release tag for %s", repository)
		}
		return release.Tag, nil
	}
	gitCommit := func(source, ref string) (string, error) {
		args := []string{"ls-remote", "--", source, ref}
		if ref != "HEAD" {
			args = append(args, ref+"^{}")
		}
		var output bytes.Buffer
		git := p
		git.Out, git.Err = &output, io.Discard
		if code, err := git.Run(ctx, "git", args...); err != nil {
			return "", err
		} else if code != 0 {
			return "", fmt.Errorf("git ls-remote failed (exit %d)", code)
		}
		refs := map[string]string{}
		for _, line := range strings.Split(output.String(), "\n") {
			if fields := strings.Fields(line); len(fields) == 2 {
				refs[fields[1]] = fields[0]
			}
		}
		commit := refs[ref+"^{}"]
		if commit == "" {
			commit = refs[ref]
		}
		if !pluginCommit.MatchString(commit) {
			return "", fmt.Errorf("invalid Git commit for %s", ref)
		}
		return commit, nil
	}
	tag, err := latestTag("zdharma-continuum/zinit")
	if err != nil {
		return nil, err
	}
	commit, err := gitCommit("https://github.com/zdharma-continuum/zinit.git", "refs/tags/"+tag)
	if err != nil {
		return nil, err
	}
	record("git", "zinit", tag, commit)
	for _, dependency := range dependencies {
		if dependency.Kind != "nvim-plugin" && dependency.Kind != "zsh-plugin" {
			continue
		}
		commit, err := gitCommit(dependency.Source, "HEAD")
		if err != nil {
			return nil, err
		}
		record(dependency.Kind, dependency.Name, commit)
	}
	tag, err = latestTag("jdx/mise")
	if err != nil {
		return nil, err
	}
	version := strings.TrimPrefix(tag, "v")
	if !numericToolVersion.MatchString(version) {
		return nil, fmt.Errorf("invalid mise release version")
	}
	for _, platform := range []string{"linux", "macos"} {
		for _, arch := range []string{"amd64", "arm64"} {
			assetArch := arch
			if arch == "amd64" {
				assetArch = "x64"
			}
			source := "https://github.com/jdx/mise/releases/download/" + tag + "/mise-" + tag + "-" + platform + "-" + assetArch
			archive := filepath.Join(temporary, platform+"-"+arch)
			transport := p
			transport.Err = io.Discard
			if code, err := transport.Curl(ctx, "transfer", source, "-o", archive); err != nil {
				return nil, err
			} else if code != 0 {
				return nil, fmt.Errorf("mise download failed (exit %d)", code)
			}
			file, err := os.Open(archive)
			if err != nil {
				return nil, err
			}
			digest := sha256.New()
			_, err = io.Copy(digest, file)
			closeErr := file.Close()
			if err != nil {
				return nil, err
			}
			if closeErr != nil {
				return nil, closeErr
			}
			record("download", "mise", version, platform, arch, source, fmt.Sprintf("%x", digest.Sum(nil)))
		}
	}
	for _, tool := range miseToolSources {
		tag, err := latestTag(tool[1])
		if err != nil {
			return nil, err
		}
		record("mise-tool", tool[0], strings.TrimPrefix(strings.TrimPrefix(tag, "v"), "jq-"))
	}
	tag, err = latestTag("mason-org/mason-registry")
	if err != nil {
		return nil, err
	}
	archive := filepath.Join(temporary, "registry.json.zip")
	transport := p
	transport.Err = io.Discard
	url := "https://github.com/mason-org/mason-registry/releases/download/" + tag + "/registry.json.zip"
	if code, err := transport.Curl(ctx, "transfer", url, "-o", archive); err != nil {
		return nil, err
	} else if code != 0 {
		return nil, fmt.Errorf("Mason registry download failed (exit %d)", code)
	}
	lspUpdates, err := lspRegistryUpdates(archive, pins)
	if err != nil {
		return nil, err
	}
	for _, update := range lspUpdates {
		fmt.Fprintln(&metadata, update)
	}
	return parseDependencyMetadata(metadata.String())
}

// miseToolSources is each mise tool's upstream repository for release discovery.
// Node and Python release lines remain an explicit maintainer choice; a test
// keeps this list aligned with the mise tools declared in packages.conf.
var miseToolSources = [][2]string{
	{"starship", "starship/starship"}, {"fzf", "junegunn/fzf"},
	{"zoxide", "ajeetdsouza/zoxide"}, {"ripgrep", "BurntSushi/ripgrep"},
	{"eza", "eza-community/eza"}, {"bat", "sharkdp/bat"}, {"jq", "jqlang/jq"},
	{"neovim", "neovim/neovim"}, {"tree-sitter", "tree-sitter/tree-sitter"},
	{"uv", "astral-sh/uv"}, {"gh", "cli/cli"}, {"lazygit", "jesseduffield/lazygit"},
}

func lspRegistryUpdates(archive string, pins map[string]string) ([]string, error) {
	zipFile, err := zip.OpenReader(archive)
	if err != nil {
		return nil, err
	}
	defer zipFile.Close()
	if len(zipFile.File) != 1 || zipFile.File[0].Name != "registry.json" || zipFile.File[0].UncompressedSize64 > 10<<20 {
		return nil, fmt.Errorf("invalid Mason registry archive")
	}
	file, err := zipFile.File[0].Open()
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, 10<<20+1))
	err = file.Close()
	if readErr != nil {
		return nil, readErr
	}
	if err != nil {
		return nil, err
	}
	if len(data) > 10<<20 {
		return nil, fmt.Errorf("Mason registry too large")
	}
	var registry []struct {
		Source struct {
			ID string `json:"id"`
		} `json:"source"`
		Neovim struct {
			LSPConfig string `json:"lspconfig"`
		} `json:"neovim"`
	}
	if err := json.Unmarshal(data, &registry); err != nil {
		return nil, fmt.Errorf("invalid Mason registry: %w", err)
	}
	versions := map[string]string{}
	for _, entry := range registry {
		server := entry.Neovim.LSPConfig
		if pins[server] == "" {
			continue
		}
		at := strings.LastIndexByte(entry.Source.ID, '@')
		if at < 0 || !strings.HasPrefix(entry.Source.ID, "pkg:") || !lspVersion.MatchString(entry.Source.ID[at+1:]) || versions[server] != "" {
			return nil, fmt.Errorf("invalid or duplicate Mason registry version for %s", server)
		}
		versions[server] = entry.Source.ID[at+1:]
	}
	servers := make([]string, 0, len(pins))
	for server := range pins {
		if versions[server] == "" {
			return nil, fmt.Errorf("Mason registry missing %s", server)
		}
		servers = append(servers, server)
	}
	sort.Strings(servers)
	updates := make([]string, 0, len(servers))
	for _, server := range servers {
		updates = append(updates, "lsp-server "+server+" "+versions[server])
	}
	return updates, nil
}
