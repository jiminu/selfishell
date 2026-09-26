#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

source "$ROOT_DIR/tests/test_helper.bash"

RELEASE_FIXTURE_ROOT=""
RELEASE_FIXTURE_VERSION=0.2.2

native_release_archive_name() {
  local platform architecture
  case "$(uname -s)" in
    Darwin) platform=macos ;;
    Linux) platform=linux ;;
    *) fail 'Native release tests require Linux or macOS' ;;
  esac
  case "$(uname -m)" in
    arm64 | aarch64) architecture=arm64 ;;
    x86_64 | amd64) architecture=amd64 ;;
    *) fail 'Native release tests require AMD64 or ARM64' ;;
  esac
  printf 'selfishell-%s-%s-%s.tar.gz\n' "$1" "$platform" "$architecture"
}

setup_release_fixture() {
  local version
  local next_version=0.2.3
  local prerelease_version=0.3.0-beta.2

  version="$RELEASE_FIXTURE_VERSION"
  RELEASE_FIXTURE_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/selfishell-release-fixture.XXXXXX")"
  mkdir -p "$RELEASE_FIXTURE_ROOT/artifacts" \
    "$RELEASE_FIXTURE_ROOT/next-artifacts" \
    "$RELEASE_FIXTURE_ROOT/prerelease-artifacts"
  bash "$ROOT_DIR/scripts/build-release.sh" --version "$version" \
    --output "$RELEASE_FIXTURE_ROOT/artifacts" >/dev/null
  bash "$ROOT_DIR/scripts/build-release.sh" --version "$next_version" \
    --output "$RELEASE_FIXTURE_ROOT/next-artifacts" >/dev/null
  bash "$ROOT_DIR/scripts/build-release.sh" --version "$prerelease_version" \
    --output "$RELEASE_FIXTURE_ROOT/prerelease-artifacts" >/dev/null
}

teardown_release_fixture() {
  if [[ -n "$RELEASE_FIXTURE_ROOT" && -d "$RELEASE_FIXTURE_ROOT" ]]; then
    rm -rf "$RELEASE_FIXTURE_ROOT"
  fi
}

setup_release_home() {
  local version
  local next_version=0.2.3
  local prerelease_version=0.3.0-beta.2

  setup_test_home
  version="$RELEASE_FIXTURE_VERSION"
  export SELFISHELL_RELEASE_ROOT="file://$TEST_ROOT/releases"
  # Execute host-native archives; configuration detection below remains simulated.
  SELFISHELL_BOOTSTRAP_OS="$(uname -s)"
  SELFISHELL_BOOTSTRAP_ARCH="$(uname -m)"
  export SELFISHELL_BOOTSTRAP_OS SELFISHELL_BOOTSTRAP_ARCH
  export XDG_CONFIG_HOME="$HOME/.config"
  export XDG_STATE_HOME="$HOME/.local/state"
  export SELFISHELL_TEST_SYSTEM_NAME=Linux
  export SELFISHELL_TEST_MACHINE_ARCH="$SELFISHELL_BOOTSTRAP_ARCH"
  export SELFISHELL_TEST_OS_RELEASE_FILE="$TEST_ROOT/os-release"
  export SELFISHELL_TEST_PROC_VERSION_FILE="$TEST_ROOT/proc-version"
  printf 'ID=ubuntu\n' >"$SELFISHELL_TEST_OS_RELEASE_FILE"
  printf 'Linux version 6.8.0\n' >"$SELFISHELL_TEST_PROC_VERSION_FILE"

  mkdir -p "$TEST_ROOT/artifacts" "$TEST_ROOT/next-artifacts" "$TEST_ROOT/prerelease-artifacts" \
    "$TEST_ROOT/releases/download/v$version" "$TEST_ROOT/releases/download/v$next_version" \
    "$TEST_ROOT/releases/download/v$prerelease_version" \
    "$TEST_ROOT/releases/latest/download"
  cp -R "$RELEASE_FIXTURE_ROOT/artifacts/." "$TEST_ROOT/artifacts/"
  cp -R "$RELEASE_FIXTURE_ROOT/next-artifacts/." "$TEST_ROOT/next-artifacts/"
  cp -R "$RELEASE_FIXTURE_ROOT/prerelease-artifacts/." "$TEST_ROOT/prerelease-artifacts/"
  cp "$TEST_ROOT/artifacts"/* "$TEST_ROOT/releases/download/v$version/"
  cp "$TEST_ROOT/next-artifacts"/* "$TEST_ROOT/releases/download/v$next_version/"
  cp "$TEST_ROOT/prerelease-artifacts"/* "$TEST_ROOT/releases/download/v$prerelease_version/"
  cp "$TEST_ROOT/next-artifacts/VERSION" "$TEST_ROOT/releases/latest/download/VERSION"
}

teardown_release_home() {
  unset SELFISHELL_RELEASE_ROOT SELFISHELL_RELEASE_TAGS_API_URL
  unset SELFISHELL_BOOTSTRAP_OS SELFISHELL_BOOTSTRAP_ARCH
  unset XDG_CONFIG_HOME XDG_STATE_HOME
  unset SELFISHELL_CURL_CONNECT_TIMEOUT SELFISHELL_CURL_LOW_SPEED_LIMIT
  unset SELFISHELL_CURL_LOW_SPEED_TIME SELFISHELL_CURL_METADATA_MAX_TIME
  unset SELFISHELL_TEST_SYSTEM_NAME SELFISHELL_TEST_MACHINE_ARCH
  unset SELFISHELL_TEST_OS_RELEASE_FILE SELFISHELL_TEST_PROC_VERSION_FILE
  teardown_test_home
}

run_bootstrap() {
  bash "$ROOT_DIR/install.sh" --prefix "$TEST_ROOT/prefix" "$@"
}

test_builds_all_platform_architecture_artifacts() {
  local version
  version="$RELEASE_FIXTURE_VERSION"

  for artifact in \
    "selfishell-$version-linux-amd64.tar.gz" \
    "selfishell-$version-linux-arm64.tar.gz" \
    "selfishell-$version-macos-amd64.tar.gz" \
    "selfishell-$version-macos-arm64.tar.gz"; do
    [[ -f "$TEST_ROOT/artifacts/$artifact" ]] || fail "Missing release artifact: $artifact"
  done
  [[ -s "$TEST_ROOT/artifacts/SHA256SUMS" ]] || fail "Missing release checksums"
}

test_release_artifact_uses_config_payload_root() {
  local archive_entries
  local version

  version="$RELEASE_FIXTURE_VERSION"
  archive_entries="$(tar -tzf "$TEST_ROOT/artifacts/selfishell-$version-linux-amd64.tar.gz")"

  grep -Fqx './config/shared/zsh/common.zsh' <<<"$archive_entries" ||
    fail "Release artifact is missing the shared configuration payload"
  grep -Fqx './config/macos/zshrc' <<<"$archive_entries" ||
    fail "Release artifact is missing the macOS configuration payload"
  grep -Fqx './config/ubuntu/zshrc' <<<"$archive_entries" ||
    fail "Release artifact is missing the Ubuntu configuration payload"
  ! grep -Eq '^\./(common|mac|ubuntu)/' <<<"$archive_entries" ||
    fail "Release artifact still includes a legacy configuration payload root"
}

test_release_artifacts_are_reproducible() {
  local version artifact
  local second_output="$TEST_ROOT/reproducible-artifacts"

  version="$RELEASE_FIXTURE_VERSION"
  mkdir -p "$second_output"
  sleep 1
  bash "$ROOT_DIR/scripts/build-release.sh" --version "$version" --output "$second_output" >/dev/null

  for artifact in "$TEST_ROOT/artifacts"/*.tar.gz; do
    cmp -s "$artifact" "$second_output/$(basename "$artifact")" ||
      fail "Release artifact is not reproducible: $(basename "$artifact")"
  done
  cmp -s "$TEST_ROOT/artifacts/SHA256SUMS" "$second_output/SHA256SUMS" ||
    fail "Reproducible artifacts produced different checksums"
}

test_installs_exact_version_and_cli_links() {
  local version platform architecture artifact
  version="$RELEASE_FIXTURE_VERSION"
  case "$(uname -s)" in
    Darwin) platform=macos ;;
    Linux) platform=linux ;;
  esac
  case "$(uname -m)" in
    arm64 | aarch64) architecture=arm64 ;;
    x86_64 | amd64) architecture=amd64 ;;
  esac
  # Only the executable host archive is available; cross-platform payloads
  # must not be selected merely because today's Bash payloads are identical.
  for artifact in "$TEST_ROOT/releases/download/v$version/"*.tar.gz; do
    [[ "${artifact##*/}" == "selfishell-$version-$platform-$architecture.tar.gz" ]] || rm "$artifact"
  done

  run_bootstrap --version "$version" >/dev/null

  assert_symlink_to "releases/$version" "$TEST_ROOT/prefix/share/selfishell/current"
  assert_symlink_to "$TEST_ROOT/prefix/share/selfishell/current/bin/selfishell" "$TEST_ROOT/prefix/bin/selfishell"
  assert_symlink_to selfishell "$TEST_ROOT/prefix/bin/sfs"
  [[ "$("$TEST_ROOT/prefix/bin/selfishell" version)" == "selfishell $version" ]] ||
    fail "Installed CLI reports the wrong version"
}

