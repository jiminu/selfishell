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
# the system vi stays untouched; the README must describe that, not claim vi
# also resolves to Neovim.
test_readme_vi_alias_documentation_matches_implementation() {
  grep -Eq "\`vi\`[^.]*resolve" "$ROOT_DIR/README.md" &&
    fail "README claims vi resolves to Neovim, but no such alias exists"
  grep -Fq "\`vim\` resolves to Neovim" "$ROOT_DIR/README.md" ||
    fail "README does not document that vim resolves to Neovim in the developer profile"
  grep -Fq "alias vim='nvim'" "$ROOT_DIR/common/aliases-editor.zsh" ||
    fail "vim alias implementation not found where expected"
  grep -Fq "alias view='nvim -R'" "$ROOT_DIR/common/aliases-editor.zsh" ||
    fail "view (read-only) alias implementation not found where expected"
  grep -Eq "alias vi=" "$ROOT_DIR/common/aliases-editor.zsh" &&
    fail "A vi alias was reintroduced; update the README explanation if this is intentional"
  return 0
}

# Cross-checks every mise-managed developer-profile tool's pinned version
# (common/mise.toml's [tools] table) against the user-facing profile
# descriptions, so a version bump doesn't silently leave the docs stale.
test_developer_profile_docs_include_mise_tool_versions() {
  local tool version friendly_name

  while IFS='=' read -r tool version; do
    tool="$(printf '%s' "$tool" | tr -d '[:space:]')"
    version="$(printf '%s' "$version" | tr -d '[:space:]"')"
    [[ -n "$tool" && -n "$version" ]] || continue

    case "$tool" in
      node) friendly_name="Node.js" ;;
      python) friendly_name="Python" ;;
      neovim) friendly_name="Neovim" ;;
      tree-sitter) friendly_name="Tree-sitter CLI" ;;
      uv) friendly_name="uv" ;;
      *) fail "Unrecognized mise tool in common/mise.toml: $tool (add it to this test's name mapping)" ;;
    esac

    grep -Fq "$friendly_name $version" "$ROOT_DIR/README.md" ||
      fail "README.md does not document $friendly_name $version"
    grep -Fq "$friendly_name $version" "$ROOT_DIR/docs/PROFILES.md" ||
      fail "docs/PROFILES.md does not document $friendly_name $version"
  done < <(awk '
    /^\[/ { in_tools = ($0 == "[tools]"); next }
    in_tools && /=/ { print }
  ' "$ROOT_DIR/common/mise.toml")
}

# AGENTS.md's "Current State" once described Temurin/kubectl/kubectx as
# mise-managed in the developer profile; none of those are in
# common/mise.toml, which manages Neovim/Tree-sitter/Node.js/Python/uv.
test_agents_developer_profile_tool_list_matches_mise_toml() {
  local mise_line

  mise_line="$(join_wrapped_match 'pinned mise binary for' "$ROOT_DIR/AGENTS.md")"
  [[ -n "$mise_line" ]] || fail "AGENTS.md no longer describes the developer profile's mise-managed tools"

  for tool in Temurin kubectl kubectx; do
    [[ "$mise_line" != *"$tool"* ]] ||
      fail "AGENTS.md still claims $tool is managed by mise in the developer profile"
  done
  [[ "$mise_line" == *uv* ]] || fail "AGENTS.md's mise-managed tool list does not mention uv"
}

# docs/project/MILESTONES.md's M3 profile-boundary description must match
# the actual profiles/minimal.conf and profiles/developer.conf package lists.
test_milestones_profile_boundaries_match_profile_files() {
  local minimal_line developer_line tool

  # The backtick-quoted labels are literal patterns, not expansions.
  # shellcheck disable=SC2016
  minimal_line="$(join_wrapped_match '`minimal`:' "$ROOT_DIR/docs/project/MILESTONES.md")"
  # shellcheck disable=SC2016
  developer_line="$(join_wrapped_match '`developer`:' "$ROOT_DIR/docs/project/MILESTONES.md")"

  [[ -n "$minimal_line" ]] || fail "MILESTONES.md no longer documents the minimal profile boundary"
  [[ -n "$developer_line" ]] || fail "MILESTONES.md no longer documents the developer profile boundary"

  for tool in fzf zoxide ripgrep eza bat jq; do
    [[ "$minimal_line" != *"$tool"* ]] ||
      fail "MILESTONES.md claims minimal includes $tool, but profiles/minimal.conf does not"
    grep -Fqi "$tool" "$ROOT_DIR/profiles/minimal.conf" &&
      fail "profiles/minimal.conf now installs $tool; the minimal boundary description needs a matching update"
  done
  for tool in Temurin kubectl kubectx; do
    [[ "$developer_line" != *"$tool"* ]] ||
      fail "MILESTONES.md still claims developer manages $tool via mise"
  done
  [[ "$developer_line" == *uv* ]] || fail "MILESTONES.md's developer boundary does not mention uv"
}

# AGENTS.md's automatic dependency-release rule must match the actual gate
# (scripts/verify-dependency-release-diff.sh), which allows dependencies.conf
# plus the two Zsh pin files rather than requiring dependencies.conf alone.
test_agents_dependency_release_rule_matches_current_gate() {
  grep -Fq 'sole changed' "$ROOT_DIR/AGENTS.md" &&
    fail "AGENTS.md still describes the old dependencies.conf-only release gate"
  grep -Fq 'common/completion.zsh' "$ROOT_DIR/AGENTS.md" ||
    fail "AGENTS.md does not mention the Zsh pin files the release gate now allows"
  grep -Fq 'common/interactive.zsh' "$ROOT_DIR/AGENTS.md" ||
    fail "AGENTS.md does not mention the Zsh pin files the release gate now allows"
}

test_performance_docs_document_full_benchmark_mode() {
  grep -Fq -- '--mode full' "$ROOT_DIR/docs/PERFORMANCE.md" ||
    fail "docs/PERFORMANCE.md does not document scripts/benchmark.sh --mode full"
  grep -Fq 'SELFISHELL_BENCHMARK_PROFILE' "$ROOT_DIR/docs/PERFORMANCE.md" ||
    fail "docs/PERFORMANCE.md does not document the SELFISHELL_BENCHMARK_PROFILE env var"
  grep -Fq 'shell-full-profile-benchmark' "$ROOT_DIR/docs/PERFORMANCE.md" ||
    fail "docs/PERFORMANCE.md does not name the full-profile benchmark's CI job"
  grep -Fq 'shell-performance-full-profile' "$ROOT_DIR/docs/PERFORMANCE.md" ||
    fail "docs/PERFORMANCE.md does not name the full-profile benchmark's artifact"
}

test_readme_vi_alias_documentation_matches_implementation
printf 'PASS: test_readme_vi_alias_documentation_matches_implementation\n'
test_developer_profile_docs_include_mise_tool_versions
printf 'PASS: test_developer_profile_docs_include_mise_tool_versions\n'
test_agents_developer_profile_tool_list_matches_mise_toml
printf 'PASS: test_agents_developer_profile_tool_list_matches_mise_toml\n'
test_milestones_profile_boundaries_match_profile_files
printf 'PASS: test_milestones_profile_boundaries_match_profile_files\n'
test_agents_dependency_release_rule_matches_current_gate
printf 'PASS: test_agents_dependency_release_rule_matches_current_gate\n'
test_performance_docs_document_full_benchmark_mode
printf 'PASS: test_performance_docs_document_full_benchmark_mode\n'
