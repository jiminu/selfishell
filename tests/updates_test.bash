#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/tests/test_helper.bash"
source "$ROOT_DIR/lib/common.sh"
source "$ROOT_DIR/lib/paths.sh"
source "$ROOT_DIR/lib/dependencies.sh"

setup_update_home() {
  setup_test_home
  export XDG_STATE_HOME="$HOME/.local/state"
  export SELFISHELL_TEST_SYSTEM_NAME=Linux
  export SELFISHELL_TEST_MACHINE_ARCH=x86_64
  export SELFISHELL_TEST_OS_RELEASE_FILE="$TEST_ROOT/os-release"
  export SELFISHELL_TEST_PROC_VERSION_FILE="$TEST_ROOT/proc-version"
  printf 'ID=ubuntu\n' >"$SELFISHELL_TEST_OS_RELEASE_FILE"
  printf 'Linux version 6.8.0\n' >"$SELFISHELL_TEST_PROC_VERSION_FILE"
}

teardown_update_home() {
  unset XDG_CONFIG_HOME XDG_STATE_HOME SELFISHELL_DEPENDENCIES_FILE
  unset SELFISHELL_TEST_SYSTEM_NAME SELFISHELL_TEST_MACHINE_ARCH
  unset SELFISHELL_TEST_OS_RELEASE_FILE SELFISHELL_TEST_PROC_VERSION_FILE
  teardown_test_home
}

run_dependency_install() {
  local dependency="$1"

  bash -c '
    source "$1/lib/common.sh"
    source "$1/lib/paths.sh"
    source "$1/lib/dependencies.sh"
    dependency_install "$2" linux amd64
  ' _ "$ROOT_DIR" "$dependency"
}

test_tools_update_synchronizes_profile_packages() {
  local output
  export XDG_CONFIG_HOME="$HOME/.config"
  mkdir -p "$XDG_STATE_HOME/selfishell"
  printf 'minimal\n' >"$XDG_STATE_HOME/selfishell/profile"

  output="$(bash "$ROOT_DIR/bin/selfishell" update --tools-only --dry-run)"
  [[ "$output" == *'Would install required apt packages:'* ]] ||
    fail "Tools update did not synchronize package-manager packages"
  [[ "$output" == *'git'* ]] || fail "Tools update did not include the current profile packages"
  [[ "$output" != *'Neovim plugins'* ]] || fail "Minimal tools update included Neovim plugin setup"
}

test_tools_update_skip_packages_avoids_package_operations() {
  local output
  export XDG_CONFIG_HOME="$HOME/.config"
  mkdir -p "$XDG_STATE_HOME/selfishell"
  printf 'minimal\n' >"$XDG_STATE_HOME/selfishell/profile"

  output="$(bash "$ROOT_DIR/bin/selfishell" update --tools-only --skip-packages --dry-run)"
  [[ "$output" == *'Skipping package and tool installation.'* ]] ||
    fail "--skip-packages did not report skipping package and tool installation: $output"
  [[ "$output" != *'apt packages'* ]] ||
    fail "--skip-packages still touched apt packages: $output"
}

test_tools_update_reports_unchanged_summary_without_per_resource_noise() {
  local output
  export XDG_CONFIG_HOME="$HOME/.config"
  mkdir -p "$XDG_STATE_HOME/selfishell"
  printf 'minimal\n' >"$XDG_STATE_HOME/selfishell/profile"

  bash "$ROOT_DIR/bin/selfishell" update --tools-only --skip-packages --yes >/dev/null
  output="$(bash "$ROOT_DIR/bin/selfishell" update --tools-only --skip-packages --yes)"

  [[ "$output" != *'Unchanged:'* ]] ||
    fail "Reapplying unmodified configuration printed a per-resource Unchanged line: $output"
  [[ "$output" == *'items unchanged.'* ]] ||
    fail "Reapplying unmodified configuration did not report an unchanged summary: $output"
}

