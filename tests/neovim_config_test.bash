#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/tests/test_helper.bash"

setup_neovim_test() {
  setup_test_home
  export XDG_CACHE_HOME="$HOME/.cache"
  export XDG_CONFIG_HOME="$HOME/.config"
  export XDG_DATA_HOME="$HOME/.local/share"
  export XDG_STATE_HOME="$HOME/.local/state"
  mkdir -p "$TEST_ROOT/tmp" "$XDG_CACHE_HOME" "$XDG_CONFIG_HOME" "$XDG_DATA_HOME" "$XDG_STATE_HOME"
}

run_neovim_fixture() {
  local fixture="$1"

  NVIM_LOG_FILE="$TEST_ROOT/nvim.log" \
    SELFISHELL_NVIM_TEST_FIXTURE="$ROOT_DIR/tests/fixtures/neovim/$fixture" \
    SELFISHELL_TEST_REVISION="${SELFISHELL_TEST_REVISION:-}" \
    TMPDIR="$TEST_ROOT/tmp" \
    nvim --headless -u NONE -i NONE \
    --cmd "set runtimepath^=$ROOT_DIR/common/nvim" \
    '+lua dofile(vim.env.SELFISHELL_NVIM_TEST_FIXTURE)' \
    +qa 2>&1
}

test_treesitter_autocmd_uses_detected_filetypes() {
  local output

  if ! command -v nvim >/dev/null 2>&1; then
    skip 'test_treesitter_autocmd_uses_detected_filetypes (Neovim unavailable)'
  fi

  output="$(run_neovim_fixture treesitter_autocmd.lua)"
  # This headless nvim build emits CRLF line endings for print() on some
  # platforms (observed on WSL2); normalize before the line-spanning match.
  output="${output//$'\r'/}"

  [[ "$output" == *$'sh\nterraform'* ]] ||
    fail "Tree-sitter autocmd did not use FileType values and Terraform mapping: $output"
}

test_zsh_uses_builtin_syntax_fallback() {
  local output

  if ! command -v nvim >/dev/null 2>&1; then
    skip 'test_zsh_uses_builtin_syntax_fallback (Neovim unavailable)'
  fi

  output="$(run_neovim_fixture zsh_syntax.lua)"

  [[ "$output" == *'Zsh syntax fallback: OK'* ]] ||
    fail "Zsh built-in syntax fallback is broken: $output"
}

test_treesitter_auto_installs_missing_parsers_on_filetype() {
  local output

  if ! command -v nvim >/dev/null 2>&1; then
    skip 'test_treesitter_auto_installs_missing_parsers_on_filetype (Neovim unavailable)'
  fi

  output="$(run_neovim_fixture treesitter_auto_install.lua)"

  [[ "$output" == *'Tree-sitter auto-install: OK'* ]] ||
    fail "Tree-sitter auto-install on FileType is broken: $output"
}

