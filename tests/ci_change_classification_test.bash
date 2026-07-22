#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/tests/test_helper.bash"

setup_classification_repo() {
  setup_test_home
  TEST_REPO="$TEST_ROOT/repo"
  mkdir -p "$TEST_REPO/common/nvim" "$TEST_REPO/docs"
  git -C "$TEST_REPO" init -q
  git -C "$TEST_REPO" config user.name 'Selfishell Tests'
  git -C "$TEST_REPO" config user.email 'tests@selfishell.invalid'
  printf '%s\n' '# Selfishell' >"$TEST_REPO/README.md"
  printf '%s\n' 'print("initial")' >"$TEST_REPO/common/nvim/init.lua"
  printf '%s\n' '#!/usr/bin/env bash' >"$TEST_REPO/install.sh"
  cat >"$TEST_REPO/dependencies.conf" <<'EOF'
download starship 1.0 linux amd64 https://example.invalid/starship old .local/bin/starship raw
nvim-plugin example/plugin 1111111111111111111111111111111111111111 all all https://example.invalid/plugin.git - - -
EOF
  git -C "$TEST_REPO" add .
  git -C "$TEST_REPO" commit -qm initial
  BASE_SHA="$(git -C "$TEST_REPO" rev-parse HEAD)"
}

classify_committed_change() {
  local head_sha

  git -C "$TEST_REPO" add .
  git -C "$TEST_REPO" commit -qm change
  head_sha="$(git -C "$TEST_REPO" rev-parse HEAD)"
  (
    cd "$TEST_REPO"
    bash "$ROOT_DIR/scripts/classify-ci-changes.sh" "$BASE_SHA" "$head_sha"
  )
}

assert_classification() {
  local output="$1"
  local expected_runtime="$2"
  local expected_ubuntu_e2e="$3"

  grep -Fqx "runtime=$expected_runtime" <<<"$output" ||
    fail "Expected runtime=$expected_runtime, got: $output"
  grep -Fqx "ubuntu_container_e2e=$expected_ubuntu_e2e" <<<"$output" ||
    fail "Expected ubuntu_container_e2e=$expected_ubuntu_e2e, got: $output"
}

test_skips_runtime_for_documentation_only_changes() {
  local output

  setup_classification_repo
  printf '%s\n' 'Documentation update.' >"$TEST_REPO/docs/guide.md"
  output="$(classify_committed_change)"
  assert_classification "$output" false false
  teardown_test_home
}

test_skips_ubuntu_e2e_for_neovim_configuration_only_changes() {
  local output

  setup_classification_repo
  printf '%s\n' 'print("updated")' >"$TEST_REPO/common/nvim/init.lua"
  output="$(classify_committed_change)"
  assert_classification "$output" true false
  teardown_test_home
}

test_skips_ubuntu_e2e_for_neovim_dependency_only_changes() {
  local output

  setup_classification_repo
  sed -i.bak 's/1111111111111111111111111111111111111111/2222222222222222222222222222222222222222/' \
    "$TEST_REPO/dependencies.conf"
  rm "$TEST_REPO/dependencies.conf.bak"
  output="$(classify_committed_change)"
  assert_classification "$output" true false
  teardown_test_home
}

test_runs_ubuntu_e2e_for_non_neovim_dependency_changes() {
  local output

  setup_classification_repo
  sed -i.bak 's/download starship 1.0/download starship 2.0/' "$TEST_REPO/dependencies.conf"
  rm "$TEST_REPO/dependencies.conf.bak"
  output="$(classify_committed_change)"
  assert_classification "$output" true true
  teardown_test_home
}

test_runs_ubuntu_e2e_when_neovim_and_runtime_changes_are_mixed() {
  local output

  setup_classification_repo
  printf '%s\n' 'print("updated")' >"$TEST_REPO/common/nvim/init.lua"
  printf '%s\n' '#!/usr/bin/env bash' 'printf "updated\\n"' >"$TEST_REPO/install.sh"
  output="$(classify_committed_change)"
  assert_classification "$output" true true
  teardown_test_home
}

test_falls_back_to_all_checks_when_the_base_is_unavailable() {
  local output

  setup_classification_repo
  output="$({
    cd "$TEST_REPO"
    bash "$ROOT_DIR/scripts/classify-ci-changes.sh" missing HEAD
  })"
  assert_classification "$output" true true
  teardown_test_home
}

test_skips_runtime_for_documentation_only_changes
printf 'PASS: test_skips_runtime_for_documentation_only_changes\n'
test_skips_ubuntu_e2e_for_neovim_configuration_only_changes
printf 'PASS: test_skips_ubuntu_e2e_for_neovim_configuration_only_changes\n'
test_skips_ubuntu_e2e_for_neovim_dependency_only_changes
printf 'PASS: test_skips_ubuntu_e2e_for_neovim_dependency_only_changes\n'
test_runs_ubuntu_e2e_for_non_neovim_dependency_changes
printf 'PASS: test_runs_ubuntu_e2e_for_non_neovim_dependency_changes\n'
test_runs_ubuntu_e2e_when_neovim_and_runtime_changes_are_mixed
printf 'PASS: test_runs_ubuntu_e2e_when_neovim_and_runtime_changes_are_mixed\n'
test_falls_back_to_all_checks_when_the_base_is_unavailable
printf 'PASS: test_falls_back_to_all_checks_when_the_base_is_unavailable\n'
