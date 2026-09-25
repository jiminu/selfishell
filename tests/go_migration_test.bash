#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
source "$ROOT_DIR/tests/test_helper.bash"

case "${*:-}" in
  '' | '--phase baseline') PHASE=baseline ;;
  '--phase config') PHASE=config ;;
  *)
    printf 'Usage: bash tests/go_migration_test.bash [--phase baseline|config]\n' >&2
    exit 2
    ;;
esac

SNAPSHOT_SCRIPT="$ROOT_DIR/tests/fixtures/go_migration/snapshot.py"
REFERENCE_COMMIT=3bbbfa0346ee74eb47f31a81ec666340a5ef6018
LEGACY_COMMIT=d025710338036f1f54b948f1f3e5c17a0b3f7e38

archive_migration_source() {
  local commit="$1" destination="$2"
  if ! git -C "$ROOT_DIR" cat-file -e "$commit^{commit}" 2>/dev/null; then
    printf 'Missing migration reference %s. Fetch repository history before running tests (CI: fetch-depth: 0).\n' "$commit" >&2
    return 1
  fi
  mkdir -p "$destination"
  git -C "$ROOT_DIR" archive "$commit" | tar -xf - -C "$destination"
}

prepare_migration_tools() {
  local name executable
  mkdir -p "$TEST_ROOT/tools" "$TEST_ROOT/tmp"
  # An allowlist prevents accidental package installation or caller mise use.
  for name in bash env cat chmod cp mv rm mkdir ln readlink dirname basename \
    find sed awk grep cut sort head tail tr cksum cmp dd uname touch mktemp rmdir wc; do
    executable="$(PATH=/usr/bin:/bin:/usr/sbin:/sbin command -v "$name")"
    ln -s "$executable" "$TEST_ROOT/tools/$name"
  done
  cp "$ROOT_DIR/tests/fixtures/go_migration/date.bash" "$TEST_ROOT/tools/date"
  chmod +x "$TEST_ROOT/tools/date"
}

run_migration_scenario() {
  local executable="$1" scenario="$2" captures="$3" python
  python="$(command -v python3)"
  env -i HOME="$HOME" XDG_CONFIG_HOME="$HOME/.config" \
    XDG_DATA_HOME="$HOME/.local/share" XDG_STATE_HOME="$HOME/.local/state" \
    XDG_CACHE_HOME="$HOME/.cache" PATH="$TEST_ROOT/tools" \
    SHELL=/bin/zsh TMPDIR="$TEST_ROOT/tmp" LC_ALL=C TZ=UTC \
    /bin/bash "$ROOT_DIR/tests/fixtures/go_migration/scenario.bash" \
    "$executable" "$scenario" "$captures" "$python" "$SNAPSHOT_SCRIPT"
}

run_config_scenario() {
  local executable="$1" scenario="$2" captures="$3" platform="$4" system proc python
  python="$(command -v python3)"
  system=Darwin
  if [[ "$platform" != macos ]]; then
    system=Linux
  fi
  proc="$TEST_ROOT/proc-version"
  if [[ "$platform" == ubuntu-wsl ]]; then
    proc="$TEST_ROOT/proc-version-wsl"
  fi
  env -i HOME="$HOME" XDG_CONFIG_HOME="$HOME/.config" \
    XDG_DATA_HOME="$HOME/.local/share" XDG_STATE_HOME="$HOME/.local/state" \
    XDG_CACHE_HOME="$HOME/.cache" PATH="$TEST_ROOT/tools" \
    SHELL=/bin/zsh TMPDIR="$TEST_ROOT/tmp" LC_ALL=C TZ=UTC \
    SELFISHELL_TEST_SYSTEM_NAME="$system" \
    SELFISHELL_TEST_OS_RELEASE_FILE="$TEST_ROOT/os-release" \
    SELFISHELL_TEST_PROC_VERSION_FILE="$proc" \
    /bin/bash "$ROOT_DIR/tests/fixtures/go_migration/config_scenario.bash" \
    "$executable" "$scenario" "$captures" "$python" "$SNAPSHOT_SCRIPT"
}

compare_migration_captures() {
  local expected actual
  if ! diff -qr "$1" "$2"; then
    for expected in "$1"/*.stderr "$1"/*.stdout; do
      [[ -f "$expected" ]] || continue
      actual="$2/${expected##*/}"
      if [[ -f "$actual" ]] && ! cmp -s "$expected" "$actual"; then
        diff -u "$expected" "$actual" || true
      fi
    done
    return 1
  fi
}

