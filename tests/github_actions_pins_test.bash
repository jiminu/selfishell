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

# `gh pr list ... | grep -q '^0$' && gh pr create ...` used to make the whole
# step exit non-zero whenever an automation PR was already open (grep finds
# no match, and under GitHub Actions' default `bash -e`, that failing left
# side of `&&` becomes the script's own exit status) -- which is this
# workflow's normal steady state, so every scheduled run reported failure and
# opened a bogus alert issue. Extract the fixed if/else block directly out of
# the workflow file and execute it with a mocked `gh`, so this stays honest
# to whatever the YAML actually contains rather than a copy that can drift.
extract_lines_between() {
  local file="$1"
  local start_pattern="$2"
  local end_pattern="$3"

  awk -v start="$start_pattern" -v end="$end_pattern" '
    $0 ~ start { active = 1 }
    active { print }
    active && $0 ~ end { exit }
  ' "$file"
}

test_dependency_update_pr_step_does_not_fail_when_pr_already_open() {
  local workflow="$ROOT_DIR/.github/workflows/dependency-updates.yml"
  local snippet fake_bin status output

  setup_test_home
  snippet="$(extract_lines_between "$workflow" 'open_pr_count=' '^ *fi$')"
  [[ -n "$snippet" ]] || fail "Could not extract the PR-creation snippet from dependency-updates.yml"

  fake_bin="$TEST_ROOT/bin"
  mkdir -p "$fake_bin"
  cat >"$fake_bin/gh" <<'EOF'
#!/usr/bin/env bash
if [[ "$1" == pr && "$2" == list ]]; then
  printf '1\n'
  exit 0
fi
if [[ "$1" == pr && "$2" == create ]]; then
  printf 'UNEXPECTED: gh pr create ran even though a PR was already open\n' >&2
  exit 1
fi
exit 0
EOF
  chmod +x "$fake_bin/gh"

  set +e
  output="$(PATH="$fake_bin:$PATH" BRANCH=automation/dependency-updates bash -c "$snippet" 2>&1)"
  status=$?
  set -e

  [[ "$status" -eq 0 ]] ||
    fail "The PR-creation step failed (exit $status) even though a PR was already open: $output"
  [[ "$output" != *'UNEXPECTED'* ]] || fail "$output"
  teardown_test_home
}

test_dependency_update_pr_step_creates_pr_when_none_open() {
  local workflow="$ROOT_DIR/.github/workflows/dependency-updates.yml"
  local snippet fake_bin status output

  setup_test_home
  snippet="$(extract_lines_between "$workflow" 'open_pr_count=' '^ *fi$')"
  [[ -n "$snippet" ]] || fail "Could not extract the PR-creation snippet from dependency-updates.yml"

  fake_bin="$TEST_ROOT/bin"
  mkdir -p "$fake_bin"
  cat >"$fake_bin/gh" <<'EOF'
#!/usr/bin/env bash
if [[ "$1" == pr && "$2" == list ]]; then
  printf '0\n'
  exit 0
fi
if [[ "$1" == pr && "$2" == create ]]; then
  printf 'CREATED\n'
  exit 0
fi
exit 0
EOF
  chmod +x "$fake_bin/gh"

  set +e
  output="$(PATH="$fake_bin:$PATH" BRANCH=automation/dependency-updates bash -c "$snippet" 2>&1)"
  status=$?
  set -e

  [[ "$status" -eq 0 ]] ||
    fail "The PR-creation step failed (exit $status) when it should have created a PR: $output"
  [[ "$output" == *'CREATED'* ]] || fail "gh pr create did not run when no PR was open: $output"
  teardown_test_home
}

# On a pull_request event, the "changes" job's checkout is the PR's own
# (unreviewed) content, including its own copy of scripts/classify-ci-changes.sh
# -- which gates whether the real lifecycle e2e jobs run. A PR could edit
# that script to always report "nothing relevant changed" and suppress its
# own e2e coverage. The fix runs the classifier as it exists at $BASE_SHA
# (a commit that predates the PR) instead of the checked-out copy. This test
# builds a real throwaway git repo with a "trusted" classifier at the base
# commit and a "tampered" one at the head commit, extracts the actual
# "Detect runtime changes" run: block out of ci.yml, and confirms it produces
# the base (trusted) classifier's output, not the tampered one's.
test_ci_classification_step_uses_base_ref_classifier_not_pr_content() {
  local workflow="$ROOT_DIR/.github/workflows/ci.yml"
  local snippet repo base_sha head_sha github_output github_summary status

  setup_test_home
  repo="$TEST_ROOT/repo"
  mkdir -p "$repo/scripts"
  git -C "$repo" init --quiet
  git -C "$repo" config user.email test@example.com
  git -C "$repo" config user.name test

  printf '#!/usr/bin/env bash\nprintf "runtime=true\\n"\nprintf "ubuntu_container_e2e=true\\n"\n' \
    >"$repo/scripts/classify-ci-changes.sh"
  git -C "$repo" add scripts/classify-ci-changes.sh
  git -C "$repo" commit --quiet -m 'trusted base classifier'
  base_sha="$(git -C "$repo" rev-parse HEAD)"

  printf '#!/usr/bin/env bash\nprintf "runtime=false\\n"\nprintf "ubuntu_container_e2e=false\\n"\n' \
    >"$repo/scripts/classify-ci-changes.sh"
  git -C "$repo" add scripts/classify-ci-changes.sh
  git -C "$repo" commit --quiet -m 'tampered PR classifier'
  head_sha="$(git -C "$repo" rev-parse HEAD)"

  snippet="$(awk '
    /Detect runtime changes/ { in_step = 1 }
    in_step && /run: \|/ { in_run = 1; next }
    in_run && !/^ {10}/ && !/^$/ { exit }
    in_run { print }
  ' "$workflow")"
  [[ -n "$snippet" ]] || fail "Could not extract the Detect runtime changes run: block from ci.yml"

  github_output="$TEST_ROOT/github-output"
  github_summary="$TEST_ROOT/github-summary"
  : >"$github_output"
  : >"$github_summary"

  set +e
  (
    cd "$repo" && EVENT_NAME=pull_request BASE_SHA="$base_sha" GITHUB_SHA="$head_sha" \
      GITHUB_OUTPUT="$github_output" GITHUB_STEP_SUMMARY="$github_summary" \
      bash -c "$snippet"
  )
  status=$?
  set -e

  [[ "$status" -eq 0 ]] || fail "The classification step failed (exit $status)"
  grep -Fqx 'runtime=true' "$github_output" ||
    fail "Classification did not use the trusted base-ref classifier: $(cat "$github_output")"
  grep -Fqx 'ubuntu_container_e2e=true' "$github_output" ||
    fail "Classification did not use the trusted base-ref classifier: $(cat "$github_output")"
  teardown_test_home
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