# A directory-symlinked release path must be rejected the same way on first
# install as it is on update: -d alone would follow the symlink and accept
# whatever it points to as this version's release.
test_bootstrap_rejects_symlinked_release_directory() {
  local version status

  version="$RELEASE_FIXTURE_VERSION"
  mkdir -p "$TEST_ROOT/elsewhere/bin" "$TEST_ROOT/prefix/share/selfishell/releases"
  printf '#!/usr/bin/env bash\nexit 0\n' >"$TEST_ROOT/elsewhere/bin/selfishell"
  chmod +x "$TEST_ROOT/elsewhere/bin/selfishell"
  printf '%s\n' "$version" >"$TEST_ROOT/elsewhere/VERSION"
  ln -s "$TEST_ROOT/elsewhere" "$TEST_ROOT/prefix/share/selfishell/releases/$version"

  set +e
  run_bootstrap --version "$version" >"$TEST_ROOT/stdout" 2>"$TEST_ROOT/stderr"
  status=$?
  set -e

  [[ "$status" -ne 0 ]] || fail "bootstrap activated a symlinked release directory"
  grep -Fq 'Release path is not a directory' "$TEST_ROOT/stderr" ||
    fail "bootstrap did not reject the symlinked release directory"
  [[ ! -L "$TEST_ROOT/prefix/share/selfishell/current" ]] ||
    fail "bootstrap activated current despite a symlinked release directory"
  [[ -L "$TEST_ROOT/prefix/share/selfishell/releases/$version" ]] ||
    fail "The symlinked release path was replaced instead of rejected"
}

test_latest_uses_published_version_file() {
  run_bootstrap >/dev/null
  [[ "$(<"$TEST_ROOT/prefix/share/selfishell/current/VERSION")" == 0.2.3 ]] ||
    fail "Latest installation selected the wrong version"
}

test_bootstrap_uses_bounded_curl_policy() {
  local fake_bin="$TEST_ROOT/fakebin"
  mkdir -p "$fake_bin"
  cat >"$fake_bin/curl" <<'EOF'
#!/usr/bin/env bash
printf 'call' >>"$HOME/curl-calls"
printf ' %s' "$@" >>"$HOME/curl-calls"
printf '\n' >>"$HOME/curl-calls"
exec /usr/bin/curl "$@"
EOF
  chmod +x "$fake_bin/curl"

  PATH="$fake_bin:$PATH" run_bootstrap >/dev/null

  grep -Fq -- '--connect-timeout 10' "$HOME/curl-calls" ||
    fail "Bootstrap curl calls did not set a connection timeout"
  grep -Fq -- '--speed-limit 1024' "$HOME/curl-calls" ||
    fail "Bootstrap curl calls did not set a low-speed limit"
  grep -Fq -- '--speed-time 30' "$HOME/curl-calls" ||
    fail "Bootstrap curl calls did not set a low-speed duration"
  grep -Fq -- '--max-time 15' "$HOME/curl-calls" ||
    fail "Bootstrap metadata lookup did not set a total timeout"
  grep -F -- '-o ' "$HOME/curl-calls" | grep -Fvq -- '--max-time' ||
    fail "Bootstrap release download used the metadata total timeout"
}

test_bootstrap_rejects_invalid_curl_policy() {
  local output status version
  version="$RELEASE_FIXTURE_VERSION"

  set +e
  output="$(SELFISHELL_CURL_LOW_SPEED_TIME=invalid \
    run_bootstrap --version "$version" 2>&1)"
  status=$?
  set -e

  [[ "$status" -eq 2 ]] || fail "Invalid bootstrap curl policy should return a usage error"
  [[ "$output" == *'must be positive integers'* ]] ||
    fail "Invalid bootstrap curl policy did not explain the accepted values"
  [[ ! -e "$TEST_ROOT/prefix/share/selfishell/current" ]] ||
    fail "Invalid bootstrap curl policy changed the active release"
}

test_bootstrap_rejects_invalid_semantic_versions() {
  local output status version

  for version in 01.2.3 1.02.3 1.2.3-alpha..1 1.2.3-alpha.01 '' v; do
    set +e
    output="$(run_bootstrap --version "$version" 2>&1)"
    status=$?
    set -e
    [[ "$status" -ne 0 ]] || fail "Bootstrap accepted invalid version: $version"
    [[ "$output" == *'Invalid semantic version'* ]] ||
      fail "Bootstrap did not explain invalid version: $version"
    [[ ! -e "$TEST_ROOT/prefix/share/selfishell/current" ]] ||
      fail "Invalid version changed the active release: $version"
  done
}

test_latest_falls_back_to_published_prerelease() {
  rm "$TEST_ROOT/releases/latest/download/VERSION"
  printf '[{"name":"v0.3.0-beta.2"}]\n' >"$TEST_ROOT/tags-api.json"
  export SELFISHELL_RELEASE_TAGS_API_URL="file://$TEST_ROOT/tags-api.json"

  run_bootstrap >/dev/null

  [[ "$(<"$TEST_ROOT/prefix/share/selfishell/current/VERSION")" == 0.3.0-beta.2 ]] ||
    fail "Prerelease fallback selected the wrong version"
}

test_update_falls_back_to_published_prerelease() {
  local version
  version="$RELEASE_FIXTURE_VERSION"
  run_bootstrap --version "$version" >/dev/null
  rm "$TEST_ROOT/releases/latest/download/VERSION"
  printf '[{"name":"v0.3.0-beta.2"}]\n' >"$TEST_ROOT/tags-api.json"
  export SELFISHELL_RELEASE_TAGS_API_URL="file://$TEST_ROOT/tags-api.json"

  "$TEST_ROOT/prefix/bin/selfishell" update --cli-only --yes >/dev/null

  assert_symlink_to 'releases/0.3.0-beta.2' "$TEST_ROOT/prefix/share/selfishell/current"
}