snapshot_home() {
  python3 -B "$SNAPSHOT_SCRIPT" "$HOME"
}

assert_snapshot_changed() {
  snapshot_home >"$TEST_ROOT/after.json"
  ! cmp -s "$TEST_ROOT/before.json" "$TEST_ROOT/after.json" || fail "$1"
}

test_snapshot_detects_bytes_modes_links_and_missing_paths() {
  printf 'a\r\n\000b' >"$HOME/file"
  chmod 0640 "$HOME/file"
  ln -s file "$HOME/link"
  snapshot_home >"$TEST_ROOT/before.json"

  printf 'a\n\000b' >"$HOME/file"
  assert_snapshot_changed 'Snapshot hid a CRLF change'
  printf 'a\r\n\000b\n' >"$HOME/file"
  assert_snapshot_changed 'Snapshot hid a final newline change'
  printf 'a\r\n\000b' >"$HOME/file"
  chmod 0600 "$HOME/file"
  assert_snapshot_changed 'Snapshot hid a permission change'
  chmod 0640 "$HOME/file"
  rm "$HOME/link"
  ln -s missing "$HOME/link"
  assert_snapshot_changed 'Snapshot hid a changed or dangling link'
  rm "$HOME/link"
  cp "$HOME/file" "$HOME/link"
  assert_snapshot_changed 'Snapshot hid a replaced path type'
  rm "$HOME/link"
  assert_snapshot_changed 'Snapshot hid a missing path'
}

test_snapshot_does_not_follow_links_or_read_special_files() {
  mkdir "$TEST_ROOT/outside"
  printf 'outside\n' >"$TEST_ROOT/outside/file"
  ln -s "$TEST_ROOT/outside" "$HOME/directory-link"
  ln -s . "$HOME/cycle"
  mkfifo "$HOME/pipe"
  snapshot_home >"$TEST_ROOT/before.json"
  printf 'changed\n' >"$TEST_ROOT/outside/file"
  snapshot_home >"$TEST_ROOT/after.json"
  cmp -s "$TEST_ROOT/before.json" "$TEST_ROOT/after.json" ||
    fail 'Snapshot followed a link outside the observed tree'
}

test_snapshot_detects_added_backups_and_state_changes() {
  mkdir -p "$HOME/.local/state/selfishell/backups"
  printf '2\nfile\nactive\n/target\n/source\n-\n123:4\n' >"$HOME/.local/state/selfishell/file.state"
  snapshot_home >"$TEST_ROOT/before.json"
  printf 'original\n' >"$HOME/.local/state/selfishell/backups/file.backup.20000101000000"
  assert_snapshot_changed 'Snapshot hid an added backup'
  rm "$HOME/.local/state/selfishell/backups/file.backup.20000101000000"
  printf '2\nfile\npending\n/target\n/source\n-\n123:4\n' >"$HOME/.local/state/selfishell/file.state"
  assert_snapshot_changed 'Snapshot hid a pending state change'
}

test_fixed_reference_matches_itself() {
  local scenario pass
  archive_migration_source "$REFERENCE_COMMIT" "$TEST_ROOT/reference"
  # git archive omits metadata; the CLI uses this marker for source-version output.
  mkdir "$TEST_ROOT/reference/.git"
  prepare_migration_tools
  for scenario in empty existing; do
    for pass in first second; do
      rm -rf "$HOME"
      mkdir -p "$HOME" "$TEST_ROOT/$scenario-$pass"
      run_migration_scenario "$TEST_ROOT/reference/bin/selfishell" "$scenario" "$TEST_ROOT/$scenario-$pass"
    done
    compare_migration_captures "$TEST_ROOT/$scenario-first" "$TEST_ROOT/$scenario-second"
  done
}

test_comparison_rejects_output_status_and_filesystem_changes() {
  local name status
  mkdir "$TEST_ROOT/expected" "$TEST_ROOT/actual"
  for name in stdout stderr status home.json; do
    printf 'original\n' >"$TEST_ROOT/expected/$name"
  done
  for name in stdout stderr status home.json; do
    cp "$TEST_ROOT/expected/"* "$TEST_ROOT/actual/"
    printf 'changed\n' >"$TEST_ROOT/actual/$name"
    status=0
    compare_migration_captures "$TEST_ROOT/expected" "$TEST_ROOT/actual" >/dev/null 2>&1 || status=$?
    [[ "$status" == 1 ]] || fail "Comparison did not report a difference in $name (status $status)"
  done
}