test_download_dependency_is_checksum_verified_and_recorded() {
  local payload checksum output
  payload="$TEST_ROOT/tool"
  printf '#!/bin/sh\nprintf tool-1.0\\n\n' >"$payload"
  checksum="$(fixture_sha256 "$payload")"
  export SELFISHELL_DEPENDENCIES_FILE="$TEST_ROOT/dependencies.conf"
  printf 'download tool 1.0 linux amd64 file://%s %s .local/bin/tool raw\n' "$payload" "$checksum" >"$SELFISHELL_DEPENDENCIES_FILE"

  output="$(run_dependency_install tool)"
  [[ "$output" == *'Installed approved dependency: tool 1.0'* ]] || fail "Dependency install was not reported"
  assert_file_content '1.0' "$XDG_STATE_HOME/selfishell/dependencies/tool"
  [[ -x "$HOME/.local/bin/tool" ]] || fail "Verified tool was not installed"
}

test_checksum_failure_preserves_existing_managed_tool() {
  local status
  mkdir -p "$HOME/.local/bin" "$XDG_STATE_HOME/selfishell/dependencies"
  printf 'old tool\n' >"$HOME/.local/bin/tool"
  printf '0.9\n' >"$XDG_STATE_HOME/selfishell/dependencies/tool"
  printf 'new tool\n' >"$TEST_ROOT/tool"
  export SELFISHELL_DEPENDENCIES_FILE="$TEST_ROOT/dependencies.conf"
  printf 'download tool 1.0 linux amd64 file://%s %064d .local/bin/tool raw\n' "$TEST_ROOT/tool" 0 >"$SELFISHELL_DEPENDENCIES_FILE"

  set +e
  run_dependency_install tool >/dev/null 2>&1
  status=$?
  set -e
  [[ "$status" -ne 0 ]] || fail "Invalid checksum should fail"
  assert_file_content 'old tool' "$HOME/.local/bin/tool"
  assert_file_content '0.9' "$XDG_STATE_HOME/selfishell/dependencies/tool"
}

test_download_move_failure_does_not_report_success() {
  local status
  mkdir -p "$HOME/.local/bin"
  printf '#!/bin/sh\nprintf tool-1.0\\n\n' >"$TEST_ROOT/tool"
  local checksum
  checksum="$(fixture_sha256 "$TEST_ROOT/tool")"
  export SELFISHELL_DEPENDENCIES_FILE="$TEST_ROOT/dependencies.conf"
  printf 'download tool 1.0 linux amd64 file://%s %s .local/bin/tool raw\n' "$TEST_ROOT/tool" "$checksum" >"$SELFISHELL_DEPENDENCIES_FILE"
  chmod 0555 "$HOME/.local/bin"

  set +e
  run_dependency_install tool >/dev/null 2>&1
  status=$?
  set -e
  chmod 0755 "$HOME/.local/bin"

  [[ "$status" -ne 0 ]] || fail "A failed move must not report success"
  [[ ! -e "$XDG_STATE_HOME/selfishell/dependencies/tool" ]] || fail "A failed move must not be recorded as installed"
  [[ ! -e "$HOME/.local/bin/tool" ]] || fail "A failed move must not leave a partial target"
}

test_write_version_failure_does_not_report_success() {
  local payload checksum output status
  local fake_bin
  payload="$TEST_ROOT/tool"
  printf '#!/bin/sh\nprintf tool-1.0\\n\n' >"$payload"
  checksum="$(fixture_sha256 "$payload")"
  export SELFISHELL_DEPENDENCIES_FILE="$TEST_ROOT/dependencies.conf"
  printf 'download tool 1.0 linux amd64 file://%s %s .local/bin/tool raw\n' "$payload" "$checksum" >"$SELFISHELL_DEPENDENCIES_FILE"

  fake_bin="$TEST_ROOT/fakebin"
  mkdir -p "$fake_bin"
  cat >"$fake_bin/mv" <<'EOF'
#!/usr/bin/env bash
for argument in "$@"; do
  case "$argument" in
    */selfishell/dependencies/tool.tmp.*) exit 1 ;;
  esac
