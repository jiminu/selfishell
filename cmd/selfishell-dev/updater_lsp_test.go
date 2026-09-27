package main

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiminu/selfishell/internal/selfishell"
)

const lspFixture = "local M = {\n  lsp = {\n    \"lua_ls@3.19.1\", -- keep\n    \"tombi@v1.5.5\",\n    \"marksman@2026-02-08\",\n  },\n  other = { \"lua_ls@0.0.1\" },\n}\nreturn M\n"

func TestUpdaterLSPMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, metadata, source, want string
		fail                         bool
	}{
		{"edit", "lsp-server lua_ls 3.20.0\n", lspFixture, strings.Replace(lspFixture, "lua_ls@3.19.1", "lua_ls@3.20.0", 1), false},
		{"same", "lsp-server tombi v1.5.5\n", lspFixture, lspFixture, false},
		{"older", "lsp-server marksman 2026-02-07\n", lspFixture, lspFixture, false},
		{"unknown", "lsp-server missing 1.0.0\n", lspFixture, lspFixture, true},
		{"invalid candidate", "lsp-server lua_ls nightly\n", lspFixture, lspFixture, true},
		{"duplicate declaration", "lsp-server lua_ls 3.20.0\n", strings.Replace(lspFixture, "    \"tombi@v1.5.5\",", "    \"lua_ls@3.19.1\",", 1), "", true},
		{"unpinned declaration", "lsp-server lua_ls 3.20.0\n", strings.Replace(lspFixture, "lua_ls@3.19.1", "lua_ls", 1), "", true},
		{"atomic validation", "mise-tool uv 0.12.3\nlsp-server missing 1.0.0\n", lspFixture, lspFixture, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			manifest := filepath.Join(root, "dependencies.conf")
			metadata := filepath.Join(root, "metadata")
			language := filepath.Join(root, "config/shared/nvim/lua/config/languages.lua")
			put(t, manifest, "# empty\n")
			put(t, metadata, tc.metadata)
			put(t, language, tc.source)
			put(t, filepath.Join(root, "config/shared/mise.toml"), mise)
			var output bytes.Buffer
			status := runDependencyUpdate(context.Background(), root, []string{"--metadata", metadata}, selfishell.Process{Out: &output, Err: &output})
			if (status != 0) != tc.fail {
				t.Fatalf("status=%d, error=%s", status, output.String())
			}
			want := tc.want
			if want == "" {
				want = tc.source
			}
			exact(t, language, want)
			if tc.name == "atomic validation" {
				exact(t, filepath.Join(root, "config/shared/mise.toml"), mise)
			}
		})
	}
}

func TestUpdaterLSPRegistry(t *testing.T) {
	root := t.TempDir()
	archive := filepath.Join(root, "registry.zip")
	writeRegistryZip(t, archive, `[{"name":"lua-language-server","source":{"id":"pkg:github/LuaLS/lua-language-server@3.20.0"},"neovim":{"lspconfig":"lua_ls"}},{"name":"tombi","source":{"id":"pkg:github/tombi-toml/tombi@v1.5.6"},"neovim":{"lspconfig":"tombi"}},{"name":"marksman","source":{"id":"pkg:github/artempyanykh/marksman@2026-02-09"},"neovim":{"lspconfig":"marksman"}}]`)
	got, err := lspRegistryUpdates(archive, map[string]string{"lua_ls": "3.19.1", "tombi": "v1.5.5", "marksman": "2026-02-08"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, "\n") != "lsp-server lua_ls 3.20.0\nlsp-server marksman 2026-02-09\nlsp-server tombi v1.5.6" {
		t.Fatalf("updates: %q", got)
	}
	t.Run("scoped package", func(t *testing.T) {
		writeRegistryZip(t, archive, `[{"source":{"id":"pkg:npm/@example/language-server@2.3.4"},"neovim":{"lspconfig":"example_ls"}}]`)
		updates, err := lspRegistryUpdates(archive, map[string]string{"example_ls": "2.3.3"})
		if err != nil || strings.Join(updates, "\n") != "lsp-server example_ls 2.3.4" {
			t.Fatalf("updates=%q error=%v", updates, err)
		}
	})
	for _, tc := range []struct{ name, data string }{
		{"missing", `[]`},
		{"invalid", `[{"neovim":{"lspconfig":"lua_ls"},"source":{"id":"pkg:npm/lua@nightly"}}]`},
		{"duplicate", `[{"neovim":{"lspconfig":"lua_ls"},"source":{"id":"pkg:npm/lua@3.20.0"}},{"neovim":{"lspconfig":"lua_ls"},"source":{"id":"pkg:npm/lua@3.20.1"}}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writeRegistryZip(t, archive, tc.data)
			if _, err := lspRegistryUpdates(archive, map[string]string{"lua_ls": "3.19.1"}); err == nil {
				t.Fatal("invalid registry accepted")
			}
		})
	}
}

func writeRegistryZip(t *testing.T, archive, content string) {
	t.Helper()
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("registry.json")
	if err != nil {
		t.Fatal(err)
	}
	_, err = entry.Write([]byte(content))
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