test_every_neovim_plugin_has_an_approved_revision() {
  local repository revision declared_plugins configured_plugins diff_output

  # lazy.nvim bootstraps itself (config/lazy.lua) rather than being declared
  # via plugin(...) in common/nvim/lua/plugins/*.lua, so it's pinned in
  # dependencies.conf without a matching Lua declaration -- checked
  # separately below instead of folded into the set comparison.
  declared_plugins="$(sed -n 's/.*plugin("\([^"]*\)".*/\1/p' "$ROOT_DIR"/common/nvim/lua/plugins/*.lua | sort -u)"
  configured_plugins="$(awk '$1 == "nvim-plugin" && $2 != "folke/lazy.nvim" { print $2 }' "$ROOT_DIR/dependencies.conf" | sort -u)"

  [[ -n "$declared_plugins" ]] || fail "No Neovim plugins were discovered in common/nvim/lua/plugins/*.lua"

  diff_output="$(diff <(printf '%s\n' "$declared_plugins") <(printf '%s\n' "$configured_plugins") || true)"
  [[ -z "$diff_output" ]] ||
    fail "Neovim plugins declared in common/nvim/lua/plugins/*.lua and nvim-plugin entries in dependencies.conf differ:
$diff_output"

  while IFS= read -r repository; do
    revision="$(awk -v repository="$repository" '$1 == "nvim-plugin" && $2 == repository { print $3 }' "$ROOT_DIR/dependencies.conf")"
    [[ "$revision" =~ ^[0-9a-f]{40}$ ]] ||
      fail "Neovim plugin is missing an approved revision: $repository"
  done <<<"$declared_plugins"

  revision="$(awk '$1 == "nvim-plugin" && $2 == "folke/lazy.nvim" { print $3 }' "$ROOT_DIR/dependencies.conf")"
  [[ "$revision" =~ ^[0-9a-f]{40}$ ]] || fail "lazy.nvim is missing an approved revision"
}

test_pinned_neovim_plugin_specs_load() {
  local output

  if ! command -v nvim >/dev/null 2>&1; then
    skip 'test_pinned_neovim_plugin_specs_load (Neovim unavailable)'
  fi

  mkdir -p "$XDG_CONFIG_HOME/nvim"
  cp "$ROOT_DIR/dependencies.conf" "$XDG_CONFIG_HOME/nvim/plugin-versions.conf"
  output="$(run_neovim_fixture pinned_plugin_specs.lua)"

  [[ "$output" == *'pinned plugin specs: OK'* ]] || fail "Pinned Neovim plugin specs did not load: $output"
}

test_plugin_versions_module_tolerates_missing_manifest() {
  local output

  if ! command -v nvim >/dev/null 2>&1; then
    skip 'test_plugin_versions_module_tolerates_missing_manifest (Neovim unavailable)'
  fi

  mkdir -p "$XDG_CONFIG_HOME/nvim"
  output="$(run_neovim_fixture plugin_versions_missing_manifest.lua)"

  [[ "$output" == *'plugin_versions missing manifest: OK'* ]] ||
    fail "config.plugin_versions crashed on a missing manifest instead of degrading gracefully: $output"
}

test_editor_workflow_options_and_keymaps() {
  local output

  if ! command -v nvim >/dev/null 2>&1; then
    skip 'test_editor_workflow_options_and_keymaps (Neovim unavailable)'
  fi

  mkdir -p "$XDG_CONFIG_HOME/nvim"
  cp "$ROOT_DIR/dependencies.conf" "$XDG_CONFIG_HOME/nvim/plugin-versions.conf"
  output="$(run_neovim_fixture editor_workflow.lua)"

  [[ "$output" == *'editor workflows: OK'* ]] || fail "Editor workflow configuration is invalid: $output"
}

test_lazy_revision_prefers_detached_head() {
  local lazy_path
  local output
  local revision

  if ! command -v nvim >/dev/null 2>&1; then
    skip 'test_lazy_revision_prefers_detached_head (Neovim unavailable)'
  fi

  revision="$(awk '$1 == "nvim-plugin" && $2 == "folke/lazy.nvim" { print $3 }' "$ROOT_DIR/dependencies.conf")"
  lazy_path="$XDG_DATA_HOME/selfishell/nvim/lazy/lazy.nvim"
  mkdir -p "$lazy_path/.git"
  printf '%s\n' "$revision" >"$lazy_path/.git/HEAD"

  output="$(SELFISHELL_TEST_REVISION="$revision" run_neovim_fixture lazy_detached_head.lua)"

  [[ "$output" == *'true'* ]] || fail "lazy.nvim did not use its detached HEAD directly: $output"
}

test_lazy_revision_falls_back_for_symbolic_head() {
  local lazy_path
  local output
  local revision

  if ! command -v nvim >/dev/null 2>&1; then
    skip 'test_lazy_revision_falls_back_for_symbolic_head (Neovim unavailable)'
  fi

  lazy_path="$XDG_DATA_HOME/selfishell/nvim/lazy/lazy.nvim"
  mkdir -p "$lazy_path"
  rm -rf "$lazy_path/.git"
  git -C "$lazy_path" init --quiet
  git -C "$lazy_path" -c user.name=Selfishell -c user.email=selfishell@example.invalid \
    commit --allow-empty --quiet --message test
  revision="$(git -C "$lazy_path" rev-parse HEAD)"
  grep -Eq '^ref: refs/heads/' "$lazy_path/.git/HEAD" ||
    fail "Test repository did not create a symbolic HEAD"

  output="$(SELFISHELL_TEST_REVISION="$revision" run_neovim_fixture lazy_symbolic_head.lua)"

  [[ "$output" == *'true'* ]] ||
    fail "lazy.nvim did not fall back to Git for a symbolic HEAD: $output"
}

test_lsp_mason_setup_is_not_filetype_limited() {
  local output

  if ! command -v nvim >/dev/null 2>&1; then
    skip 'test_lsp_mason_setup_is_not_filetype_limited (Neovim unavailable)'
  fi

  mkdir -p "$XDG_CONFIG_HOME/nvim"
  cp "$ROOT_DIR/dependencies.conf" "$XDG_CONFIG_HOME/nvim/plugin-versions.conf"
  output="$(run_neovim_fixture lsp_mason_setup.lua)"

  [[ "$output" == *'LSP Mason setup: OK'* ]] || fail "mason-lspconfig setup is invalid: $output"
}

test_last_cursor_restore_targets_correct_window_and_skips_invalid_cases() {
  local output

  if ! command -v nvim >/dev/null 2>&1; then
    skip 'test_last_cursor_restore_targets_correct_window_and_skips_invalid_cases (Neovim unavailable)'
  fi

  output="$(run_neovim_fixture cursor_restore.lua)"

  [[ "$output" == *'cursor restore targeting: OK'* ]] ||
    fail "Last-cursor-position restore did not target windows correctly: $output"
}

test_mason_lsp_servers_are_versioned() {
  local declared_servers server

  declared_servers="$(sed -n '/^  lsp = {/,/^  },/p' "$ROOT_DIR/common/nvim/lua/config/languages.lua" |
    grep -oE '"[^"]+"' | tr -d '"')"
  [[ -n "$declared_servers" ]] || fail "No default LSP servers were discovered in languages.lua"

  # Most Mason packages track bare semver, optionally "v"-prefixed when a
  # package's version follows its upstream release tag verbatim (e.g. tombi's
  # v1.2.7 GitHub releases). marksman instead cuts dated releases
  # (YYYY-MM-DD), so that exact-date form is accepted too -- any of these
  # still pins to one specific, resolvable Mason version.
  while IFS= read -r server; do
    [[ "$server" =~ ^[A-Za-z0-9_-]+@(v?[0-9]+\.[0-9]+\.[0-9]+|[0-9]{4}-[0-9]{2}-[0-9]{2})$ ]] ||
      fail "Default LSP server is not pinned to a supported version format: $server"
  done <<<"$declared_servers"
}

run_discovered_tests setup_neovim_test teardown_test_home
