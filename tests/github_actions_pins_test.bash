#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/tests/test_helper.bash"

# Every `uses:` reference across the workflows must be pinned to a full
# 40-character commit SHA (with a `# vX.Y.Z` comment recording the version
# it corresponds to), not a mutable tag like `@v6` -- so a compromised or
# retagged upstream action can't silently start running different code the
# next time a workflow triggers.
test_github_actions_are_pinned_to_full_commit_shas() {
  local workflow line action_at ref

  while IFS= read -r workflow; do
    while IFS= read -r line; do
      action_at="$(printf '%s\n' "$line" | sed -n 's/.*uses:[[:space:]]*\([^[:space:]]*\).*/\1/p')"
      [[ -n "$action_at" ]] || continue
      ref="${action_at#*@}"
      [[ "$ref" =~ ^[0-9a-f]{40}$ ]] ||
        fail "$workflow uses a non-SHA action reference: $action_at (want a full 40-character commit SHA)"
      printf '%s\n' "$line" | grep -Eq '#[[:space:]]*v[0-9]' ||
        fail "$workflow pins $action_at without a \"# vX.Y.Z\" version comment"
    done < <(grep -F 'uses:' "$workflow")
  done < <(find "$ROOT_DIR/.github/workflows" -type f -name '*.yml')
}

test_dependabot_tracks_github_actions() {
  [[ -r "$ROOT_DIR/.github/dependabot.yml" ]] ||
    fail "No .github/dependabot.yml; SHA-pinned actions need Dependabot's github-actions ecosystem to stay updated"
  grep -Fq 'github-actions' "$ROOT_DIR/.github/dependabot.yml" ||
    fail ".github/dependabot.yml does not track the github-actions ecosystem"
}

test_pinned_neovim_job_runs_configuration_tests() {
  local workflow="$ROOT_DIR/.github/workflows/ci.yml"
  local config_command='bash tests/neovim_config_test.bash'
  local lifecycle_command='bash scripts/neovim-e2e.sh'
  local config_match job lifecycle_match

  job="$(awk '
    /^  neovim-developer-e2e:/ { in_job = 1 }
    in_job && /^  [[:alnum:]_-]+:/ && $1 != "neovim-developer-e2e:" { exit }
    in_job { print }
  ' "$workflow")"
  config_match="$(printf '%s\n' "$job" | grep -nF "$config_command")" || true
  lifecycle_match="$(printf '%s\n' "$job" | grep -nF "$lifecycle_command")" || true

  [[ -n "$config_match" ]] ||
    fail "Pinned Neovim CI job does not run the configuration tests"
  [[ -n "$lifecycle_match" && "${config_match%%:*}" -lt "${lifecycle_match%%:*}" ]] ||
    fail "Pinned Neovim configuration tests must run before the lifecycle E2E"
}

run_discovered_tests
