package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jiminu/selfishell/internal/selfishell"
)

func discoverDependencyUpdates(ctx context.Context, p selfishell.Process, dependencies []selfishell.Dependency) ([][]string, error) {
	temporary, err := os.MkdirTemp("", "selfishell-dependency-discovery-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(temporary)
	var metadata strings.Builder
	record := func(fields ...string) { fmt.Fprintln(&metadata, strings.Join(fields, " ")) }
	latestTag := func(repository string) (string, error) {
		args := []string{"-H", "Accept: application/vnd.github+json", "-H", "X-GitHub-Api-Version: 2022-11-28"}
		for _, value := range p.Env {
			if token, ok := strings.CutPrefix(value, "GH_TOKEN="); ok && token != "" {
				args = append(args, "-H", "Authorization: Bearer "+token)
			}
		}
		args = append(args, "https://api.github.com/repos/"+repository+"/releases/latest")
		var output bytes.Buffer
		transport := p
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
	// Node and Python release lines remain an explicit maintainer choice.
	for _, tool := range [][2]string{
		{"starship", "starship/starship"}, {"fzf", "junegunn/fzf"},
		{"zoxide", "ajeetdsouza/zoxide"}, {"ripgrep", "BurntSushi/ripgrep"},
		{"eza", "eza-community/eza"}, {"bat", "sharkdp/bat"}, {"jq", "jqlang/jq"},
		{"neovim", "neovim/neovim"}, {"tree-sitter", "tree-sitter/tree-sitter"},
		{"uv", "astral-sh/uv"}, {"gh", "cli/cli"}, {"lazygit", "jesseduffield/lazygit"},
	} {
		tag, err := latestTag(tool[1])
		if err != nil {
			return nil, err
		}
		record("mise-tool", tool[0], strings.TrimPrefix(strings.TrimPrefix(tag, "v"), "jq-"))
	}
	return parseDependencyMetadata(metadata.String())
}