test_latest_lookup_failure_is_actionable() {
  local output status
  rm "$TEST_ROOT/releases/latest/download/VERSION"

  set +e
  output="$(run_bootstrap 2>&1)"
  status=$?
  set -e

  [[ "$status" -ne 0 ]] || fail "Missing release metadata should fail"
  [[ "$output" == *'Use --version VERSION to select one.'* ]] ||
    fail "Missing release metadata did not provide version guidance"
  [[ "$output" != *'curl:'* ]] || fail "Raw curl errors should not leak from release discovery"
}

test_unpublished_tag_is_not_selected() {
  local status
  rm "$TEST_ROOT/releases/latest/download/VERSION"
  printf '[{"name":"v9.9.9-beta.1"}]\n' >"$TEST_ROOT/tags-api.json"
  export SELFISHELL_RELEASE_TAGS_API_URL="file://$TEST_ROOT/tags-api.json"

  set +e
  run_bootstrap >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -ne 0 ]] || fail "A tag without published VERSION metadata was selected"
}

test_cli_update_and_offline_rollback() {
  local output version
  version="$RELEASE_FIXTURE_VERSION"
  run_bootstrap --version "$version" >/dev/null
  mkdir -p "$TEST_ROOT/prefix/share/selfishell/releases/0.0.1/bin"
  printf '#!/usr/bin/env bash\n' >"$TEST_ROOT/prefix/share/selfishell/releases/0.0.1/bin/selfishell"

  "$TEST_ROOT/prefix/bin/selfishell" update --cli-only --version 0.2.3 --yes >/dev/null
  assert_symlink_to 'releases/0.2.3' "$TEST_ROOT/prefix/share/selfishell/current"
  assert_symlink_to "releases/$version" "$TEST_ROOT/prefix/share/selfishell/previous"
  [[ ! -e "$TEST_ROOT/prefix/share/selfishell/releases/0.0.1" ]] ||
    fail "CLI update did not prune an inactive release"
  [[ -d "$TEST_ROOT/prefix/share/selfishell/releases/$version" ]] ||
    fail "CLI update pruned the rollback release"
  output="$("$TEST_ROOT/prefix/bin/selfishell" status 2>&1)" || true
  [[ "$output" == *"[CLI] Current: 0.2.3 | Rollback: $version"* ]] ||
    fail "Status did not report the retained rollback release"

  SELFISHELL_RELEASE_ROOT='file:///unavailable' \
    "$TEST_ROOT/prefix/bin/selfishell" rollback --yes >/dev/null
  assert_symlink_to "releases/$version" "$TEST_ROOT/prefix/share/selfishell/current"
  assert_symlink_to 'releases/0.2.3' "$TEST_ROOT/prefix/share/selfishell/previous"
  output="$("$TEST_ROOT/prefix/bin/selfishell" status 2>&1)" || true
  [[ "$output" == *"[CLI] Current: $version | Rollback: 0.2.3"* ]] ||
    fail "Status did not update the rollback release after rollback"
}

test_release_validate_extracted_members_accepts_safe_symlinks() {
  local status=0

  bash -c '
    set -euo pipefail
    source "$1/lib/common.sh"
    source "$1/lib/releases.sh"
    mkdir -p "$2/bin"
    printf ok >"$2/bin/selfishell"
    ln -s selfishell "$2/bin/sfs"
    release_validate_extracted_members "$2"
  ' _ "$ROOT_DIR" "$TEST_ROOT/member-fixture-ok" || status=$?

  [[ "$status" -eq 0 ]] || fail "A release archive with a safe sibling symlink was rejected"
}

test_release_validate_extracted_members_rejects_fifo() {
  local status=0

  bash -c '
    set -euo pipefail
    source "$1/lib/common.sh"
    source "$1/lib/releases.sh"
    mkdir -p "$2/bin"
    printf ok >"$2/bin/selfishell"
    mkfifo "$2/bin/evil-fifo"
    ! release_validate_extracted_members "$2"
  ' _ "$ROOT_DIR" "$TEST_ROOT/member-fixture-fifo" || status=$?

  [[ "$status" -eq 0 ]] || fail "A release archive containing a FIFO was accepted"
}

test_release_validate_extracted_members_rejects_traversal_symlink() {
  local status=0

  bash -c '
    set -euo pipefail
    source "$1/lib/common.sh"
    source "$1/lib/releases.sh"
    mkdir -p "$2/bin"
    printf ok >"$2/bin/selfishell"
    ln -s ../../../etc/passwd "$2/bin/evil-link"
    ! release_validate_extracted_members "$2"
  ' _ "$ROOT_DIR" "$TEST_ROOT/member-fixture-traversal" || status=$?

  [[ "$status" -eq 0 ]] || fail "A release archive containing a traversal symlink was accepted"
}

test_release_validate_extracted_members_rejects_dangling_symlink() {
  local status=0

  bash -c '
    set -euo pipefail
    source "$1/lib/common.sh"
    source "$1/lib/releases.sh"
    mkdir -p "$2/bin"
    printf ok >"$2/bin/selfishell"
    ln -s does-not-exist "$2/bin/evil-link"
    ! release_validate_extracted_members "$2"
  ' _ "$ROOT_DIR" "$TEST_ROOT/member-fixture-dangling" || status=$?

  [[ "$status" -eq 0 ]] || fail "A release archive containing a dangling symlink was accepted"
}

test_release_atomic_link_recovers_from_stale_temporary_path() {
  local status=0

  # Simulates debris left by a process killed mid-swap (a killed prior run
  # sharing this same PID): release_atomic_link must not hard-fail just
  # because its usual temporary name is already occupied.
  bash -c '
    set -euo pipefail
    source "$1/lib/common.sh"
    source "$1/lib/releases.sh"
    mkdir -p "$2"
    ln -s target-a "$2/link.tmp.$$"
    release_atomic_link target-b "$2/link"
    [[ "$(readlink "$2/link")" == target-b ]]
  ' _ "$ROOT_DIR" "$TEST_ROOT/atomic-link-fixture" || status=$?

  [[ "$status" -eq 0 ]] || fail "release_atomic_link did not recover from a stale temporary path"
}

test_update_rejects_preexisting_incomplete_release_directory() {
  local version status
  version="$RELEASE_FIXTURE_VERSION"
  run_bootstrap --version "$version" >/dev/null

  mkdir -p "$TEST_ROOT/prefix/share/selfishell/releases/0.2.3/bin"
  printf 'not-selfishell\n' >"$TEST_ROOT/prefix/share/selfishell/releases/0.2.3/bin/selfishell"

  set +e
  "$TEST_ROOT/prefix/bin/selfishell" update --cli-only --version 0.2.3 --yes \
    >"$TEST_ROOT/stdout" 2>"$TEST_ROOT/stderr"
  status=$?
  set -e

  [[ "$status" -ne 0 ]] ||
    fail "update activated an incomplete pre-existing release directory"
  grep -Fq 'Existing release is incomplete' "$TEST_ROOT/stderr" ||
    fail "update did not report the incomplete release directory"
  assert_symlink_to "releases/$version" "$TEST_ROOT/prefix/share/selfishell/current"
}

