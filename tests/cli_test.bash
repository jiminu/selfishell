#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

source "$ROOT_DIR/tests/test_helper.bash"

test_help_is_default_command() {
  local output
  output="$(bash "$ROOT_DIR/bin/selfishell")"

  [[ "$output" == *'Usage:'* ]] || fail "Default command should show help"
  [[ "$output" == *'selfishell <command>'* ]] || fail "Help should use the canonical command"
}

test_version_reads_version_file() {
  local expected
  local output

  expected="$(<"$ROOT_DIR/VERSION")"
  output="$(bash "$ROOT_DIR/bin/selfishell" version)"
  [[ "$output" == "selfishell $expected" ]] || fail "Unexpected version output: $output"
}

test_check_script_discovers_all_shell_files() {
  local file
  local fixture="lib/.check_discovery_fixture_$$.sh"
  local bash_files=()
  local zsh_files=()

  # Mirrors scripts/check.sh's own discovery snippet so this test fails if
  # that discovery mechanism regresses back to a hand-maintained list that
  # can silently omit files (as it once did for lib/resources.sh,
  # lib/profile_scan.sh, and common/aliases-editor.zsh).
  while IFS= read -r file; do
    bash_files+=("$file")
  done < <(
    cd "$ROOT_DIR" && {
      printf '%s\n' bin/selfishell install.sh
      find lib scripts tests -type f \( -name '*.sh' -o -name '*.bash' \)
    } | sort -u
  )
  while IFS= read -r file; do
    zsh_files+=("$file")
  done < <(
    cd "$ROOT_DIR" && {
      printf '%s\n' mac/.zshrc ubuntu/.zshrc
      find common -type f -name '*.zsh'
    } | sort -u
  )

  for file in lib/resources.sh lib/profile_scan.sh; do
    printf '%s\n' "${bash_files[@]}" | grep -Fqx "$file" ||
      fail "check.sh discovery did not include: $file"
  done
  printf '%s\n' "${zsh_files[@]}" | grep -Fqx common/aliases-editor.zsh ||
    fail "check.sh discovery did not include: common/aliases-editor.zsh"

  local fixture_path="$ROOT_DIR/$fixture"
  # shellcheck disable=SC2064 # intentionally expanded now, as a safety net
  # in case fail() below exits before the explicit cleanup at the end.
  trap "rm -f '$fixture_path'" EXIT
  printf '#!/usr/bin/env bash\ntrue\n' >"$fixture_path"

  bash_files=()
  while IFS= read -r file; do
    bash_files+=("$file")
  done < <(
    cd "$ROOT_DIR" && {
      printf '%s\n' bin/selfishell install.sh
      find lib scripts tests -type f \( -name '*.sh' -o -name '*.bash' \)
    } | sort -u
  )
  printf '%s\n' "${bash_files[@]}" | grep -Fqx "$fixture" ||
    fail "check.sh discovery did not pick up a newly added lib/*.sh file"

  rm -f "$fixture_path"
  trap - EXIT
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

  output="$(SELFISHELL_RELEASE_ROOT="file://$release_root" bash "$ROOT_DIR/bin/selfishell" version --available)"

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
  output="$(bash "$ROOT_DIR/bin/selfishell" unknown 2>&1)"
  status=$?
  set -e

  [[ "$status" -eq 2 ]] || fail "Unknown command should return exit code 2"
  [[ "$output" == *'Unknown command: unknown'* ]] || fail "Missing unknown command error"
}

test_removed_self_update_command_is_rejected() {
  local output
  local status

  set +e
  output="$(bash "$ROOT_DIR/bin/selfishell" self-update 2>&1)"
  status=$?
  set -e

  [[ "$status" -eq 2 ]] || fail "Removed self-update command should return exit code 2"
  [[ "$output" == *'Unknown command: self-update'* ]] ||
    fail "Removed self-update command should be reported as unknown"
}

test_removed_legacy_install_command_is_rejected() {
  local status

  set +e
  bash "$ROOT_DIR/bin/selfishell" legacy-install >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -eq 2 ]] || fail "Removed legacy-install command should return usage error"
}

test_update_rejects_conflicting_scopes() {
  local status

  set +e
  bash "$ROOT_DIR/bin/selfishell" update --cli-only --tools-only >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -eq 2 ]] || fail "Conflicting update scopes should return exit code 2"
}

test_update_rejects_version_for_tools_only() {
  local status

  set +e
  bash "$ROOT_DIR/bin/selfishell" update --tools-only --version 0.2.0 >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -eq 2 ]] || fail "Tools-only version selection should return exit code 2"
}