done
exec /bin/mv "$@"
EOF
  chmod +x "$fake_bin/mv"

  set +e
  output="$(PATH="$fake_bin:$PATH" run_dependency_install tool 2>&1)"
  status=$?
  set -e

  [[ "$status" -ne 0 ]] || fail "A forced dependency-version-write failure should propagate as an error"
  [[ "$output" != *'Installed approved dependency'* ]] ||
    fail "A forced dependency-version-write failure printed a success message"
  [[ -x "$HOME/.local/bin/tool" ]] ||
    fail "The downloaded tool should still be usable even though its version record failed"
  [[ "$(find "$XDG_STATE_HOME/selfishell/dependencies" -maxdepth 1 -name 'tool.tmp.*' 2>/dev/null | wc -l)" -eq 0 ]] ||
    fail "A forced dependency-version-write failure left a temporary file behind"

  output="$(run_dependency_install tool)"
  [[ "$output" == *'Externally installed; preserving'* ]] ||
    fail "Retrying after removing the forced failure did not behave as expected: $output"
}

test_dependency_temporary_directory_creation_failure_does_not_report_success() {
  local payload checksum output status
  local fake_bin="$TEST_ROOT/fakebin"
  payload="$TEST_ROOT/tool"
  printf '#!/bin/sh\nprintf tool-1.0\\n\n' >"$payload"
  checksum="$(fixture_sha256 "$payload")"
  export SELFISHELL_DEPENDENCIES_FILE="$TEST_ROOT/dependencies.conf"
  printf 'download tool 1.0 linux amd64 file://%s %s .local/bin/tool raw\n' "$payload" "$checksum" >"$SELFISHELL_DEPENDENCIES_FILE"

  mkdir -p "$fake_bin"
  # Only the dependency-install temporary directory pattern is forced to
  # fail; every other mktemp invocation (dependency_write_version, etc.)
  # must still reach the real mktemp.
  cat >"$fake_bin/mktemp" <<'EOF'
#!/usr/bin/env bash
for argument in "$@"; do
  case "$argument" in
    */selfishell-dependency.*) exit 1 ;;
  esac
done
exec /usr/bin/mktemp "$@"
EOF
  chmod +x "$fake_bin/mktemp"
  cat >"$fake_bin/curl" <<'EOF'
#!/usr/bin/env bash
printf 'called\n' >>"$HOME/curl-calls"
exit 1
EOF
  chmod +x "$fake_bin/curl"

  set +e
  output="$(PATH="$fake_bin:$PATH" run_dependency_install tool 2>&1)"
  status=$?
  set -e

  [[ "$status" -ne 0 ]] || fail "A failed dependency temporary-directory creation should propagate as an error"
  [[ ! -e "$HOME/curl-calls" ]] || fail "A failed mktemp still attempted a download"
  [[ ! -e "$HOME/.local/bin/tool" ]] || fail "A failed mktemp left a partial target"
  [[ ! -e "$XDG_STATE_HOME/selfishell/dependencies/tool" ]] || fail "A failed mktemp recorded a dependency version"
  [[ "$output" != *'Installed approved dependency'* ]] || fail "A failed mktemp printed a success message"
}

test_managed_download_valid_target_is_already_approved() {
  local payload checksum output
  payload="$TEST_ROOT/tool"
  printf '#!/bin/sh\nprintf tool-1.0\\n\n' >"$payload"
  checksum="$(fixture_sha256 "$payload")"
  export SELFISHELL_DEPENDENCIES_FILE="$TEST_ROOT/dependencies.conf"
  printf 'download tool 1.0 linux amd64 file://%s %s .local/bin/tool raw\n' "$payload" "$checksum" >"$SELFISHELL_DEPENDENCIES_FILE"
  mkdir -p "$HOME/.local/bin" "$XDG_STATE_HOME/selfishell/dependencies"
  printf 'existing-install-marker\n' >"$HOME/.local/bin/tool"
  chmod 0755 "$HOME/.local/bin/tool"
  printf '1.0\n' >"$XDG_STATE_HOME/selfishell/dependencies/tool"

  output="$(run_dependency_install tool)"
  [[ -z "$output" ]] ||
    fail "An already-approved dependency printed unexpected output: $output"
  assert_file_content 'existing-install-marker' "$HOME/.local/bin/tool"
}