# A directory-symlinked release path must never be accepted as valid: -x/-r
# would otherwise follow it and treat some other path's contents as this
# version's release.
test_update_rejects_symlinked_release_directory() {
  local version status

  version="$RELEASE_FIXTURE_VERSION"
  run_bootstrap --version "$version" >/dev/null

  mkdir -p "$TEST_ROOT/elsewhere/bin"
  printf '#!/usr/bin/env bash\nexit 0\n' >"$TEST_ROOT/elsewhere/bin/selfishell"
  chmod +x "$TEST_ROOT/elsewhere/bin/selfishell"
  printf '0.2.3\n' >"$TEST_ROOT/elsewhere/VERSION"
  ln -s "$TEST_ROOT/elsewhere" "$TEST_ROOT/prefix/share/selfishell/releases/0.2.3"

  set +e
  "$TEST_ROOT/prefix/bin/selfishell" update --cli-only --version 0.2.3 --yes \
    >"$TEST_ROOT/stdout" 2>"$TEST_ROOT/stderr"
  status=$?
  set -e

  [[ "$status" -ne 0 ]] ||
    fail "update activated a symlinked release directory"
  grep -Fq 'Existing release is incomplete' "$TEST_ROOT/stderr" ||
    fail "update did not reject the symlinked release directory"
  assert_symlink_to "releases/$version" "$TEST_ROOT/prefix/share/selfishell/current"
  [[ -L "$TEST_ROOT/prefix/share/selfishell/releases/0.2.3" ]] ||
    fail "The symlinked release path was replaced instead of rejected"
  [[ "$(<"$TEST_ROOT/elsewhere/VERSION")" == '0.2.3' ]] ||
    fail "The symlink target was modified"
}

# A non-symlink previous path is preserved and only warned about, matching
# release_atomic_link's existing non-fatal contract for the previous link;
# the update as a whole must still succeed.
test_update_preserves_non_link_previous_path_with_a_warning() {
  local version output

  version="$RELEASE_FIXTURE_VERSION"
  run_bootstrap --version "$version" >/dev/null
  printf 'user data\n' >"$TEST_ROOT/prefix/share/selfishell/previous"

  output="$("$TEST_ROOT/prefix/bin/selfishell" update --cli-only --version 0.2.3 --yes 2>&1)"

  assert_symlink_to 'releases/0.2.3' "$TEST_ROOT/prefix/share/selfishell/current"
  [[ "$(<"$TEST_ROOT/prefix/share/selfishell/previous")" == 'user data' ]] ||
    fail "The non-symlink previous path was overwritten instead of preserved"
  [[ "$output" == *'Failed to update the previous release link'* ]] ||
    fail "A preserved non-symlink previous path did not warn: $output"
}

# Run the retained binary so the guard handles a non-link current path;
# the public entrypoint would fail with ENOTDIR before reaching the guard.
test_update_rejects_non_link_current_path() {
  local version status retained_cli

  version="$RELEASE_FIXTURE_VERSION"
  run_bootstrap --version "$version" >/dev/null
  retained_cli="$TEST_ROOT/prefix/share/selfishell/releases/$version/bin/selfishell"
  rm -f "$TEST_ROOT/prefix/share/selfishell/current"
  printf 'user data\n' >"$TEST_ROOT/prefix/share/selfishell/current"

  set +e
  "$retained_cli" update --cli-only --version 0.2.3 --yes \
    >"$TEST_ROOT/stdout" 2>"$TEST_ROOT/stderr"
  status=$?
  set -e

  [[ "$status" -ne 0 ]] || fail "update replaced a non-symlink current path"
  [[ ! -L "$TEST_ROOT/prefix/share/selfishell/current" ]] ||
    fail "current was replaced with a symlink despite being a regular file"
  [[ "$(<"$TEST_ROOT/prefix/share/selfishell/current")" == 'user data' ]] ||
    fail "The non-symlink current path was overwritten instead of preserved"
  [[ "$(<"$TEST_ROOT/stderr")" == *'requires a versioned Selfishell installation'* ]] ||
    fail "update did not fail via the versioned-installation guard: $(<"$TEST_ROOT/stderr")"
}

test_update_tolerates_duplicate_identical_checksum_entry() {
  local version archive_name checksum_file line
  version="$RELEASE_FIXTURE_VERSION"
  run_bootstrap --version "$version" >/dev/null

  # release_install (used by `update`) resolves its platform from the real
  # `uname -s` rather than the SELFISHELL_TEST_SYSTEM_NAME override, so the
  # archive it actually fetches must be named after the real host platform.
  archive_name="$(native_release_archive_name 0.2.3)"
  checksum_file="$TEST_ROOT/releases/download/v0.2.3/SHA256SUMS"
  line="$(awk -v name="$archive_name" '$2 == name' "$checksum_file")"
  printf '%s\n' "$line" >>"$checksum_file"

  "$TEST_ROOT/prefix/bin/selfishell" update --cli-only --version 0.2.3 --yes >/dev/null
  assert_symlink_to 'releases/0.2.3' "$TEST_ROOT/prefix/share/selfishell/current"
}

test_update_rejects_conflicting_duplicate_checksum_entry() {
  local version archive_name checksum_file status
  version="$RELEASE_FIXTURE_VERSION"
  run_bootstrap --version "$version" >/dev/null

  archive_name="$(native_release_archive_name 0.2.3)"
  checksum_file="$TEST_ROOT/releases/download/v0.2.3/SHA256SUMS"
  printf '%064d  %s\n' 0 "$archive_name" >>"$checksum_file"

  set +e
  "$TEST_ROOT/prefix/bin/selfishell" update --cli-only --version 0.2.3 --yes \
    >"$TEST_ROOT/stdout" 2>"$TEST_ROOT/stderr"
  status=$?
  set -e

  [[ "$status" -ne 0 ]] || fail "update accepted a conflicting duplicate checksum entry"
  assert_symlink_to "releases/$version" "$TEST_ROOT/prefix/share/selfishell/current"
}

test_rollback_accepts_v_prefixed_version() {
  local version
  version="$RELEASE_FIXTURE_VERSION"
  run_bootstrap --version "$version" >/dev/null
  "$TEST_ROOT/prefix/bin/selfishell" update --cli-only --version 0.2.3 --yes >/dev/null

  "$TEST_ROOT/prefix/bin/selfishell" rollback "v$version" --yes >/dev/null
  assert_symlink_to "releases/$version" "$TEST_ROOT/prefix/share/selfishell/current"
}

test_rollback_to_currently_active_version_is_noop() {
  local version output
  version="$RELEASE_FIXTURE_VERSION"
  run_bootstrap --version "$version" >/dev/null

  output="$("$TEST_ROOT/prefix/bin/selfishell" rollback "$version" --yes)"
  [[ "$output" == *'Release is already active:'* ]] ||
    fail "Rolling back to the active version should be a no-op"
  assert_symlink_to "releases/$version" "$TEST_ROOT/prefix/share/selfishell/current"
}

test_rollback_rejects_invalid_semver() {
  local version status
  version="$RELEASE_FIXTURE_VERSION"
  run_bootstrap --version "$version" >/dev/null

  for bad_version in 'not-a-version' '1.2' '' v; do
    set +e
    "$TEST_ROOT/prefix/bin/selfishell" rollback "$bad_version" --yes \
      >"$TEST_ROOT/stdout" 2>"$TEST_ROOT/stderr"
    status=$?
    set -e

    [[ "$status" -eq 2 ]] ||
      fail "Invalid rollback version '$bad_version' should exit with usage status 2, got $status"
    grep -Fq 'Invalid semantic version' "$TEST_ROOT/stderr" ||
      fail "Invalid rollback version '$bad_version' did not report an error"
    assert_symlink_to "releases/$version" "$TEST_ROOT/prefix/share/selfishell/current"
  done
}

test_rollback_rejects_path_traversal_version() {
  local version status
  version="$RELEASE_FIXTURE_VERSION"
  run_bootstrap --version "$version" >/dev/null

  for malicious in '../current' '../../tmp' '/absolute/path'; do
    set +e
    "$TEST_ROOT/prefix/bin/selfishell" rollback "$malicious" --yes \
      >"$TEST_ROOT/stdout" 2>"$TEST_ROOT/stderr"
    status=$?
    set -e

    [[ "$status" -ne 0 ]] || fail "rollback accepted a malicious version: '$malicious'"
    [[ -L "$TEST_ROOT/prefix/share/selfishell/current" ]] ||
      fail "rollback with '$malicious' left 'current' as a non-symlink"
    assert_symlink_to "releases/$version" "$TEST_ROOT/prefix/share/selfishell/current"
  done
}

