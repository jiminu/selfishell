#!/usr/bin/env bash

set -euo pipefail

if [[ $# -ne 2 || ! "$2" =~ ^[0-9a-f]{40}$ ]]; then
  printf 'Usage: %s OWNER/REPO COMMIT_SHA\n' "$0" >&2
  exit 2
fi
repo="$1"
commit="$2"

# Select the latest push run for this exact main commit, never a PR or tag run.
run_id="$(gh run list --repo "$repo" --workflow ci.yml --branch main \
  --event push --commit "$commit" --limit 1 --json databaseId \
  --jq '.[0].databaseId // empty')"
if [[ ! "$run_id" =~ ^[0-9]+$ ]]; then
  printf 'No main push CI run found for %s. Wait for CI and rerun the release workflow.\n' "$commit" >&2
  exit 1
fi

gh run watch "$run_id" --repo "$repo" --interval 10 --exit-status
# Require success explicitly: neutral/skipped runs must not authorize a release.
conclusion="$(gh run view "$run_id" --repo "$repo" --json conclusion --jq .conclusion)"
if [[ "$conclusion" != success ]]; then
  printf 'Main push CI run %s must succeed; got: %s\n' "$run_id" "$conclusion" >&2
  exit 1
fi
printf 'Verified main push CI run %s for %s\n' "$run_id" "$commit"