test_dependency_install_already_approved_increments_unchanged_count() {
  local payload checksum
  payload="$TEST_ROOT/tool"
  printf '#!/bin/sh\nprintf tool-1.0\\n\n' >"$payload"
  checksum="$(fixture_sha256 "$payload")"
  export SELFISHELL_DEPENDENCIES_FILE="$TEST_ROOT/dependencies.conf"
  printf 'download tool 1.0 linux amd64 file://%s %s .local/bin/tool raw\n' "$payload" "$checksum" >"$SELFISHELL_DEPENDENCIES_FILE"
  mkdir -p "$HOME/.local/bin" "$XDG_STATE_HOME/selfishell/dependencies"
  printf 'existing-install-marker\n' >"$HOME/.local/bin/tool"
  chmod 0755 "$HOME/.local/bin/tool"
  printf '1.0\n' >"$XDG_STATE_HOME/selfishell/dependencies/tool"

  unset SELFISHELL_UNCHANGED_COUNT
  dependency_install tool linux amd64
  [[ "${SELFISHELL_UNCHANGED_COUNT:-}" == 1 ]] ||
    fail "An already-approved dependency did not increment SELFISHELL_UNCHANGED_COUNT: ${SELFISHELL_UNCHANGED_COUNT:-<unset>}"
}

test_managed_download_version_bump_reports_updated() {
  local payload checksum output
  payload="$TEST_ROOT/tool"
  printf '#!/bin/sh\nprintf tool-2.0\\n\n' >"$payload"
  checksum="$(fixture_sha256 "$payload")"
  export SELFISHELL_DEPENDENCIES_FILE="$TEST_ROOT/dependencies.conf"
  printf 'download tool 2.0 linux amd64 file://%s %s .local/bin/tool raw\n' "$payload" "$checksum" >"$SELFISHELL_DEPENDENCIES_FILE"
  mkdir -p "$HOME/.local/bin" "$XDG_STATE_HOME/selfishell/dependencies"
  printf 'old-1.0-install\n' >"$HOME/.local/bin/tool"
  chmod 0755 "$HOME/.local/bin/tool"
  printf '1.0\n' >"$XDG_STATE_HOME/selfishell/dependencies/tool"

  output="$(run_dependency_install tool)"
  [[ "$output" == *'Updated approved dependency: tool 2.0'* ]] ||
    fail "A version bump over an already-approved dependency was not reported as updated: $output"
  [[ "$output" != *'Installed approved dependency'* ]] ||
    fail "A version bump must be reported as updated, not a fresh install"
  cmp -s "$payload" "$HOME/.local/bin/tool" || fail "The dependency target was not replaced with the new version"
  assert_file_content '2.0' "$XDG_STATE_HOME/selfishell/dependencies/tool"
}