test_rollback_rejects_release_directory_with_mismatched_version_file() {
  local version status
  version="$RELEASE_FIXTURE_VERSION"
  run_bootstrap --version "$version" >/dev/null
  "$TEST_ROOT/prefix/bin/selfishell" update --cli-only --version 0.2.3 --yes >/dev/null

  printf '9.9.9\n' >"$TEST_ROOT/prefix/share/selfishell/releases/$version/VERSION"

  set +e
  "$TEST_ROOT/prefix/bin/selfishell" rollback "$version" --yes \
    >"$TEST_ROOT/stdout" 2>"$TEST_ROOT/stderr"
  status=$?
  set -e

  [[ "$status" -ne 0 ]] ||
    fail "rollback accepted a release directory whose VERSION file does not match its name"
  assert_symlink_to 'releases/0.2.3' "$TEST_ROOT/prefix/share/selfishell/current"
}

test_rollback_rejects_traversal_shaped_previous_link() {
  local version status
  version="$RELEASE_FIXTURE_VERSION"
  run_bootstrap --version "$version" >/dev/null
  "$TEST_ROOT/prefix/bin/selfishell" update --cli-only --version 0.2.3 --yes >/dev/null

  rm "$TEST_ROOT/prefix/share/selfishell/previous"
  ln -s 'releases/../current' "$TEST_ROOT/prefix/share/selfishell/previous"

  set +e
  "$TEST_ROOT/prefix/bin/selfishell" rollback --yes \
    >"$TEST_ROOT/stdout" 2>"$TEST_ROOT/stderr"
  status=$?
  set -e

  [[ "$status" -ne 0 ]] || fail "rollback followed a traversal-shaped previous link"
  assert_symlink_to 'releases/0.2.3' "$TEST_ROOT/prefix/share/selfishell/current"
}

test_rollback_rejects_previous_link_outside_releases() {
  local version status
  version="$RELEASE_FIXTURE_VERSION"
  run_bootstrap --version "$version" >/dev/null
  "$TEST_ROOT/prefix/bin/selfishell" update --cli-only --version 0.2.3 --yes >/dev/null

  rm "$TEST_ROOT/prefix/share/selfishell/previous"
  ln -s '/etc/passwd' "$TEST_ROOT/prefix/share/selfishell/previous"

  set +e
  "$TEST_ROOT/prefix/bin/selfishell" rollback --yes \
    >"$TEST_ROOT/stdout" 2>"$TEST_ROOT/stderr"
  status=$?
  set -e

  [[ "$status" -ne 0 ]] || fail "rollback followed a previous link pointing outside releases/"
  assert_symlink_to 'releases/0.2.3' "$TEST_ROOT/prefix/share/selfishell/current"
}

test_cli_update_to_current_version_preserves_rollback() {
  local output version
  version="$RELEASE_FIXTURE_VERSION"
  run_bootstrap --version "$version" >/dev/null
  "$TEST_ROOT/prefix/bin/selfishell" update --cli-only --version 0.2.3 --yes >/dev/null

  output="$(SELFISHELL_RELEASE_ROOT='file:///unavailable' \
    "$TEST_ROOT/prefix/bin/selfishell" update --cli-only --version 0.2.3 --yes)"

  [[ "$output" == *'already at 0.2.3; skipping CLI update'* ]] ||
    fail "Same-version CLI update was not reported as a no-op"
  assert_symlink_to 'releases/0.2.3' "$TEST_ROOT/prefix/share/selfishell/current"
  assert_symlink_to "releases/$version" "$TEST_ROOT/prefix/share/selfishell/previous"
  [[ -d "$TEST_ROOT/prefix/share/selfishell/releases/$version" ]] ||
    fail "Same-version CLI update pruned the rollback release"
}

test_default_update_reports_up_to_date_without_synchronizing() {
  local output version
  version="$RELEASE_FIXTURE_VERSION"
  run_bootstrap --version "$version" >/dev/null
  "$TEST_ROOT/prefix/bin/selfishell" update --cli-only --version 0.2.3 --yes >/dev/null

  output="$("$TEST_ROOT/prefix/bin/selfishell" update --yes)"

  [[ "$output" == *'Selfishell is up to date (0.2.3).'* ]] ||
    fail "Default update at the latest release did not report up to date: $output"
  [[ "$output" != *'already at'* ]] ||
    fail "Default update printed the CLI-only wording as well as the up-to-date message: $output"
  # No configuration is installed here, so update_tools_and_configuration would have
  # printed this had it run: its absence proves the phase was skipped, not just
  # that the summary line was suppressed.
  [[ "$output" != *'skipping tools and configuration'* ]] ||
    fail "Default update ran the tools/configuration phase despite already being at the latest release: $output"
}

test_update_version_matching_current_is_a_noop() {
  local output version
  version="$RELEASE_FIXTURE_VERSION"
  run_bootstrap --version "$version" >/dev/null

  output="$("$TEST_ROOT/prefix/bin/selfishell" update --version "$version" --yes)"

  [[ "$output" == *"Selfishell is up to date ($version)."* ]] ||
    fail "Default update targeting the already-installed version did not report up to date: $output"
  [[ "$output" != *'skipping tools and configuration'* ]] ||
    fail "Default update ran the tools/configuration phase for an explicit current-version target: $output"
}

test_default_update_skip_packages_is_a_noop_when_current() {
  local output version
  version="$RELEASE_FIXTURE_VERSION"
  run_bootstrap --version "$version" >/dev/null

  output="$("$TEST_ROOT/prefix/bin/selfishell" update --skip-packages --version "$version" --yes)"

  [[ "$output" == *"Selfishell is up to date ($version)."* ]] ||
    fail "--skip-packages did not preserve the default update's up-to-date no-op: $output"
  [[ "$output" != *'Skipping package and tool installation.'* ]] ||
    fail "--skip-packages triggered the tools/configuration phase for an already-current release: $output"
}

test_update_release_move_failure_does_not_corrupt_symlinks() {
  local version
  local fake_bin="$TEST_ROOT/fakebin"
  local status=0

  version="$RELEASE_FIXTURE_VERSION"
  run_bootstrap --version "$version" >/dev/null

  mkdir -p "$fake_bin"
  cat >"$fake_bin/mv" <<'EOF'
#!/usr/bin/env bash
for argument in "$@"; do
  case "$argument" in
    */releases/0.2.3) exit 1 ;;
  esac
done
exec /bin/mv "$@"
EOF
  chmod +x "$fake_bin/mv"

  set +e
  PATH="$fake_bin:$PATH" "$TEST_ROOT/prefix/bin/selfishell" update --cli-only --version 0.2.3 --yes \
    >"$TEST_ROOT/stdout" 2>"$TEST_ROOT/stderr"
  status=$?
  set -e

  ((status != 0)) || fail "A forced release-move failure should propagate as an error"
  assert_symlink_to "releases/$version" "$TEST_ROOT/prefix/share/selfishell/current"
  [[ ! -e "$TEST_ROOT/prefix/share/selfishell/previous" ]] ||
    fail "A forced release-move failure must not create a previous link"
  ! grep -Fq 'Selfishell updated' "$TEST_ROOT/stdout" ||
    fail "A forced release-move failure printed a success message"
  [[ ! -d "$TEST_ROOT/prefix/share/selfishell/releases/0.2.3" ]] ||
    fail "A forced release-move failure must not leave a partial release directory"

  "$TEST_ROOT/prefix/bin/selfishell" update --cli-only --version 0.2.3 --yes >/dev/null
  assert_symlink_to 'releases/0.2.3' "$TEST_ROOT/prefix/share/selfishell/current"
}

