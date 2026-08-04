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

test_lsp_user_extension_derives_filetypes_from_nvim_lspconfig() {
  local output

  if ! command -v nvim >/dev/null 2>&1; then
    skip 'test_lsp_user_extension_derives_filetypes_from_nvim_lspconfig (Neovim unavailable)'
  fi

  mkdir -p "$XDG_CONFIG_HOME/selfishell"
  cat >"$XDG_CONFIG_HOME/selfishell/nvim.user.lua" <<'EOF'
return {
  servers = {
    fake_server = {},
  },
}
EOF

  mkdir -p "$XDG_DATA_HOME/nvim/lazy/nvim-lspconfig/lsp"
  cat >"$XDG_DATA_HOME/nvim/lazy/nvim-lspconfig/lsp/fake_server.lua" <<'EOF'
return {
  filetypes = { "fake_ft" },
}
EOF

  output="$(run_neovim_fixture lsp_user_extension_auto_filetypes.lua)"

  [[ "$output" == *'User extension auto filetypes: OK'* ]] ||
    fail "Filetypes were not auto-derived from nvim-lspconfig: $output"
}

test_lsp_user_extension_empty_filetypes_falls_back_to_nvim_lspconfig() {
  local output

  if ! command -v nvim >/dev/null 2>&1; then
    skip 'test_lsp_user_extension_empty_filetypes_falls_back_to_nvim_lspconfig (Neovim unavailable)'
  fi

  mkdir -p "$XDG_CONFIG_HOME/selfishell"
  cat >"$XDG_CONFIG_HOME/selfishell/nvim.user.lua" <<'EOF'
return {
  servers = {
    fake_server = { filetypes = {} },
  },
}
EOF

  mkdir -p "$XDG_DATA_HOME/nvim/lazy/nvim-lspconfig/lsp"
  cat >"$XDG_DATA_HOME/nvim/lazy/nvim-lspconfig/lsp/fake_server.lua" <<'EOF'
return {
  filetypes = { "fake_ft" },
}
EOF

  output="$(run_neovim_fixture lsp_user_extension_auto_filetypes.lua)"

  [[ "$output" == *'User extension auto filetypes: OK'* ]] ||
    fail "An empty filetypes list did not fall back to nvim-lspconfig: $output"
}

test_lsp_user_extension_without_filetypes_or_lspconfig_match_is_rejected() {
  local output

  if ! command -v nvim >/dev/null 2>&1; then
    skip 'test_lsp_user_extension_without_filetypes_or_lspconfig_match_is_rejected (Neovim unavailable)'
  fi

  mkdir -p "$XDG_CONFIG_HOME/selfishell"
  cat >"$XDG_CONFIG_HOME/selfishell/nvim.user.lua" <<'EOF'
return {
  servers = {
    unknown_server = {},
  },
}
EOF

  output="$(run_neovim_fixture lsp_user_extension_no_filetypes_found.lua)"

  [[ "$output" == *'User extension no filetypes found: OK'* ]] ||
    fail "A server with no filetypes and no nvim-lspconfig match was not rejected cleanly: $output"
}

test_lsp_user_extension_redeclaring_default_server_does_not_duplicate() {
  local output

  if ! command -v nvim >/dev/null 2>&1; then
    skip 'test_lsp_user_extension_redeclaring_default_server_does_not_duplicate (Neovim unavailable)'
  fi

  mkdir -p "$XDG_CONFIG_HOME/selfishell"
  cat >"$XDG_CONFIG_HOME/selfishell/nvim.user.lua" <<'EOF'
return {
  servers = {
    pyright = { filetypes = { "python" } },
  },
}
EOF

  output="$(run_neovim_fixture lsp_user_extension_redeclares_default.lua)"

  [[ "$output" == *'User extension redeclare handling: OK'* ]] ||
    fail "Redeclaring a default server duplicated its lsp entry: $output"
}

test_lsp_user_extension_redeclaring_default_server_with_version_does_not_duplicate() {
  local output

  if ! command -v nvim >/dev/null 2>&1; then
    skip 'test_lsp_user_extension_redeclaring_default_server_with_version_does_not_duplicate (Neovim unavailable)'
  fi

  mkdir -p "$XDG_CONFIG_HOME/selfishell"
  cat >"$XDG_CONFIG_HOME/selfishell/nvim.user.lua" <<'EOF'
return {
  servers = {
    ["pyright@2.0.0"] = { filetypes = { "python" } },
  },
}
EOF

  output="$(run_neovim_fixture lsp_user_extension_redeclares_default.lua)"

  [[ "$output" == *'User extension redeclare handling: OK'* ]] ||
    fail "Redeclaring a default server with a version suffix duplicated its lsp entry: $output"
}