test_missing_reference_fails_without_fetching_history() {
  local status=0
  git init -q "$TEST_ROOT/shallow"
  ROOT_DIR="$TEST_ROOT/shallow" archive_migration_source "$REFERENCE_COMMIT" "$TEST_ROOT/export" \
    >"$TEST_ROOT/stdout" 2>"$TEST_ROOT/stderr" || status=$?
  [[ "$status" == 1 ]] || fail 'Missing history did not fail'
  [[ ! -e "$TEST_ROOT/export" ]] || fail 'Missing history created a partial reference'
  grep -Fq 'fetch-depth: 0' "$TEST_ROOT/stderr" || fail 'Missing history did not explain CI setup'
}

test_legacy_release_is_reproducible_and_runs_on_native_host() {
  local platform architecture artifact expected actual
  local release_root="$TEST_ROOT/releases" source_root="$TEST_ROOT/legacy-source"
  local prefix="$HOME/.local"

  archive_migration_source "$LEGACY_COMMIT" "$source_root"
  bash "$source_root/scripts/build-release.sh" --version 1.3.1 --output "$TEST_ROOT/first-build" >/dev/null
  bash "$source_root/scripts/build-release.sh" --version 1.3.1 --output "$TEST_ROOT/second-build" >/dev/null
  compare_migration_captures "$TEST_ROOT/first-build" "$TEST_ROOT/second-build"
  assert_file_content 1.3.1 "$TEST_ROOT/first-build/VERSION"
  for platform in linux macos; do
    for architecture in amd64 arm64; do
      artifact="selfishell-1.3.1-$platform-$architecture.tar.gz"
      [[ -f "$TEST_ROOT/first-build/$artifact" ]] || fail "Missing old-release archive: $artifact"
      expected="$(awk -v name="$artifact" '$2 == name { print $1 }' "$TEST_ROOT/first-build/SHA256SUMS")"
      actual="$(fixture_sha256 "$TEST_ROOT/first-build/$artifact")"
      [[ "$expected" == "$actual" ]] || fail "Invalid old-release checksum: $artifact"
    done
  done

  mkdir -p "$release_root/download/v1.3.1"
  cp "$TEST_ROOT/first-build/"* "$release_root/download/v1.3.1/"
  # Do not simulate another architecture: the bootstrap must select this host.
  env -i HOME="$HOME" PATH=/usr/bin:/bin:/usr/sbin:/sbin TMPDIR="$TEST_ROOT" \
    SELFISHELL_RELEASE_ROOT="file://$release_root" \
    /bin/bash "$source_root/install.sh" --version 1.3.1 --prefix "$prefix" >/dev/null
  rm -rf "$source_root"
  [[ "$("$prefix/bin/selfishell" version)" == 'selfishell 1.3.1' ]] ||
    fail 'Old installed release depends on its removed source'
  assert_symlink_to selfishell "$prefix/bin/sfs"
  prepare_migration_tools
  mkdir "$TEST_ROOT/captures"
  run_migration_scenario "$prefix/bin/selfishell" existing "$TEST_ROOT/captures"
  env -i HOME="$HOME" PATH="$TEST_ROOT/tools" SHELL=/bin/zsh \
    "$prefix/bin/selfishell" uninstall --restore --purge --yes >/dev/null
  [[ ! -e "$prefix/bin/selfishell" && ! -L "$prefix/bin/selfishell" ]] ||
    fail 'Old-release purge left the CLI installed'
  printf 'Native legacy archive executed: %s/%s\n' "$(uname -s)" "$(uname -m)"
}