test_update_activation_link_failure_does_not_report_success() {
  local version
  local fake_bin="$TEST_ROOT/fakebin"
  local status=0

  version="$RELEASE_FIXTURE_VERSION"
  run_bootstrap --version "$version" >/dev/null

  mkdir -p "$fake_bin"
  cat >"$fake_bin/ln" <<'EOF'
#!/usr/bin/env bash
for argument in "$@"; do
  case "$argument" in
    */share/selfishell/current.tmp.*) exit 1 ;;
  esac
done
exec /bin/ln "$@"
EOF
  chmod +x "$fake_bin/ln"

  set +e
  PATH="$fake_bin:$PATH" "$TEST_ROOT/prefix/bin/selfishell" update --cli-only --version 0.2.3 --yes \
    >"$TEST_ROOT/stdout" 2>"$TEST_ROOT/stderr"
  status=$?
  set -e

  ((status != 0)) || fail "A forced activation-link failure should propagate as an error"
  assert_symlink_to "releases/$version" "$TEST_ROOT/prefix/share/selfishell/current"
  ! grep -Fq 'Selfishell updated' "$TEST_ROOT/stdout" ||
    fail "A forced activation-link failure printed a success message"
  [[ -d "$TEST_ROOT/prefix/share/selfishell/releases/0.2.3" ]] ||
    fail "The downloaded release directory should still be usable for a retry"

  "$TEST_ROOT/prefix/bin/selfishell" update --cli-only --version 0.2.3 --yes >/dev/null
  assert_symlink_to 'releases/0.2.3' "$TEST_ROOT/prefix/share/selfishell/current"
}

test_default_update_skips_missing_configuration_and_updates_cli() {
  local output
  local result_line skip_line
  local version

  version="$RELEASE_FIXTURE_VERSION"
  run_bootstrap --version "$version" >/dev/null

  output="$("$TEST_ROOT/prefix/bin/selfishell" update --version 0.2.3 --yes)"
  [[ "$output" == *'skipping tools and configuration'* ]] ||
    fail "Default update did not skip an uninstalled configuration"
  # The continuation runs from the new release and learns the version it
  # replaced from the previous-release link, so the source version here also
  # proves that link survived the switch.
  [[ "$output" == *"Selfishell updated: $version -> 0.2.3"* ]] ||
    fail "Default update did not report the version transition: $output"
  result_line="$(printf '%s\n' "$output" | awk '/Selfishell updated: / { print NR; exit }')"
  skip_line="$(printf '%s\n' "$output" | awk '/skipping tools and configuration/ { print NR; exit }')"
  [[ -n "$result_line" && -n "$skip_line" && "$skip_line" -lt "$result_line" ]] ||
    fail "Default update did not report its result after continuing with the new CLI"
  assert_symlink_to 'releases/0.2.3' "$TEST_ROOT/prefix/share/selfishell/current"
}

test_cli_only_update_reports_the_version_transition_once() {
  local output occurrences version

  version="$RELEASE_FIXTURE_VERSION"
  run_bootstrap --version "$version" >/dev/null

  output="$("$TEST_ROOT/prefix/bin/selfishell" update --cli-only --version 0.2.3 --yes)"
  [[ "$output" == *"Selfishell updated: $version -> 0.2.3"* ]] ||
    fail "--cli-only did not report the version transition: $output"
  occurrences="$(printf '%s\n' "$output" | grep -c 'Selfishell updated' || true)"
  [[ "$occurrences" == 1 ]] ||
    fail "--cli-only reported the update $occurrences times: $output"
  [[ "$output" != *'CLI updated to'* ]] ||
    fail "--cli-only still printed the release-installation success line: $output"
  assert_symlink_to 'releases/0.2.3' "$TEST_ROOT/prefix/share/selfishell/current"
}

test_update_dry_run_preserves_active_release() {
  local output
  local version

  version="$RELEASE_FIXTURE_VERSION"
  run_bootstrap --version "$version" >/dev/null

  output="$("$TEST_ROOT/prefix/bin/selfishell" update --version 0.2.3 --dry-run)"
  [[ "$output" == *'Would update Selfishell CLI to 0.2.3.'* ]] ||
    fail "Update dry-run did not preview the CLI release"
  assert_symlink_to "releases/$version" "$TEST_ROOT/prefix/share/selfishell/current"
  [[ ! -e "$TEST_ROOT/prefix/share/selfishell/releases/0.2.3" ]] ||
    fail "Update dry-run installed a CLI release"
}

test_checksum_mismatch_preserves_active_release() {
  local version
  local archive
  local active_before
  local status

  version="$RELEASE_FIXTURE_VERSION"
  archive="$TEST_ROOT/releases/download/v$version/$(native_release_archive_name "$version")"
  run_bootstrap --version "$version" >/dev/null
  active_before="$(readlink "$TEST_ROOT/prefix/share/selfishell/current")"
  printf 'corruption' >>"$archive"

  set +e
  run_bootstrap --version "$version" >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -eq 1 ]] || fail "Checksum mismatch should fail"
  [[ "$(readlink "$TEST_ROOT/prefix/share/selfishell/current")" == "$active_before" ]] ||
    fail "Checksum failure changed the active release"
}

test_specific_version_never_falls_back_to_latest() {
  local status

  set +e
  run_bootstrap --version 9.9.9 >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -ne 0 ]] || fail "Missing exact version should fail"
  [[ ! -e "$TEST_ROOT/prefix/share/selfishell/current" ]] ||
    fail "Exact version failure unexpectedly installed latest"
}

test_bootstrap_installs_cli_only_by_default() {
  run_bootstrap >/dev/null

  [[ -x "$TEST_ROOT/prefix/bin/selfishell" ]] || fail "CLI was not installed"
  [[ ! -e "$XDG_CONFIG_HOME/selfishell" ]] || fail "Bootstrap changed user configuration"
  [[ ! -e "$HOME/.bashrc" && ! -e "$HOME/.zshrc" ]] ||
    fail "Default bootstrap changed shell startup files"
}

test_bootstrap_upgrade_retains_rollback_and_prunes_inactive_release() {
  local version
  version="$RELEASE_FIXTURE_VERSION"
  run_bootstrap --version "$version" >/dev/null
  mkdir -p "$TEST_ROOT/prefix/share/selfishell/releases/0.0.1"

  run_bootstrap --version 0.2.3 >/dev/null

  assert_symlink_to 'releases/0.2.3' "$TEST_ROOT/prefix/share/selfishell/current"
  assert_symlink_to "releases/$version" "$TEST_ROOT/prefix/share/selfishell/previous"
  [[ ! -e "$TEST_ROOT/prefix/share/selfishell/releases/0.0.1" ]] ||
    fail "Bootstrap upgrade retained an inactive release"
  SELFISHELL_RELEASE_ROOT='file:///unavailable' \
    "$TEST_ROOT/prefix/bin/selfishell" rollback --yes >/dev/null
  assert_symlink_to "releases/$version" "$TEST_ROOT/prefix/share/selfishell/current"
}