test_lsp_user_extension_rejects_non_string_filetype_element() {
  local output

  if ! command -v nvim >/dev/null 2>&1; then
    skip 'test_lsp_user_extension_rejects_non_string_filetype_element (Neovim unavailable)'
  fi

  mkdir -p "$XDG_CONFIG_HOME/selfishell"
  cat >"$XDG_CONFIG_HOME/selfishell/nvim.user.lua" <<'EOF'
return {
  servers = {
    clangd = { filetypes = { "c", 42 } },
  },
}
EOF

  output="$(run_neovim_fixture lsp_user_extension_invalid_filetype_element.lua)"

  [[ "$output" == *'User extension invalid filetype element: OK'* ]] ||
    fail "A non-string filetype element was not rejected: $output"
}

test_lsp_user_extension_derives_filetypes_when_lspconfig_file_requires_a_sibling_module() {
  local output

  if ! command -v nvim >/dev/null 2>&1; then
    skip 'test_lsp_user_extension_derives_filetypes_when_lspconfig_file_requires_a_sibling_module (Neovim unavailable)'
  fi

  mkdir -p "$XDG_CONFIG_HOME/selfishell"
  cat >"$XDG_CONFIG_HOME/selfishell/nvim.user.lua" <<'EOF'
return {
  servers = {
    needs_require = {},
  },
}
EOF

  # Several real nvim-lspconfig lsp/<name>.lua files (eslint, tailwindcss,
  # biome, ...) `require` a sibling module such as lspconfig.util. Reproduce
  # that shape here rather than only the bare-table fixture the other
  # auto-derive test uses.
  mkdir -p "$XDG_DATA_HOME/nvim/lazy/nvim-lspconfig/lsp" "$XDG_DATA_HOME/nvim/lazy/nvim-lspconfig/lua"
  cat >"$XDG_DATA_HOME/nvim/lazy/nvim-lspconfig/lua/sibling_module.lua" <<'EOF'
return {}
EOF
  cat >"$XDG_DATA_HOME/nvim/lazy/nvim-lspconfig/lsp/needs_require.lua" <<'EOF'
require("sibling_module")
return {
  filetypes = { "needs_require_ft" },
}
EOF

  output="$(run_neovim_fixture lsp_user_extension_lspconfig_with_require.lua)"

  [[ "$output" == *'User extension lspconfig require: OK'* ]] ||
    fail "Filetypes were not derived when the nvim-lspconfig file requires a sibling module: $output"
}

test_lsp_user_extension_rejects_file_with_no_return_value() {
  local output

  if ! command -v nvim >/dev/null 2>&1; then
    skip 'test_lsp_user_extension_rejects_file_with_no_return_value (Neovim unavailable)'
  fi

  mkdir -p "$XDG_CONFIG_HOME/selfishell"
  cat >"$XDG_CONFIG_HOME/selfishell/nvim.user.lua" <<'EOF'
return false
EOF

  output="$(run_neovim_fixture lsp_user_extension_no_return_value.lua)"

  [[ "$output" == *'User extension no return value: OK'* ]] ||
    fail "A nvim.user.lua returning false was not rejected with a notification: $output"
}

test_lsp_user_extension_rejects_invalid_server_names() {
  local output

  if ! command -v nvim >/dev/null 2>&1; then
    skip 'test_lsp_user_extension_rejects_invalid_server_names (Neovim unavailable)'
  fi

  mkdir -p "$XDG_CONFIG_HOME/selfishell"
  cat >"$XDG_CONFIG_HOME/selfishell/nvim.user.lua" <<'EOF'
return {
  servers = {
    [""] = { filetypes = { "c" } },
    ["../../etc/passwd"] = { filetypes = { "c" } },
  },
}
EOF

  output="$(run_neovim_fixture lsp_user_extension_invalid_name.lua)"

  [[ "$output" == *'User extension invalid name: OK'* ]] ||
    fail "Invalid server names were not rejected: $output"
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

  for server in lua_ls pyright bashls ts_ls; do
    grep -Eq '"'"$server"'@[0-9]+\.[0-9]+\.[0-9]+"' "$ROOT_DIR/common/nvim/lua/config/languages.lua" ||
      fail "Mason LSP server is not versioned: $server"
  done
}

run_discovered_tests setup_neovim_test teardown_test_home
