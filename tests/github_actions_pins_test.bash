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

# A workflow_dispatch release with the default `ref: main` is a mutable
# branch: if verify and publish each independently re-resolved it, a push
# landing between the two could let publish release a commit verify never
# tested. The `resolve` job pins the ref to one commit SHA up front so both
# jobs are guaranteed to check out the exact same, already-verified commit.
extract_workflow_job() {
  local workflow="$1"
  local job="$2"

  awk -v job="  $job:" '
    $0 == job { in_job = 1; print; next }
    in_job && /^  [[:alnum:]_-]+:/ { exit }
    in_job { print }
  ' "$workflow"
}

test_release_workflow_pins_verify_and_publish_to_one_resolved_sha() {
  local workflow="$ROOT_DIR/.github/workflows/release.yml"
  local resolve_job verify_job publish_job

  resolve_job="$(extract_workflow_job "$workflow" resolve)"
  verify_job="$(extract_workflow_job "$workflow" verify)"
  publish_job="$(extract_workflow_job "$workflow" publish)"

  [[ -n "$resolve_job" ]] || fail "release.yml has no resolve job to pin one immutable commit"
  printf '%s\n' "$resolve_job" | grep -Fq 'sha:' ||
    fail "resolve job does not declare a sha output"

  printf '%s\n' "$verify_job" | grep -Fq 'needs: resolve' ||
    fail "verify job does not depend on resolve"
  # shellcheck disable=SC2016 # Matching a literal GitHub Actions expression.
  printf '%s\n' "$verify_job" | grep -Fq 'ref: ${{ needs.resolve.outputs.sha }}' ||
    fail "verify job does not check out the resolved commit SHA"

  printf '%s\n' "$publish_job" | grep -Eq 'needs:.*resolve' ||
    fail "publish job does not depend on resolve"
  # shellcheck disable=SC2016 # Matching a literal GitHub Actions expression.
  printf '%s\n' "$publish_job" | grep -Fq 'ref: ${{ needs.resolve.outputs.sha }}' ||
    fail "publish job does not check out the resolved commit SHA"
}

test_release_workflow_scopes_permissions_per_job() {
  local workflow="$ROOT_DIR/.github/workflows/release.yml"
  local job

  grep -qE '^permissions:' "$workflow" &&
    fail "release.yml declares a shared top-level permissions block instead of per-job scoping"

  for job in resolve verify publish notify; do
    extract_workflow_job "$workflow" "$job" | grep -Fq 'permissions:' ||
      fail "release.yml job \"$job\" does not declare its own permissions"
  done
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