test_managed_download_broken_target_is_not_already_approved() {
  local payload checksum output shape
  payload="$TEST_ROOT/tool"
  printf '#!/bin/sh\nprintf tool-1.0\\n\n' >"$payload"
  checksum="$(fixture_sha256 "$payload")"
  export SELFISHELL_DEPENDENCIES_FILE="$TEST_ROOT/dependencies.conf"
  printf 'download tool 1.0 linux amd64 file://%s %s .local/bin/tool raw\n' "$payload" "$checksum" >"$SELFISHELL_DEPENDENCIES_FILE"
  mkdir -p "$XDG_STATE_HOME/selfishell/dependencies"
  printf '1.0\n' >"$XDG_STATE_HOME/selfishell/dependencies/tool"

  for shape in non-executable valid-symlink; do
    rm -rf "$HOME/.local/bin/tool"
    mkdir -p "$HOME/.local/bin"
    case "$shape" in
      non-executable)
        printf 'not executable\n' >"$HOME/.local/bin/tool"
        chmod 0644 "$HOME/.local/bin/tool"
        ;;
      valid-symlink)
        # A Selfishell-managed target must never be a symlink, even one that
        # points at a perfectly usable executable.
        ln -s "$payload" "$HOME/.local/bin/tool"
        ;;
    esac

    output="$(run_dependency_install tool)"
    [[ "$output" == *'Installed approved dependency: tool 1.0'* ]] ||
      fail "A same-version, broken ($shape) managed target was not reinstalled: $output"
  done
}

test_managed_symlink_target_recovery_preserves_symlink_destination() {
  local payload checksum output destination
  payload="$TEST_ROOT/tool"
  printf '#!/bin/sh\nprintf tool-1.0\\n\n' >"$payload"
  checksum="$(fixture_sha256 "$payload")"
  export SELFISHELL_DEPENDENCIES_FILE="$TEST_ROOT/dependencies.conf"
  printf 'download tool 1.0 linux amd64 file://%s %s .local/bin/tool raw\n' "$payload" "$checksum" >"$SELFISHELL_DEPENDENCIES_FILE"
  mkdir -p "$XDG_STATE_HOME/selfishell/dependencies"
  printf '1.0\n' >"$XDG_STATE_HOME/selfishell/dependencies/tool"

  # A Selfishell-managed target that has become a symlink to some unrelated
  # user-owned path must be recovered by replacing the symlink itself; the
  # path it points to must never be modified.
  destination="$TEST_ROOT/some-user-file"
  printf 'user owned content\n' >"$destination"
  mkdir -p "$HOME/.local/bin"
  ln -s "$destination" "$HOME/.local/bin/tool"

  output="$(run_dependency_install tool)"
  [[ "$output" == *'Installed approved dependency: tool 1.0'* ]] ||
    fail "A managed symlink target was not recovered: $output"
  [[ -f "$HOME/.local/bin/tool" && ! -L "$HOME/.local/bin/tool" ]] ||
    fail "The recovered managed target is not a real executable file"
  assert_file_content 'user owned content' "$destination"
  [[ "$(find "$HOME/.local/bin" -maxdepth 1 -name 'tool.previous.*' | wc -l)" -eq 0 ]] ||
    fail "A successful managed symlink recovery left a previous-symlink temporary path behind"
}

test_managed_git_valid_target_is_already_approved() {
  local repo="$TEST_ROOT/repo"
  local output
  mkdir -p "$repo"
  git -C "$repo" init --quiet
  git -C "$repo" config user.email test@example.com
  git -C "$repo" config user.name test
  printf 'marker\n' >"$repo/marker"
  git -C "$repo" add marker
  git -C "$repo" commit --quiet -m initial
  git -C "$repo" tag v1.0

  export SELFISHELL_DEPENDENCIES_FILE="$TEST_ROOT/dependencies.conf"
  printf 'git testgit v1.0 linux amd64 %s - .local/share/testgit marker\n' "$repo" >"$SELFISHELL_DEPENDENCIES_FILE"
  run_dependency_install testgit >/dev/null

  output="$(run_dependency_install testgit)"
  [[ -z "$output" ]] ||
    fail "An already-approved git dependency printed unexpected output: $output"
}

