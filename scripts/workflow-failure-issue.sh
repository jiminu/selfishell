#!/usr/bin/env bash

set -euo pipefail

title="${1:-}"
status="${2:-}"
run_url="${3:-}"
label=automation-failure

[[ -n "$title" && -n "$status" && -n "$run_url" ]] || {
  printf 'Usage: scripts/workflow-failure-issue.sh TITLE STATUS RUN_URL\n' >&2
  exit 2
}
command -v gh >/dev/null 2>&1 || {
  printf 'gh is required for workflow failure notifications.\n' >&2
  exit 1
}

issue_number="$(gh issue list --state open --label "$label" --search "\"$title\" in:title" --json number,title \
  --jq ".[] | select(.title == \"$title\") | .number" | sed -n '1p')"

if [[ "$status" == success ]]; then
  [[ -n "$issue_number" ]] || exit 0
  gh issue close "$issue_number" --comment "Automation recovered successfully: $run_url"
  exit 0
fi

body="The automated workflow failed. Review the run and resolve the underlying problem before retrying.\n\nRun: $run_url"
if [[ -n "$issue_number" ]]; then
  gh issue comment "$issue_number" --body "$body"
  exit 0
fi

gh label create "$label" --description 'Automated workflow needs attention' --color D73A4A --force
gh issue create --title "$title" --label "$label" --body "$body"