test_bootstrap_same_version_preserves_rollback_release() {
  local version
  version="$RELEASE_FIXTURE_VERSION"
  run_bootstrap --version "$version" >/dev/null
  run_bootstrap --version 0.2.3 >/dev/null
  rm "$TEST_ROOT/prefix/bin/sfs"

  run_bootstrap --version 0.2.3 >/dev/null

  assert_symlink_to 'releases/0.2.3' "$TEST_ROOT/prefix/share/selfishell/current"
  assert_symlink_to "releases/$version" "$TEST_ROOT/prefix/share/selfishell/previous"
  [[ -d "$TEST_ROOT/prefix/share/selfishell/releases/$version" ]] ||
    fail "Same-version bootstrap pruned the rollback release"
  assert_symlink_to selfishell "$TEST_ROOT/prefix/bin/sfs"
}

test_setup_is_explicit_and_can_skip_packages() {
  run_bootstrap --setup --yes --skip-packages >/dev/null

  [[ -f "$HOME/.zshrc" && ! -L "$HOME/.zshrc" ]] || fail "Setup did not create a user-owned .zshrc"
  grep -Fqx '# >>> Selfishell initialize >>>' "$HOME/.zshrc" || fail "Setup did not add the Zsh loader"
  assert_file_content '1' "$XDG_STATE_HOME/selfishell/configured"
}

test_missing_bin_path_prints_actionable_message() {
  local output
  output="$(PATH=/usr/bin:/bin run_bootstrap)"

  [[ "$output" == *"export PATH=\"$TEST_ROOT/prefix/bin:\$PATH\""* ]] ||
    fail "Missing PATH guidance did not include a current-shell command"
  [[ "$output" == *'Add this command to your shell startup file'* ]] ||
    fail "Missing PATH guidance did not explain manual persistent setup"
  [[ "$output" == *"$TEST_ROOT/prefix/bin/selfishell install"* ]] ||
    fail "Missing PATH guidance did not include the absolute CLI command"
}

test_purge_dry_run_preserves_installation() {
  run_bootstrap --setup --skip-packages --yes >/dev/null

  "$TEST_ROOT/prefix/bin/selfishell" uninstall --restore --purge --dry-run >/dev/null

  [[ -x "$TEST_ROOT/prefix/bin/selfishell" ]] || fail "Purge dry-run removed the CLI"
  [[ -f "$HOME/.zshrc" && ! -L "$HOME/.zshrc" ]] || fail "Purge dry-run changed .zshrc"
  grep -Fqx '# >>> Selfishell initialize >>>' "$HOME/.zshrc" || fail "Purge dry-run removed the loader"
  [[ -d "$TEST_ROOT/prefix/share/selfishell" ]] || fail "Purge dry-run removed releases"
}

test_purge_removes_cli_releases_cache_and_state() {
  run_bootstrap --setup --skip-packages --yes >/dev/null
  mkdir -p "$HOME/.cache/selfishell"
  printf 'cache\n' >"$HOME/.cache/selfishell/test"

  "$TEST_ROOT/prefix/bin/selfishell" uninstall --restore --purge --yes >/dev/null

  [[ ! -e "$TEST_ROOT/prefix/bin/selfishell" ]] || fail "Purge retained the CLI link"
  [[ ! -e "$TEST_ROOT/prefix/bin/sfs" ]] || fail "Purge retained the sfs link"
  [[ ! -e "$TEST_ROOT/prefix/share/selfishell" ]] || fail "Purge retained releases"
  [[ ! -e "$XDG_STATE_HOME/selfishell" ]] || fail "Purge retained state"
  [[ ! -e "$HOME/.cache/selfishell" ]] || fail "Purge retained cache"
  [[ -f "$HOME/.zshrc" && ! -s "$HOME/.zshrc" ]] || fail "Purge did not leave an empty user-owned .zshrc"
}

test_purge_keeps_backups_of_modified_files() {
  local output

  run_bootstrap --setup --skip-packages --yes >/dev/null
  mkdir -p "$XDG_STATE_HOME/selfishell/backups"
  printf 'user edit\n' >"$XDG_STATE_HOME/selfishell/backups/vimrc.backup.20260101000000"

  output="$("$TEST_ROOT/prefix/bin/selfishell" uninstall --restore --purge --dry-run)"
  [[ "$output" == *"Would keep backups of modified files: $XDG_STATE_HOME/selfishell/backups"* ]] ||
    fail "Purge dry run did not preview the kept backups: $output"

  output="$("$TEST_ROOT/prefix/bin/selfishell" uninstall --restore --purge --yes)"
  assert_file_content 'user edit' "$XDG_STATE_HOME/selfishell/backups/vimrc.backup.20260101000000"
  [[ "$(find "$XDG_STATE_HOME/selfishell" -mindepth 1 -maxdepth 1)" == "$XDG_STATE_HOME/selfishell/backups" ]] ||
    fail "Purge kept state other than backups"
  [[ "$output" == *"Kept backups of modified files: $XDG_STATE_HOME/selfishell/backups"* ]] ||
    fail "Purge did not report the kept backups: $output"
}

test_uninstall_without_purge_reports_cli_still_installed() {
  local output
  run_bootstrap --setup --skip-packages --yes >/dev/null

  output="$("$TEST_ROOT/prefix/bin/selfishell" uninstall --restore --yes)"

  [[ "$output" == *'The Selfishell CLI is still installed.'* ]] ||
    fail "Non-purge uninstall did not report that the CLI is still installed"
  [[ "$output" == *"selfishell uninstall --purge"* ]] ||
    fail "Non-purge uninstall did not suggest the purge follow-up"
  [[ -x "$TEST_ROOT/prefix/bin/selfishell" ]] || fail "Non-purge uninstall removed the CLI"
}

test_uninstall_purge_reports_final_state_only() {
  local output
  run_bootstrap --setup --skip-packages --yes >/dev/null

  output="$("$TEST_ROOT/prefix/bin/selfishell" uninstall --restore --purge --yes)"

  [[ "$output" != *'The Selfishell CLI is still installed.'* ]] ||
    fail "Purge uninstall falsely reported that the CLI is still installed"
  [[ "$output" != *"selfishell uninstall --purge"* ]] ||
    fail "Purge uninstall suggested running purge again"
  [[ "$output" == *'Selfishell configuration, CLI, releases, cache, and state removed.'* ]] ||
    fail "Purge uninstall did not report the final removed state: $output"
}

test_purge_refuses_non_managed_cli_path_before_uninstall() {
  local status
  run_bootstrap --setup --skip-packages --yes >/dev/null
  rm "$TEST_ROOT/prefix/bin/selfishell"
  ln -s /usr/bin/true "$TEST_ROOT/prefix/bin/selfishell"

  set +e
  bash "$TEST_ROOT/prefix/share/selfishell/current/bin/selfishell" uninstall --restore --purge --yes >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -eq 1 ]] || fail "Purge should reject a non-managed CLI path"
  assert_symlink_to /usr/bin/true "$TEST_ROOT/prefix/bin/selfishell"
  [[ -f "$HOME/.zshrc" && ! -L "$HOME/.zshrc" ]] || fail "Rejected purge changed .zshrc"
  grep -Fqx '# >>> Selfishell initialize >>>' "$HOME/.zshrc" || fail "Rejected purge removed the loader"
}