test_managed_git_broken_target_is_not_already_approved() {
  local repo="$TEST_ROOT/repo"
  local elsewhere="$TEST_ROOT/testgit-elsewhere"
  local output
  mkdir -p "$repo"
  git -C "$repo" init --quiet
  git -C "$repo" config user.email test@example.com
  git -C "$repo" config user.name test
  printf 'marker\n' >"$repo/marker"
  git -C "$repo" add marker
  git -C "$repo" commit --quiet -m initial
  git -C "$repo" tag v1.0

  export SELFISHELL_DEPENDENCIES_FILE="$TEST_ROOT/dependencies.conf"
  printf 'git testgit v1.0 linux amd64 %s - .local/share/testgit marker\n' "$repo" >"$SELFISHELL_DEPENDENCIES_FILE"
  mkdir -p "$XDG_STATE_HOME/selfishell/dependencies"
  printf 'v1.0\n' >"$XDG_STATE_HOME/selfishell/dependencies/testgit"

  rm -rf "$HOME/.local/share/testgit"
  mkdir -p "$HOME/.local/share/testgit/.git"
  output="$(run_dependency_install testgit)"
  [[ "$output" == *'Installed approved dependency: testgit v1.0'* ]] ||
    fail "A managed git target missing its marker was not reinstalled: $output"

  rm -rf "$HOME/.local/share/testgit"
  mkdir -p "$HOME/.local/share/testgit"
  printf 'marker\n' >"$HOME/.local/share/testgit/marker"
  output="$(run_dependency_install testgit)"
  [[ "$output" == *'Installed approved dependency: testgit v1.0'* ]] ||
    fail "A managed git target missing .git was not reinstalled: $output"

  rm -rf "$HOME/.local/share/testgit"
  mkdir -p "$elsewhere/.git"
  printf 'marker\n' >"$elsewhere/marker"
  ln -s "$elsewhere" "$HOME/.local/share/testgit"
  output="$(run_dependency_install testgit)"
  [[ "$output" == *'Installed approved dependency: testgit v1.0'* ]] ||
    fail "A managed git target that is a symlink was not reinstalled: $output"
  assert_file_content 'marker' "$elsewhere/marker"
}

test_external_download_valid_symlink_to_executable_is_preserved() {
  local output
  export SELFISHELL_DEPENDENCIES_FILE="$TEST_ROOT/dependencies.conf"
  printf 'download tool 1.0 linux amd64 file:///unused %064d .local/bin/tool raw\n' 0 >"$SELFISHELL_DEPENDENCIES_FILE"
  mkdir -p "$HOME/.local/bin"
  printf '#!/bin/sh\nprintf tool-real\\n\n' >"$TEST_ROOT/real-tool"
  chmod 0755 "$TEST_ROOT/real-tool"
  ln -s "$TEST_ROOT/real-tool" "$HOME/.local/bin/tool"

  output="$(run_dependency_install tool)"
  [[ "$output" == *'Externally installed; preserving'* ]] ||
    fail "A valid symlink to an executable external target was not preserved: $output"
  [[ -L "$HOME/.local/bin/tool" ]] || fail "The external symlink was replaced instead of preserved"
}

test_external_download_dangling_symlink_install_fails_without_touching_it() {
  local status output
  export SELFISHELL_DEPENDENCIES_FILE="$TEST_ROOT/dependencies.conf"
  printf 'download tool 1.0 linux amd64 file:///unused %064d .local/bin/tool raw\n' 0 >"$SELFISHELL_DEPENDENCIES_FILE"
  mkdir -p "$HOME/.local/bin"
  ln -s "$TEST_ROOT/missing-target" "$HOME/.local/bin/tool"

  set +e
  output="$(run_dependency_install tool 2>&1)"
  status=$?
  set -e

  [[ "$status" -ne 0 ]] || fail "An external dangling symlink target should not report success"
  [[ -L "$HOME/.local/bin/tool" ]] || fail "The external dangling symlink was not preserved"
  [[ "$(readlink "$HOME/.local/bin/tool")" == "$TEST_ROOT/missing-target" ]] ||
    fail "The external dangling symlink target changed"
  [[ ! -e "$XDG_STATE_HOME/selfishell/dependencies/tool" ]] ||
    fail "An external dangling symlink must not be recorded as a managed dependency"
}

