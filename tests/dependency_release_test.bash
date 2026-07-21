#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/tests/test_helper.bash"

REPO_DIR=""
BASE_SHA=""
HEAD_SHA=""
OLD_COMPLETIONS=1111111111111111111111111111111111111111
NEW_COMPLETIONS=2222222222222222222222222222222222222222
OLD_FZF_TAB=3333333333333333333333333333333333333333
NEW_FZF_TAB=4444444444444444444444444444444444444444

# Builds a throwaway repo with a "base" commit (dependencies.conf plus the
# two pinned Zsh files) and returns control before the "head" commit, so
# each test can make its own change and commit it independently.
init_release_repo() {
  REPO_DIR="$TEST_ROOT/repo"
  mkdir -p "$REPO_DIR/common"
  git -C "$REPO_DIR" init -q -b main
  git -C "$REPO_DIR" config user.email test@example.com
  git -C "$REPO_DIR" config user.name test

  cat >"$REPO_DIR/dependencies.conf" <<EOF
download starship 1.0.0 linux amd64 https://old/starship.tar.gz oldsum .local/bin/starship starship
zsh-plugin zsh-users/zsh-completions $OLD_COMPLETIONS all all https://github.com/zsh-users/zsh-completions.git - - -
zsh-plugin Aloxaf/fzf-tab $OLD_FZF_TAB all all https://github.com/Aloxaf/fzf-tab.git - - -
EOF
  cat >"$REPO_DIR/common/completion.zsh" <<EOF
zinit ice blockf ver'$OLD_COMPLETIONS'
zinit light zsh-users/zsh-completions
EOF
  cat >"$REPO_DIR/common/interactive.zsh" <<EOF
zinit ice ver'$OLD_FZF_TAB'
zinit light Aloxaf/fzf-tab
EOF
  printf 'unrelated\n' >"$REPO_DIR/README.md"
  git -C "$REPO_DIR" add -A
  git -C "$REPO_DIR" commit -q -m base
  BASE_SHA="$(git -C "$REPO_DIR" rev-parse HEAD)"
}

commit_head() {
  git -C "$REPO_DIR" add -A
  git -C "$REPO_DIR" commit -q -m head
  HEAD_SHA="$(git -C "$REPO_DIR" rev-parse HEAD)"
}

verify_diff() {
  bash "$ROOT_DIR/scripts/verify-dependency-release-diff.sh" \
    --repo-dir "$REPO_DIR" --base "$BASE_SHA" --head "$HEAD_SHA"
}

test_allows_manifest_only_change() {
  setup_test_home
  trap teardown_test_home EXIT
  init_release_repo
  sed -i.bak 's/1\.0\.0/1.0.1/' "$REPO_DIR/dependencies.conf"
  rm -f "$REPO_DIR/dependencies.conf.bak"
  commit_head

  verify_diff || fail "A dependencies.conf-only change should be allowed"
}

test_allows_manifest_and_pin_only_zsh_changes() {
  setup_test_home
  trap teardown_test_home EXIT
  init_release_repo
  sed -i.bak "s/$OLD_COMPLETIONS/$NEW_COMPLETIONS/" "$REPO_DIR/dependencies.conf" "$REPO_DIR/common/completion.zsh"
  rm -f "$REPO_DIR/dependencies.conf.bak" "$REPO_DIR/common/completion.zsh.bak"
  commit_head

  verify_diff || fail "A manifest change paired with a matching pin-only Zsh change should be allowed"
}

test_rejects_unexpected_file() {
  local status=0

  setup_test_home
  trap teardown_test_home EXIT
  init_release_repo
  sed -i.bak 's/1\.0\.0/1.0.1/' "$REPO_DIR/dependencies.conf"
  rm -f "$REPO_DIR/dependencies.conf.bak"
  printf 'surprise\n' >"$REPO_DIR/README.md"
  commit_head

  verify_diff >/dev/null 2>&1 || status=$?
  [[ "$status" -ne 0 ]] || fail "A change outside the allowlist should be rejected"
}

test_rejects_non_pin_zsh_change() {
  local status=0

  setup_test_home
  trap teardown_test_home EXIT
  init_release_repo
  sed -i.bak "s/$OLD_COMPLETIONS/$NEW_COMPLETIONS/" "$REPO_DIR/dependencies.conf" "$REPO_DIR/common/completion.zsh"
  rm -f "$REPO_DIR/dependencies.conf.bak" "$REPO_DIR/common/completion.zsh.bak"
  printf 'echo unrelated-code-change\n' >>"$REPO_DIR/common/completion.zsh"
  commit_head

  verify_diff >/dev/null 2>&1 || status=$?
  [[ "$status" -ne 0 ]] || fail "A non-pin code change in a Zsh pin file should be rejected"
}

test_rejects_zsh_change_without_manifest_zsh_plugin_change() {
  local status=0

  setup_test_home
  trap teardown_test_home EXIT
  init_release_repo
  # dependencies.conf changes, but only its unrelated download entry -- the
  # zsh-plugin record for zsh-completions is untouched.
  sed -i.bak 's/1\.0\.0/1.0.1/' "$REPO_DIR/dependencies.conf"
  sed -i.bak "s/$OLD_COMPLETIONS/$NEW_COMPLETIONS/" "$REPO_DIR/common/completion.zsh"
  rm -f "$REPO_DIR/dependencies.conf.bak" "$REPO_DIR/common/completion.zsh.bak"
  commit_head

  verify_diff >/dev/null 2>&1 || status=$?
  [[ "$status" -ne 0 ]] || fail "A Zsh pin change without a matching manifest zsh-plugin change should be rejected"
}

test_rejects_missing_manifest_change() {
  local status=0

  setup_test_home
  trap teardown_test_home EXIT
  init_release_repo
  sed -i.bak "s/$OLD_FZF_TAB/$NEW_FZF_TAB/" "$REPO_DIR/common/interactive.zsh"
  rm -f "$REPO_DIR/common/interactive.zsh.bak"
  commit_head

  verify_diff >/dev/null 2>&1 || status=$?
  [[ "$status" -ne 0 ]] || fail "A release PR must include a dependencies.conf change"
}

run_test() {
  local test_name="$1"

  "$test_name"
  printf 'PASS: %s\n' "$test_name"
}

main() {
  local test_name
  local failures=0

  while IFS= read -r test_name; do
    if ! run_test "$test_name"; then
      failures=$((failures + 1))
    fi
  done < <(declare -F | awk '{print $3}' | grep '^test_' | sort)

  if ((failures > 0)); then
    printf '%d test(s) failed\n' "$failures" >&2
    return 1
  fi
}

main "$@"