# sfs is optional: bootstrap leaves another program's sfs alone, so purge must
# not fail on one either.
test_foreign_sfs_is_left_in_place_by_bootstrap_and_purge() {
  local output

  mkdir -p "$TEST_ROOT/prefix/bin"
  printf 'user command\n' >"$TEST_ROOT/prefix/bin/sfs"
  output="$(run_bootstrap --version "$RELEASE_FIXTURE_VERSION")"
  [[ "$output" == *"Leaving $TEST_ROOT/prefix/bin/sfs in place"* ]] ||
    fail "Bootstrap did not report the foreign sfs: $output"
  assert_file_content 'user command' "$TEST_ROOT/prefix/bin/sfs"
  [[ -x "$TEST_ROOT/prefix/bin/selfishell" ]] || fail "Foreign sfs blocked the CLI installation"

  rm "$TEST_ROOT/prefix/bin/sfs"
  ln -s /usr/bin/true "$TEST_ROOT/prefix/bin/sfs"
  run_bootstrap --version 0.2.3 >/dev/null
  assert_symlink_to /usr/bin/true "$TEST_ROOT/prefix/bin/sfs"

  "$TEST_ROOT/prefix/bin/selfishell" uninstall --purge --yes >/dev/null ||
    fail "Purge failed on a foreign sfs"
  assert_symlink_to /usr/bin/true "$TEST_ROOT/prefix/bin/sfs"
  [[ ! -e "$TEST_ROOT/prefix/bin/selfishell" && ! -L "$TEST_ROOT/prefix/bin/selfishell" ]] ||
    fail "Purge left the Selfishell CLI link"
}

# A regular file or another program's link at the CLI path is user data.
test_refuses_to_replace_foreign_cli_path() {
  local kind status

  mkdir -p "$TEST_ROOT/prefix/bin"
  for kind in file link; do
    rm -f "$TEST_ROOT/prefix/bin/selfishell"
    if [[ "$kind" == file ]]; then
      printf 'user file' >"$TEST_ROOT/prefix/bin/selfishell"
    else
      ln -s /usr/bin/true "$TEST_ROOT/prefix/bin/selfishell"
    fi
    set +e
    run_bootstrap --version "$RELEASE_FIXTURE_VERSION" >/dev/null 2>&1
    status=$?
    set -e

    [[ "$status" -eq 1 ]] || fail "A foreign CLI $kind should block installation"
    if [[ "$kind" == file ]]; then
      assert_file_content 'user file' "$TEST_ROOT/prefix/bin/selfishell"
    else
      assert_symlink_to /usr/bin/true "$TEST_ROOT/prefix/bin/selfishell"
    fi
    [[ ! -e "$TEST_ROOT/prefix/share/selfishell/current" ]] ||
      fail "A foreign CLI $kind changed the active release"
  done
}

test_update_does_not_move_back_to_an_older_latest() {
  local output

  run_bootstrap --version 0.3.0-beta.2 >/dev/null
  output="$("$TEST_ROOT/prefix/bin/selfishell" update --cli-only --yes)"
  [[ "$output" == *'Selfishell 0.3.0-beta.2 is newer than the latest release 0.2.3.'* ]] ||
    fail "Update did not explain keeping the newer release: $output"
  assert_symlink_to 'releases/0.3.0-beta.2' "$TEST_ROOT/prefix/share/selfishell/current"

  "$TEST_ROOT/prefix/bin/selfishell" update --cli-only --version 0.2.3 --yes >/dev/null
  assert_symlink_to 'releases/0.2.3' "$TEST_ROOT/prefix/share/selfishell/current"
}

test_bootstrap_stops_on_termination() {
  local fake_bin="$TEST_ROOT/fakebin"
  local status

  mkdir -p "$fake_bin"
  cat >"$fake_bin/curl" <<'EOF'
#!/usr/bin/env bash
kill -TERM "$PPID"
EOF
  chmod +x "$fake_bin/curl"
  set +e
  PATH="$fake_bin:$PATH" run_bootstrap --version "$RELEASE_FIXTURE_VERSION" >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -eq 143 ]] || fail "Terminated bootstrap exited $status instead of 143"
  [[ ! -e "$TEST_ROOT/prefix/share/selfishell/current" ]] || fail "Terminated bootstrap activated a release"
}

# A tar wrapper that, after extracting, publishes the same release as a
# concurrent update would, so this update's staging lands inside it.
write_concurrent_release_tar() {
  local fake_bin="$1"
  local switch_links="$2"

  mkdir -p "$fake_bin"
  cat >"$fake_bin/tar" <<EOF
#!/usr/bin/env bash
/usr/bin/tar "\$@" || exit
while ((\$# > 0)); do
  [[ "\$1" == -C ]] && staging="\$2"
  shift
done
releases="\${staging%/*}"
version="\${staging##*/.}"
version="\${version%%.tmp.*}"
cp -R "\$staging" "\$releases/\$version"
if [[ "$switch_links" == 1 ]]; then
  share="\${releases%/*}"
  previous="\$(readlink "\$share/current")"
  rm -f "\$share/current" "\$share/previous"
  ln -s "releases/\$version" "\$share/current"
  ln -s "\$previous" "\$share/previous"
fi
EOF
  chmod +x "$fake_bin/tar"
}

test_concurrent_update_keeps_rollback_release() {
  local share="$TEST_ROOT/prefix/share/selfishell"

  run_bootstrap --version "$RELEASE_FIXTURE_VERSION" >/dev/null
  write_concurrent_release_tar "$TEST_ROOT/fakebin" 1
  PATH="$TEST_ROOT/fakebin:$PATH" "$TEST_ROOT/prefix/bin/selfishell" update --cli-only --version 0.2.3 --yes >/dev/null

  assert_symlink_to 'releases/0.2.3' "$share/current"
  assert_symlink_to "releases/$RELEASE_FIXTURE_VERSION" "$share/previous"
  [[ -d "$share/releases/$RELEASE_FIXTURE_VERSION" ]] || fail "Concurrent update pruned the rollback release"
  [[ -z "$(find "$share/releases/0.2.3" -name '.*.tmp.*')" ]] ||
    fail "Concurrent update left its staging inside the release"
}

test_concurrent_bootstrap_discards_nested_staging() {
  write_concurrent_release_tar "$TEST_ROOT/fakebin" 0
  PATH="$TEST_ROOT/fakebin:$PATH" run_bootstrap --version "$RELEASE_FIXTURE_VERSION" >/dev/null

  assert_symlink_to "releases/$RELEASE_FIXTURE_VERSION" "$TEST_ROOT/prefix/share/selfishell/current"
  [[ -z "$(find "$TEST_ROOT/prefix/share/selfishell/releases/$RELEASE_FIXTURE_VERSION" -name '.*.tmp.*')" ]] ||
    fail "Concurrent bootstrap left its staging inside the release"
}

test_day_old_staging_is_pruned() {
  local releases="$TEST_ROOT/prefix/share/selfishell/releases"

  run_bootstrap --version "$RELEASE_FIXTURE_VERSION" >/dev/null
  mkdir "$releases/.9.9.9.tmp.stale" "$releases/.9.9.9.tmp.fresh"
  touch -t 202001010000 "$releases/.9.9.9.tmp.stale"
  "$TEST_ROOT/prefix/bin/selfishell" update --cli-only --version 0.2.3 --yes >/dev/null
  [[ ! -e "$releases/.9.9.9.tmp.stale" ]] || fail "Update kept a day-old staging directory"
  [[ -d "$releases/.9.9.9.tmp.fresh" ]] || fail "Update removed a staging directory that may be in use"

  mkdir "$releases/.9.9.9.tmp.stale"
  touch -t 202001010000 "$releases/.9.9.9.tmp.stale"
  run_bootstrap --version "$RELEASE_FIXTURE_VERSION" >/dev/null
  [[ ! -e "$releases/.9.9.9.tmp.stale" ]] || fail "Bootstrap kept a day-old staging directory"
  [[ -d "$releases/.9.9.9.tmp.fresh" ]] || fail "Bootstrap removed a staging directory that may be in use"
}

main() {
  setup_release_fixture
  trap teardown_release_fixture EXIT HUP INT TERM

  run_discovered_tests_parallel \
    "${SELFISHELL_TEST_JOBS:-8}" \
    setup_release_home \
    teardown_release_home

  trap - EXIT HUP INT TERM
  teardown_release_fixture
}

main "$@"