test_external_download_directory_target_install_fails_without_touching_it() {
  local status output
  export SELFISHELL_DEPENDENCIES_FILE="$TEST_ROOT/dependencies.conf"
  printf 'download tool 1.0 linux amd64 file:///unused %064d .local/bin/tool raw\n' 0 >"$SELFISHELL_DEPENDENCIES_FILE"
  mkdir -p "$HOME/.local/bin/tool"
  printf 'user data\n' >"$HOME/.local/bin/tool/user-data"

  set +e
  output="$(run_dependency_install tool 2>&1)"
  status=$?
  set -e

  [[ "$status" -ne 0 ]] || fail "An external directory target should not report success"
  [[ -d "$HOME/.local/bin/tool" ]] || fail "The external directory target was not preserved"
  assert_file_content 'user data' "$HOME/.local/bin/tool/user-data"
  [[ ! -e "$XDG_STATE_HOME/selfishell/dependencies/tool" ]] ||
    fail "An external directory target must not be recorded as a managed dependency"
}

test_download_dependency_replaces_directory_target_without_nesting() {
  local payload checksum output
  payload="$TEST_ROOT/tool"
  printf '#!/bin/sh\nprintf tool-1.0\\n\n' >"$payload"
  checksum="$(fixture_sha256 "$payload")"
  export SELFISHELL_DEPENDENCIES_FILE="$TEST_ROOT/dependencies.conf"
  printf 'download tool 1.0 linux amd64 file://%s %s .local/bin/tool raw\n' "$payload" "$checksum" >"$SELFISHELL_DEPENDENCIES_FILE"

  # Simulates a target that was replaced by a directory (tampering, or a
  # stale leftover) while its recorded version already matches the current
  # approved version: `mv` onto an existing directory renames *into* it
  # instead of replacing it, so without the fix this would silently leave
  # the approved binary unreachable at $HOME/.local/bin/tool/archive while
  # still reporting success. The matching recorded version also means this
  # covers dependency_managed_target_is_valid() correctly rejecting a
  # directory-shaped target instead of taking the Up to date fast path.
  mkdir -p "$XDG_STATE_HOME/selfishell/dependencies"
  printf '1.0\n' >"$XDG_STATE_HOME/selfishell/dependencies/tool"
  mkdir -p "$HOME/.local/bin/tool"
  printf 'leftover\n' >"$HOME/.local/bin/tool/leftover"

  output="$(run_dependency_install tool)"
  [[ "$output" == *'Installed approved dependency: tool 1.0'* ]] ||
    fail "Dependency install over a directory target was not reported"
  [[ -f "$HOME/.local/bin/tool" ]] ||
    fail "mv nested the binary inside the pre-existing directory instead of replacing it"
  [[ -x "$HOME/.local/bin/tool" ]] || fail "Replaced target is not executable"
}

test_download_dependency_directory_target_is_restored_on_activation_failure() {
  local payload checksum output status
  local fake_bin="$TEST_ROOT/fakebin"
  payload="$TEST_ROOT/tool"
  printf '#!/bin/sh\nprintf tool-1.0\\n\n' >"$payload"
  checksum="$(fixture_sha256 "$payload")"
  export SELFISHELL_DEPENDENCIES_FILE="$TEST_ROOT/dependencies.conf"
  printf 'download tool 1.0 linux amd64 file://%s %s .local/bin/tool raw\n' "$payload" "$checksum" >"$SELFISHELL_DEPENDENCIES_FILE"

  mkdir -p "$XDG_STATE_HOME/selfishell/dependencies"
  printf '0.9\n' >"$XDG_STATE_HOME/selfishell/dependencies/tool"
  mkdir -p "$HOME/.local/bin/tool"
  printf 'leftover\n' >"$HOME/.local/bin/tool/leftover"

  mkdir -p "$fake_bin"
  # Only the final activation move (source is always the fixed "archive"
  # download file) is forced to fail; the earlier move-aside of the
  # pre-existing directory target must still succeed via the real mv.
  cat >"$fake_bin/mv" <<'EOF'
#!/usr/bin/env bash
for argument in "$@"; do
  case "$argument" in
    */archive) exit 1 ;;
  esac