config_test_candidate_matches_fixed_reference() {
  local scenario implementation platform
  [[ -x "$ROOT_DIR/.build/selfishell" ]] || fail 'Build the native Go candidate before the config phase'
  archive_migration_source "$REFERENCE_COMMIT" "$TEST_ROOT/release"
  mkdir "$TEST_ROOT/release/.git"
  cp "$TEST_ROOT/release/bin/selfishell" "$TEST_ROOT/reference-cli"
  cp "$TEST_ROOT/release/packages.conf" "$TEST_ROOT/reference-packages"
  cp "$TEST_ROOT/release/dependencies.conf" "$TEST_ROOT/reference-dependencies"
  prepare_migration_tools
  printf 'ID=ubuntu\n' >"$TEST_ROOT/os-release"
  printf 'Linux\n' >"$TEST_ROOT/proc-version"
  printf 'Linux microsoft WSL2\n' >"$TEST_ROOT/proc-version-wsl"
  for platform in macos ubuntu ubuntu-wsl; do
    for scenario in empty existing custom changed-file changed-link changed-block pending late-preflight malformed-package; do
      for implementation in bash go; do
        rm -rf "$HOME"
        mkdir -p "$HOME" "$TEST_ROOT/$platform-$scenario-$implementation"
        cp "$TEST_ROOT/reference-packages" "$TEST_ROOT/release/packages.conf"
        cp "$TEST_ROOT/reference-dependencies" "$TEST_ROOT/release/dependencies.conf"
        if [[ "$implementation" == bash ]]; then
          cp "$TEST_ROOT/reference-cli" "$TEST_ROOT/release/bin/selfishell"
        else
          cp "$ROOT_DIR/.build/selfishell" "$TEST_ROOT/release/bin/selfishell"
        fi
        chmod +x "$TEST_ROOT/release/bin/selfishell"
        run_config_scenario "$TEST_ROOT/release/bin/selfishell" "$scenario" \
          "$TEST_ROOT/$platform-$scenario-$implementation" "$platform"
      done
      compare_migration_captures "$TEST_ROOT/$platform-$scenario-bash" \
        "$TEST_ROOT/$platform-$scenario-go"
    done
  done
}

config_test_purge_matches_fixed_reference() {
  local implementation prefix="$TEST_ROOT/prefix" release
  prepare_migration_tools
  for implementation in bash go; do
    rm -rf "$HOME" "$prefix" "$TEST_ROOT/purge-export"
    mkdir -p "$HOME" "$prefix/bin" "$prefix/share/selfishell/releases" \
      "$TEST_ROOT/purge-$implementation"
    archive_migration_source "$REFERENCE_COMMIT" "$TEST_ROOT/purge-export"
    release="$prefix/share/selfishell/releases/1.0"
    cp -R "$TEST_ROOT/purge-export" "$release"
    rm -rf "$TEST_ROOT/purge-export"
    mkdir "$release/.git"
    if [[ "$implementation" == go ]]; then
      cp "$ROOT_DIR/.build/selfishell" "$release/bin/selfishell"
    fi
    ln -s releases/1.0 "$prefix/share/selfishell/current"
    ln -s ../share/selfishell/current/bin/selfishell "$prefix/bin/selfishell"
    ln -s selfishell "$prefix/bin/sfs"
    run_config_scenario "$prefix/bin/selfishell" purge \
      "$TEST_ROOT/purge-$implementation" macos
  done
  compare_migration_captures "$TEST_ROOT/purge-bash" "$TEST_ROOT/purge-go"
}

config_test_rejects_unsupported_phase() {
  local rc=0
  bash "$ROOT_DIR/tests/go_migration_test.bash" --phase unsupported \
    >"$TEST_ROOT/stdout" 2>"$TEST_ROOT/stderr" || rc=$?
  [[ "$rc" == 2 && ! -s "$TEST_ROOT/stdout" ]] || fail 'Unsupported phase was accepted'
  grep -Fq '[--phase baseline|config]' "$TEST_ROOT/stderr" ||
    fail 'Unsupported phase usage did not name supported phases'
}

config_test_invalid_dependencies_fail_before_mutation() {
  archive_migration_source "$REFERENCE_COMMIT" "$TEST_ROOT/release"
  mkdir "$TEST_ROOT/release/.git" "$TEST_ROOT/captures"
  cp "$ROOT_DIR/.build/selfishell" "$TEST_ROOT/release/bin/selfishell"
  prepare_migration_tools
  run_config_scenario "$TEST_ROOT/release/bin/selfishell" malformed-dependency \
    "$TEST_ROOT/captures" macos
  grep -Fq 'invalid manifest record' "$TEST_ROOT/captures/malformed-install.stderr" ||
    fail 'Malformed dependency was not reported'
}

if [[ "$PHASE" == baseline ]]; then
  run_discovered_tests setup_test_home teardown_test_home
else
  run_test_isolated config_test_rejects_unsupported_phase setup_test_home teardown_test_home
  run_test_isolated config_test_invalid_dependencies_fail_before_mutation setup_test_home teardown_test_home
  run_test_isolated config_test_candidate_matches_fixed_reference setup_test_home teardown_test_home
  run_test_isolated config_test_purge_matches_fixed_reference setup_test_home teardown_test_home
fi
