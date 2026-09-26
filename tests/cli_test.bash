#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

source "$ROOT_DIR/tests/test_helper.bash"
source "$ROOT_DIR/tests/cli_runner.bash"

test_help_is_default_command() {
  local output
  output="$(run_selfishell)"

  [[ "$output" == *'Usage:'* ]] || fail "Default command should show help"
  [[ "$output" == *'selfishell <command>'* ]] || fail "Help should use the canonical command"
}

test_version_reports_development_from_source_checkout() {
  local output

  output="$(bash "$ROOT_DIR/bin/selfishell" version)"
  [[ "$output" == 'selfishell development' ]] || fail "Unexpected source-checkout version output: $output"
}

test_help_and_local_version_skip_full_cli_loading() {
  local help_trace
  local version_trace

  help_trace="$(bash -x "$ROOT_DIR/bin/selfishell" help 2>&1 >/dev/null)"
  version_trace="$(bash -x "$ROOT_DIR/bin/selfishell" version 2>&1 >/dev/null)"

  [[ "$help_trace" != *'/lib/paths.sh'* ]] || fail "Help eagerly loaded the full CLI"
  [[ "$version_trace" != *'/lib/paths.sh'* ]] || fail "Local version eagerly loaded the full CLI"
}

test_version_available_reads_release_metadata() {
  local release_root output

  setup_test_home
  release_root="$TEST_ROOT/releases"
  mkdir -p "$release_root/latest/download"
  printf '1.2.3\n' >"$release_root/latest/download/VERSION"

  output="$(SELFISHELL_RELEASE_ROOT="file://$release_root" run_selfishell version --available)"

  [[ "$output" == 1.2.3 ]] || fail "Available release version was not reported"
  teardown_test_home
}

test_sfs_runs_same_cli() {
  local canonical
  local shorthand

  canonical="$(bash "$ROOT_DIR/bin/selfishell" version)"
  shorthand="$(bash "$ROOT_DIR/bin/sfs" version)"
  [[ "$shorthand" == "$canonical" ]] || fail "sfs must invoke the canonical CLI"
}

test_cli_resolves_external_symlink() {
  local output

  setup_test_home
  mkdir -p "$TEST_ROOT/bin"
  ln -s "$ROOT_DIR/bin/selfishell" "$TEST_ROOT/bin/selfishell"
  output="$(bash "$TEST_ROOT/bin/selfishell" version)"
  teardown_test_home

  [[ "$output" == "$(bash "$ROOT_DIR/bin/selfishell" version)" ]] ||
    fail "CLI should resolve its release root through an external symlink"
}

test_unknown_command_returns_usage_error() {
  local output
  local status

  set +e
  output="$(run_selfishell unknown 2>&1)"
  status=$?
  set -e

  [[ "$status" -eq 2 ]] || fail "Unknown command should return exit code 2"
  [[ "$output" == *'Unknown command: unknown'* ]] || fail "Missing unknown command error"
}

test_update_help_explains_package_upgrade_policy() {
  local output

  output="$(run_selfishell update --help)"
  [[ "$output" == *'left at their current version'* ]] ||
    fail "update --help does not explain that apt/Homebrew packages are not upgraded: $output"
}

test_update_rejects_conflicting_scopes() {
  local status

  set +e
  run_selfishell update --cli-only --tools-only >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -eq 2 ]] || fail "Conflicting update scopes should return exit code 2"
}

test_update_rejects_version_for_tools_only() {
  local status

  set +e
  run_selfishell update --tools-only --version 0.2.0 >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -eq 2 ]] || fail "Tools-only version selection should return exit code 2"
}

test_update_validates_semantic_versions() {
  local output status version

  output="$(run_selfishell update --cli-only \
    --version 1.2.3-alpha.1.x-7 --dry-run)"
  [[ "$output" == *'Would update Selfishell CLI to 1.2.3-alpha.1.x-7'* ]] ||
    fail "CLI update rejected a valid prerelease"

  # An empty value must not fall back to the latest release.
  for version in 01.2.3 1.02.3 1.2.3-alpha..1 1.2.3-alpha.01 '' v; do
    set +e
    output="$(run_selfishell update --cli-only \
      --version "$version" --dry-run 2>&1)"
    status=$?
    set -e
    [[ "$status" -eq 2 ]] || fail "CLI update accepted invalid version: $version"
    [[ "$output" == *'Invalid semantic version'* ]] ||
      fail "CLI update did not explain invalid version: $version"
  done
}

test_update_propagates_cli_install_failure() {
  local output
  local status

  set +e
  output="$(run_selfishell update --cli-only --version 9.9.9 --yes 2>&1)"
  status=$?
  set -e

  [[ "$status" -eq 1 ]] || fail "Failed CLI update should return exit code 1"
  [[ "$output" == *'This command requires a versioned Selfishell installation.'* ]] ||
    fail "Failed CLI update did not report the installation requirement"
}

test_cli_resolves_root_from_every_invocation_form() {
  local expected link_dir

  setup_test_home
  expected='selfishell development'
  link_dir="$TEST_ROOT/links"
  mkdir -p "$link_dir"
  ln -s "$ROOT_DIR/bin/selfishell" "$link_dir/direct"
  ln -s direct "$link_dir/chained"

  [[ "$(cd "$ROOT_DIR/bin" && bash selfishell version)" == "$expected" ]] ||
    fail "A bare relative invocation did not resolve the Selfishell root"
  [[ "$(bash "$ROOT_DIR/bin/selfishell" version)" == "$expected" ]] ||
    fail "An absolute invocation did not resolve the Selfishell root"
  [[ "$(bash "$link_dir/direct" version)" == "$expected" ]] ||
    fail "A symlinked invocation did not resolve the Selfishell root"
  [[ "$(bash "$link_dir/chained" version)" == "$expected" ]] ||
    fail "A chained symlink invocation did not resolve the Selfishell root"
  teardown_test_home
}

test_commands_reject_extra_arguments() {
  local status

  set +e
  run_selfishell version extra >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -eq 2 ]] || fail "Extra arguments should return exit code 2"
}

run_discovered_tests '' teardown_test_home