test_update_validates_semantic_versions() {
  local output status version

  output="$(bash "$ROOT_DIR/bin/selfishell" update --cli-only \
    --version 1.2.3-alpha.1.x-7 --dry-run)"
  [[ "$output" == *'Would update Selfishell CLI to 1.2.3-alpha.1.x-7'* ]] ||
    fail "CLI update rejected a valid prerelease"

  for version in 01.2.3 1.02.3 1.2.3-alpha..1 1.2.3-alpha.01; do
    set +e
    output="$(bash "$ROOT_DIR/bin/selfishell" update --cli-only \
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
  output="$(bash "$ROOT_DIR/bin/selfishell" update --cli-only --version 9.9.9 --yes 2>&1)"
  status=$?
  set -e

  [[ "$status" -eq 1 ]] || fail "Failed CLI update should return exit code 1"
  [[ "$output" == *'This command requires a versioned Selfishell installation.'* ]] ||
    fail "Failed CLI update did not report the installation requirement"
}

test_doctor_rejects_unsupported_platform() {
  local output
  local status

  setup_test_home
  printf 'ID=fedora\n' >"$TEST_ROOT/os-release"
  printf 'Linux version 6.8.0\n' >"$TEST_ROOT/proc-version"

  set +e
  output="$(
    SELFISHELL_TEST_SYSTEM_NAME=Linux \
      SELFISHELL_TEST_MACHINE_ARCH=x86_64 \
      SELFISHELL_TEST_OS_RELEASE_FILE="$TEST_ROOT/os-release" \
      SELFISHELL_TEST_PROC_VERSION_FILE="$TEST_ROOT/proc-version" \
      bash "$ROOT_DIR/bin/selfishell" doctor 2>&1
  )"
  status=$?
  set -e

  teardown_test_home
  [[ "$status" -eq 1 ]] || fail "Unsupported platform should return exit code 1"
  [[ "$output" == *'Ubuntu is the only supported native Linux distribution.'* ]] ||
    fail "Doctor should provide an actionable platform message"
}

test_doctor_reports_preserved_legacy_runtime_managers() {
  local output

  setup_test_home
  mkdir -p "$HOME/.nvm" "$HOME/.pyenv" "$HOME/.local/state/selfishell" "$TEST_ROOT/bin"
  printf 'developer\n' >"$HOME/.local/state/selfishell/profile"
  printf 'ID=ubuntu\n' >"$TEST_ROOT/os-release"
  printf 'Linux version 6.8.0\n' >"$TEST_ROOT/proc-version"
  printf '#!/usr/bin/env bash\nexit 0\n' >"$TEST_ROOT/bin/apt"
  chmod +x "$TEST_ROOT/bin/apt"

  set +e
  output="$(
    PATH="$TEST_ROOT/bin:/usr/bin:/bin" \
      SELFISHELL_TEST_SYSTEM_NAME=Linux \
      SELFISHELL_TEST_MACHINE_ARCH=x86_64 \
      SELFISHELL_TEST_OS_RELEASE_FILE="$TEST_ROOT/os-release" \
      SELFISHELL_TEST_PROC_VERSION_FILE="$TEST_ROOT/proc-version" \
      bash "$ROOT_DIR/bin/selfishell" doctor 2>&1
  )"
  set -e

  [[ "$output" == *"Legacy runtime manager detected: $HOME/.nvm (preserved; mise is active)"* ]] ||
    fail "Doctor did not report preserved NVM data"
  [[ "$output" == *"Legacy runtime manager detected: $HOME/.pyenv (preserved; mise is active)"* ]] ||
    fail "Doctor did not report preserved pyenv data"
  teardown_test_home
}

test_doctor_does_not_require_compiler_for_minimal_profile() {
  local output

  setup_test_home
  mkdir -p "$HOME/.local/state/selfishell" "$TEST_ROOT/bin"
  printf 'minimal\n' >"$HOME/.local/state/selfishell/profile"
  printf 'ID=ubuntu\n' >"$TEST_ROOT/os-release"
  printf 'Linux version 6.8.0\n' >"$TEST_ROOT/proc-version"
  printf '#!/usr/bin/env bash\nexit 0\n' >"$TEST_ROOT/bin/apt"
  chmod +x "$TEST_ROOT/bin/apt"

  set +e
  output="$(
    PATH="$TEST_ROOT/bin:/usr/bin:/bin" \
      SELFISHELL_TEST_SYSTEM_NAME=Linux \
      SELFISHELL_TEST_MACHINE_ARCH=x86_64 \
      SELFISHELL_TEST_OS_RELEASE_FILE="$TEST_ROOT/os-release" \
      SELFISHELL_TEST_PROC_VERSION_FILE="$TEST_ROOT/proc-version" \
      bash "$ROOT_DIR/bin/selfishell" doctor 2>&1
  )"
  set -e

  [[ "$output" != *'C compiler:'* ]] || fail "Minimal profile should not require a C compiler"
  teardown_test_home
}

test_commands_reject_extra_arguments() {
  local status

  set +e
  bash "$ROOT_DIR/bin/selfishell" version extra >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -eq 2 ]] || fail "Extra arguments should return exit code 2"
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
