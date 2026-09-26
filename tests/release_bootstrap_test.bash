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
