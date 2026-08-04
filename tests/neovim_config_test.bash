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

test_treesitter_install_rejects_false_and_missing_results() {
  local output

  if ! command -v nvim >/dev/null 2>&1; then
    skip 'test_treesitter_install_rejects_false_and_missing_results (Neovim unavailable)'
  fi

  output="$(run_neovim_fixture treesitter_install.lua)"

  [[ "$output" == *'Tree-sitter install verification: OK'* ]] ||
    fail "Tree-sitter install verification is incomplete: $output"
}

test_every_neovim_plugin_has_an_approved_revision() {
  local repository revision
  local plugin_count=0

  while IFS= read -r repository; do
    revision="$(awk -v repository="$repository" '$1 == "nvim-plugin" && $2 == repository { print $3 }' "$ROOT_DIR/dependencies.conf")"
    [[ "$revision" =~ ^[0-9a-f]{40}$ ]] ||
      fail "Neovim plugin is missing an approved revision: $repository"
    plugin_count=$((plugin_count + 1))
  done < <(sed -n 's/.*plugin("\([^"]*\)".*/\1/p' "$ROOT_DIR"/common/nvim/lua/plugins/*.lua | sort -u)

  [[ "$plugin_count" -eq 24 ]] || fail "Expected 24 pinned Neovim plugins, got $plugin_count"
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

test_buffer_delete_preserves_editor_window() {
  local output

  if ! command -v nvim >/dev/null 2>&1; then
    skip 'test_buffer_delete_preserves_editor_window (Neovim unavailable)'
  fi

  output="$(run_neovim_fixture buffer_delete.lua)"

  [[ "$output" == *'buffer delete layout: OK'* ]] ||
    fail "Buffer deletion did not preserve the editor layout: $output"
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

test_lsp_loads_only_for_supported_filetypes() {
  local output

  if ! command -v nvim >/dev/null 2>&1; then
    skip 'test_lsp_loads_only_for_supported_filetypes (Neovim unavailable)'
  fi

  mkdir -p "$XDG_CONFIG_HOME/nvim"
  cp "$ROOT_DIR/dependencies.conf" "$XDG_CONFIG_HOME/nvim/plugin-versions.conf"
  output="$(run_neovim_fixture lsp_filetypes.lua)"

  [[ "$output" == *'LSP filetype scope: OK'* ]] || fail "LSP filetype scope is invalid: $output"
}

test_lsp_user_extension_is_ignored_when_absent() {
  local output

  if ! command -v nvim >/dev/null 2>&1; then
    skip 'test_lsp_user_extension_is_ignored_when_absent (Neovim unavailable)'
  fi

  output="$(run_neovim_fixture lsp_user_extension_absent.lua)"

  [[ "$output" == *'User extension absent: OK'* ]] ||
    fail "Default LSP and filetypes were changed when no nvim.user.lua exists: $output"
}

test_lsp_user_extension_merges_valid_servers() {
  local output

  if ! command -v nvim >/dev/null 2>&1; then
    skip 'test_lsp_user_extension_merges_valid_servers (Neovim unavailable)'
  fi

  mkdir -p "$XDG_CONFIG_HOME/selfishell"
  cat >"$XDG_CONFIG_HOME/selfishell/nvim.user.lua" <<'EOF'
return {
  servers = {
    clangd = { filetypes = { "c", "cpp" } },
  },
}
EOF

  output="$(run_neovim_fixture lsp_user_extension_merge.lua)"

  [[ "$output" == *'User extension merge: OK'* ]] || fail "LSP user extension did not merge: $output"
}

test_lsp_user_extension_ignores_malformed_file() {
  local output

  if ! command -v nvim >/dev/null 2>&1; then
    skip 'test_lsp_user_extension_ignores_malformed_file (Neovim unavailable)'
  fi

  mkdir -p "$XDG_CONFIG_HOME/selfishell"
  printf 'this is not valid lua {{{\n' >"$XDG_CONFIG_HOME/selfishell/nvim.user.lua"

  output="$(run_neovim_fixture lsp_user_extension_malformed.lua)"

  [[ "$output" == *'User extension malformed handling: OK'* ]] ||
    fail "Malformed LSP user extension was not handled: $output"
}

test_lsp_user_extension_warns_on_missing_executable() {
  local output

  if ! command -v nvim >/dev/null 2>&1; then
    skip 'test_lsp_user_extension_warns_on_missing_executable (Neovim unavailable)'
  fi

  mkdir -p "$XDG_CONFIG_HOME/selfishell"
  cat >"$XDG_CONFIG_HOME/selfishell/nvim.user.lua" <<'EOF'
return {
  servers = {
    jdtls = { filetypes = { "java" } },
  },
}
EOF

  output="$(run_neovim_fixture lsp_user_extension_missing_executable.lua)"

  [[ "$output" == *'User extension executable check: OK'* ]] ||
    fail "Missing-executable warning for jdtls did not fire: $output"
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
  local server

  for server in lua_ls pyright bashls; do
    grep -Eq '"'"$server"'@[0-9]+\.[0-9]+\.[0-9]+"' "$ROOT_DIR/common/nvim/lua/config/languages.lua" ||
      fail "Mason LSP server is not versioned: $server"
  done
}

run_discovered_tests setup_neovim_test teardown_test_home
