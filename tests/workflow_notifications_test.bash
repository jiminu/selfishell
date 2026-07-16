#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/tests/test_helper.bash"

setup_notification_test() {
  setup_test_home
  export MOCK_GH_LOG="$TEST_ROOT/gh.log"
  mkdir -p "$TEST_ROOT/bin"
  cat >"$TEST_ROOT/bin/gh" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$MOCK_GH_LOG"
if [[ "$1 $2" == 'issue list' && -n "${MOCK_ISSUE_NUMBER:-}" ]]; then
  printf '%s\n' "$MOCK_ISSUE_NUMBER"
fi
EOF
  chmod +x "$TEST_ROOT/bin/gh"
  ORIGINAL_PATH="$PATH"
  export PATH="$TEST_ROOT/bin:/usr/bin:/bin"
}

teardown_notification_test() {
  export PATH="$ORIGINAL_PATH"
  unset ORIGINAL_PATH MOCK_GH_LOG MOCK_ISSUE_NUMBER
  teardown_test_home
}

test_creates_one_failure_issue() {
  setup_notification_test
  bash "$ROOT_DIR/scripts/workflow-failure-issue.sh" '[automation] Test failed' failure https://example.invalid/run
  grep -Fq 'issue create --title [automation] Test failed' "$MOCK_GH_LOG" || fail "Failure issue was not created"
  grep -Fq 'label create automation-failure' "$MOCK_GH_LOG" || fail "Failure label was not ensured"
  teardown_notification_test
}

test_reuses_and_closes_existing_issue() {
  setup_notification_test
  export MOCK_ISSUE_NUMBER=42
  bash "$ROOT_DIR/scripts/workflow-failure-issue.sh" '[automation] Test failed' failure https://example.invalid/failure
  grep -Fq 'issue comment 42' "$MOCK_GH_LOG" || fail "Existing failure issue was not reused"
  bash "$ROOT_DIR/scripts/workflow-failure-issue.sh" '[automation] Test failed' success https://example.invalid/success
  grep -Fq 'issue close 42' "$MOCK_GH_LOG" || fail "Recovered failure issue was not closed"
  teardown_notification_test
}

test_creates_one_failure_issue
printf 'PASS: test_creates_one_failure_issue\n'
test_reuses_and_closes_existing_issue
printf 'PASS: test_reuses_and_closes_existing_issue\n'