done
exec /bin/mv "$@"
EOF
  chmod +x "$fake_bin/mv"

  set +e
  output="$(PATH="$fake_bin:$PATH" run_dependency_install tool 2>&1)"
  status=$?
  set -e

  [[ "$status" -ne 0 ]] ||
    fail "A forced activation-move failure over a directory target should propagate as an error"
  [[ "$output" != *'Installed approved dependency'* ]] ||
    fail "A forced activation-move failure printed a success message"
  [[ -d "$HOME/.local/bin/tool" ]] ||
    fail "The pre-existing directory target was not restored after a failed activation"
  assert_file_content 'leftover' "$HOME/.local/bin/tool/leftover"
}

test_git_dependency_install_recovers_from_stale_previous_target() {
  local repo="$TEST_ROOT/repo"
  mkdir -p "$repo"
  git -C "$repo" init --quiet
  git -C "$repo" config user.email test@example.com
  git -C "$repo" config user.name test
  printf 'marker\n' >"$repo/marker"
  git -C "$repo" add marker
  git -C "$repo" commit --quiet -m initial
  git -C "$repo" tag v1.0

  export SELFISHELL_DEPENDENCIES_FILE="$TEST_ROOT/dependencies.conf"
  printf 'git testgit v1.0 linux amd64 %s - .local/share/testgit marker\n' "$repo" >"$SELFISHELL_DEPENDENCIES_FILE"
  printf 'stale leftover\n' >"$TEST_ROOT/stale-marker"

  # A ".previous.$$"-named directory already occupying the exact path this
  # process would use (as a killed prior run sharing a reused PID would
  # leave behind) must not make the install corrupt or nest its target; it
  # should simply be skipped in favor of an unused path.
  bash -c '
    source "$1/lib/common.sh"
    source "$1/lib/paths.sh"
    source "$1/lib/dependencies.sh"
    mkdir -p "$HOME/.local/share/testgit.previous.$$"
    cp "$2" "$HOME/.local/share/testgit.previous.$$/marker"
    dependency_install testgit linux amd64
  ' _ "$ROOT_DIR" "$TEST_ROOT/stale-marker"

  assert_file_content 'v1.0' "$XDG_STATE_HOME/selfishell/dependencies/testgit"
  assert_file_content 'marker' "$HOME/.local/share/testgit/marker"
}

test_git_checkout_failure_preserves_existing_managed_tool() {
  local status
  local repo="$TEST_ROOT/repo"
  mkdir -p "$repo"
  git -C "$repo" init --quiet
  git -C "$repo" config user.email test@example.com
  git -C "$repo" config user.name test
  printf 'marker\n' >"$repo/marker"
  git -C "$repo" add marker
  git -C "$repo" commit --quiet -m initial
  git -C "$repo" tag v1.0

  export SELFISHELL_DEPENDENCIES_FILE="$TEST_ROOT/dependencies.conf"
  printf 'git testgit v1.0 linux amd64 %s - .local/share/testgit marker\n' "$repo" >"$SELFISHELL_DEPENDENCIES_FILE"
  run_dependency_install testgit >/dev/null
  assert_file_content 'v1.0' "$XDG_STATE_HOME/selfishell/dependencies/testgit"
  assert_file_content 'marker' "$HOME/.local/share/testgit/marker"

  printf 'git testgit v9.9-missing-tag linux amd64 %s - .local/share/testgit marker\n' "$repo" >"$SELFISHELL_DEPENDENCIES_FILE"
  set +e
  run_dependency_install testgit >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -ne 0 ]] || fail "An unresolvable checkout ref must not report success"
  assert_file_content 'v1.0' "$XDG_STATE_HOME/selfishell/dependencies/testgit"
  assert_file_content 'marker' "$HOME/.local/share/testgit/marker"
}

run_discovered_tests setup_update_home teardown_update_home
