#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/tests/test_helper.bash"

# Prose in these docs wraps a sentence or list item across multiple source
# lines; prints the matching line and every following line up to (but not
# including) the next blank line or list item, joined with spaces, so a
# substring check sees the whole sentence/bullet rather than just its first
# line.
join_wrapped_match() {
  local pattern="$1" file="$2"

  awk -v pattern="$pattern" '
    $0 ~ pattern { capture = 1; print; next }
    capture && /^(-|$)/ { exit }
    capture { print }
  ' "$file" | tr '\n' ' '
}

# common/aliases-editor.zsh intentionally aliases only vim/view (not vi), so
# the system vi stays untouched; the profile guide must describe that, not claim
# vi also resolves to Neovim.
test_profile_vi_alias_documentation_matches_implementation() {
  grep -Eq "\`vi\`[^.]*resolve" "$ROOT_DIR/docs/PROFILES.md" &&
    fail "docs/PROFILES.md claims vi resolves to Neovim, but no such alias exists"
  grep -Fq "\`vim\` resolves to Neovim" "$ROOT_DIR/docs/PROFILES.md" ||
    fail "docs/PROFILES.md does not document that vim resolves to Neovim in the developer profile"
  grep -Fq "alias vim='nvim'" "$ROOT_DIR/common/aliases-editor.zsh" ||
    fail "vim alias implementation not found where expected"
  grep -Fq "alias view='nvim -R'" "$ROOT_DIR/common/aliases-editor.zsh" ||
    fail "view (read-only) alias implementation not found where expected"
  grep -Eq "alias vi=" "$ROOT_DIR/common/aliases-editor.zsh" &&
    fail "A vi alias was reintroduced; update the README explanation if this is intentional"
  return 0
}

# AGENTS.md should point to common/mise.toml instead of duplicating its tool
# list.
test_agents_developer_profile_tools_use_mise_source_of_truth() {
  local mise_line
  local mise_source="\`common/mise.toml\`"

  mise_line="$(join_wrapped_match "mise-managed tools are declared" "$ROOT_DIR/AGENTS.md")"
  [[ "$mise_line" == *"$mise_source"* ]] ||
    fail "AGENTS.md does not identify common/mise.toml as the mise-managed tool declaration"
  [[ "$mise_line" == *'source of truth'* ]] ||
    fail "AGENTS.md does not identify common/mise.toml as the mise-managed tool source of truth"
}

# docs/PROFILES.md's profile table must match the actual profiles/minimal.conf
# and profiles/developer.conf package lists.
test_profile_table_boundaries_match_profile_files() {
  local minimal_line developer_line tool

  # The backtick-quoted labels are literal table cells, not expansions.
  # shellcheck disable=SC2016
  minimal_line="$(grep -F '| `minimal` |' "$ROOT_DIR/docs/PROFILES.md")"
  # shellcheck disable=SC2016
  developer_line="$(grep -F '| `developer` |' "$ROOT_DIR/docs/PROFILES.md")"

  [[ -n "$minimal_line" ]] || fail "docs/PROFILES.md no longer documents the minimal profile boundary"
  [[ -n "$developer_line" ]] || fail "docs/PROFILES.md no longer documents the developer profile boundary"

  for tool in fzf zoxide ripgrep eza bat jq; do
    [[ "$minimal_line" != *"$tool"* ]] ||
      fail "docs/PROFILES.md claims minimal includes $tool, but profiles/minimal.conf does not"
    grep -Fqi "$tool" "$ROOT_DIR/profiles/minimal.conf" &&
      fail "profiles/minimal.conf now installs $tool; the minimal boundary description needs a matching update"
  done
  [[ "$developer_line" == *uv* ]] || fail "docs/PROFILES.md's developer boundary does not mention uv"
  return 0
}

test_performance_docs_document_full_benchmark_mode() {
  grep -Fq -- '--mode full' "$ROOT_DIR/docs/PERFORMANCE.md" ||
    fail "docs/PERFORMANCE.md does not document scripts/benchmark.sh --mode full"
  grep -Fq 'SELFISHELL_BENCHMARK_PROFILE' "$ROOT_DIR/docs/PERFORMANCE.md" ||
    fail "docs/PERFORMANCE.md does not document the SELFISHELL_BENCHMARK_PROFILE env var"
}

test_release_procedures_use_published_release_verifier() {
  local verifier='bash scripts/verify-published-release.sh'

  grep -Fq "$verifier" "$ROOT_DIR/docs/RELEASING.md" ||
    fail "docs/RELEASING.md does not use the published release verifier"
  grep -Fq "$verifier" "$ROOT_DIR/.agents/skills/release-selfishell/SKILL.md" ||
    fail "release-selfishell skill does not use the published release verifier"
}

run_discovered_tests
