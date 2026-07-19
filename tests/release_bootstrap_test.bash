#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

source "$ROOT_DIR/tests/test_helper.bash"

setup_release_home() {
  local version
  local next_version=0.2.3
  local prerelease_version=0.3.0-beta.2

  setup_test_home
  version="$(<"$ROOT_DIR/VERSION")"
  export SELFISHELL_RELEASE_ROOT="file://$TEST_ROOT/releases"
  export SELFISHELL_BOOTSTRAP_OS=Linux
  export SELFISHELL_BOOTSTRAP_ARCH=x86_64
  export XDG_CONFIG_HOME="$HOME/.config"
  export XDG_STATE_HOME="$HOME/.local/state"
  export SELFISHELL_TEST_SYSTEM_NAME=Linux
  export SELFISHELL_TEST_MACHINE_ARCH=x86_64
  export SELFISHELL_TEST_OS_RELEASE_FILE="$TEST_ROOT/os-release"
  export SELFISHELL_TEST_PROC_VERSION_FILE="$TEST_ROOT/proc-version"
  printf 'ID=ubuntu\n' >"$SELFISHELL_TEST_OS_RELEASE_FILE"
  printf 'Linux version 6.8.0\n' >"$SELFISHELL_TEST_PROC_VERSION_FILE"

  mkdir -p "$TEST_ROOT/artifacts" "$TEST_ROOT/next-artifacts" "$TEST_ROOT/prerelease-artifacts" \
    "$TEST_ROOT/releases/download/v$version" "$TEST_ROOT/releases/download/v$next_version" \
    "$TEST_ROOT/releases/download/v$prerelease_version" \
    "$TEST_ROOT/releases/latest/download"
  bash "$ROOT_DIR/scripts/build-release.sh" --version "$version" --output "$TEST_ROOT/artifacts" >/dev/null
  bash "$ROOT_DIR/scripts/build-release.sh" --version "$next_version" --output "$TEST_ROOT/next-artifacts" >/dev/null
  bash "$ROOT_DIR/scripts/build-release.sh" --version "$prerelease_version" --output "$TEST_ROOT/prerelease-artifacts" >/dev/null
  cp "$TEST_ROOT/artifacts"/* "$TEST_ROOT/releases/download/v$version/"
  cp "$TEST_ROOT/next-artifacts"/* "$TEST_ROOT/releases/download/v$next_version/"
  cp "$TEST_ROOT/prerelease-artifacts"/* "$TEST_ROOT/releases/download/v$prerelease_version/"
  cp "$TEST_ROOT/next-artifacts/VERSION" "$TEST_ROOT/releases/latest/download/VERSION"
}

teardown_release_home() {
  unset SELFISHELL_RELEASE_ROOT SELFISHELL_RELEASE_API_URL SELFISHELL_RELEASE_TAGS_API_URL
  unset SELFISHELL_BOOTSTRAP_OS SELFISHELL_BOOTSTRAP_ARCH
  unset XDG_CONFIG_HOME XDG_STATE_HOME SELFISHELL_OFFLINE
  unset SELFISHELL_TEST_SYSTEM_NAME SELFISHELL_TEST_MACHINE_ARCH
  unset SELFISHELL_TEST_OS_RELEASE_FILE SELFISHELL_TEST_PROC_VERSION_FILE
  teardown_test_home
}

run_bootstrap() {
  bash "$ROOT_DIR/install.sh" --prefix "$TEST_ROOT/prefix" "$@"
}

test_builds_all_platform_architecture_artifacts() {
  local version
  version="$(<"$ROOT_DIR/VERSION")"

  for artifact in \
    "selfishell-$version-linux-amd64.tar.gz" \
    "selfishell-$version-linux-arm64.tar.gz" \
    "selfishell-$version-macos-amd64.tar.gz" \
    "selfishell-$version-macos-arm64.tar.gz"; do
    [[ -f "$TEST_ROOT/artifacts/$artifact" ]] || fail "Missing release artifact: $artifact"
  done
  [[ -s "$TEST_ROOT/artifacts/SHA256SUMS" ]] || fail "Missing release checksums"
}

test_release_artifacts_are_reproducible() {
  local version artifact
  local second_output="$TEST_ROOT/reproducible-artifacts"

  version="$(<"$ROOT_DIR/VERSION")"
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
  local version
  version="$(<"$ROOT_DIR/VERSION")"

  run_bootstrap --version "$version" >/dev/null

  assert_symlink_to "releases/$version" "$TEST_ROOT/prefix/share/selfishell/current"
  assert_symlink_to "$TEST_ROOT/prefix/share/selfishell/current/bin/selfishell" "$TEST_ROOT/prefix/bin/selfishell"
  assert_symlink_to selfishell "$TEST_ROOT/prefix/bin/sfs"
  [[ "$("$TEST_ROOT/prefix/bin/selfishell" version)" == "selfishell $version" ]] ||
    fail "Installed CLI reports the wrong version"
}

test_latest_uses_published_version_file() {
  run_bootstrap >/dev/null
  [[ "$(<"$TEST_ROOT/prefix/share/selfishell/current/VERSION")" == 0.2.3 ]] ||
    fail "Latest installation selected the wrong version"
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
  version="$(<"$ROOT_DIR/VERSION")"
  run_bootstrap --version "$version" >/dev/null
  rm "$TEST_ROOT/releases/latest/download/VERSION"
  printf '[{"name":"v0.3.0-beta.2"}]\n' >"$TEST_ROOT/tags-api.json"
  export SELFISHELL_RELEASE_TAGS_API_URL="file://$TEST_ROOT/tags-api.json"

  "$TEST_ROOT/prefix/bin/selfishell" update --cli-only --yes >/dev/null

  assert_symlink_to 'releases/0.3.0-beta.2' "$TEST_ROOT/prefix/share/selfishell/current"
}

test_status_falls_back_to_published_prerelease() {
  local output version
  version="$(<"$ROOT_DIR/VERSION")"
  run_bootstrap --version "$version" >/dev/null
  SELFISHELL_OFFLINE=1 "$TEST_ROOT/prefix/bin/selfishell" \
    install --profile minimal --skip-packages --yes >/dev/null
  rm "$TEST_ROOT/releases/latest/download/VERSION"
  printf '[{"name":"v0.3.0-beta.2"}]\n' >"$TEST_ROOT/tags-api.json"
  export SELFISHELL_RELEASE_TAGS_API_URL="file://$TEST_ROOT/tags-api.json"

  output="$("$TEST_ROOT/prefix/bin/selfishell" status --check-updates)" || true

  [[ "$output" == *"[CLI] Current: $version | Available: 0.3.0-beta.2"* ]] ||
    fail "Status did not report the published prerelease"
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
  local version
  version="$(<"$ROOT_DIR/VERSION")"
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

  SELFISHELL_RELEASE_ROOT='file:///unavailable' \
    "$TEST_ROOT/prefix/bin/selfishell" rollback --yes >/dev/null
  assert_symlink_to "releases/$version" "$TEST_ROOT/prefix/share/selfishell/current"
  assert_symlink_to 'releases/0.2.3' "$TEST_ROOT/prefix/share/selfishell/previous"
}

test_default_update_skips_missing_configuration_and_updates_cli() {
  local output
  local cli_line skip_line
  local version

  version="$(<"$ROOT_DIR/VERSION")"
  run_bootstrap --version "$version" >/dev/null

  output="$("$TEST_ROOT/prefix/bin/selfishell" update --version 0.2.3 --yes)"
  [[ "$output" == *'skipping tools and configuration'* ]] ||
    fail "Default update did not skip an uninstalled configuration"
  cli_line="$(printf '%s\n' "$output" | awk '/CLI updated to/ { print NR; exit }')"
  skip_line="$(printf '%s\n' "$output" | awk '/skipping tools and configuration/ { print NR; exit }')"
  [[ -n "$cli_line" && -n "$skip_line" && "$cli_line" -lt "$skip_line" ]] ||
    fail "Default update did not continue with the new CLI after switching releases"
  assert_symlink_to 'releases/0.2.3' "$TEST_ROOT/prefix/share/selfishell/current"
}

test_update_dry_run_preserves_active_release() {
  local output
  local version

  version="$(<"$ROOT_DIR/VERSION")"
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

  version="$(<"$ROOT_DIR/VERSION")"
  archive="$TEST_ROOT/releases/download/v$version/selfishell-$version-linux-amd64.tar.gz"
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
  version="$(<"$ROOT_DIR/VERSION")"
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

test_add_to_path_updates_bashrc_once() {
  local count output

  output="$(SELFISHELL_BOOTSTRAP_SHELL=/bin/bash run_bootstrap --add-to-path)"
  SELFISHELL_BOOTSTRAP_SHELL=/bin/bash run_bootstrap --add-to-path >/dev/null

  [[ "$output" == *"Added $TEST_ROOT/prefix/bin to PATH in $HOME/.bashrc"* ]] ||
    fail "Bash PATH installation was not reported"
  count="$(grep -Fc '# Added by Selfishell installer' "$HOME/.bashrc")"
  [[ "$count" -eq 1 ]] || fail "Bash PATH entry was added more than once"
  PATH=/usr/bin:/bin bash -c 'source "$1"; [[ ":$PATH:" == *":$2:"* ]]' \
    _ "$HOME/.bashrc" "$TEST_ROOT/prefix/bin" || fail "Bash startup did not activate the CLI path"
}

test_purge_removes_installer_path_entry() {
  SELFISHELL_BOOTSTRAP_SHELL=/bin/bash run_bootstrap --add-to-path >/dev/null

  "$TEST_ROOT/prefix/bin/selfishell" uninstall --purge --yes >/dev/null

  [[ -r "$HOME/.bashrc" ]] || fail "Purge removed the user startup file"
  [[ "$(<"$HOME/.bashrc")" != *'# Added by Selfishell installer'* ]] ||
    fail "Purge retained the installer PATH marker"
  [[ "$(<"$HOME/.bashrc")" != *"$TEST_ROOT/prefix/bin"* ]] ||
    fail "Purge retained the installer PATH entry"
}

test_purge_preserves_modified_path_entry() {
  local status
  SELFISHELL_BOOTSTRAP_SHELL=/bin/bash run_bootstrap --add-to-path >/dev/null
  printf '# user change\n' >>"$HOME/.bashrc"
  sed -i.bak 's/export PATH=/export MODIFIED_PATH=/' "$HOME/.bashrc"
  rm "$HOME/.bashrc.bak"

  set +e
  "$TEST_ROOT/prefix/bin/selfishell" uninstall --purge --yes >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -eq 1 ]] || fail "Purge should reject a modified PATH entry"
  [[ -x "$TEST_ROOT/prefix/bin/selfishell" ]] || fail "Rejected PATH purge removed the CLI"
  grep -Fq 'MODIFIED_PATH=' "$HOME/.bashrc" || fail "Rejected PATH purge changed the startup file"
}

test_add_to_path_selects_zshrc() {
  SELFISHELL_BOOTSTRAP_SHELL=/bin/zsh run_bootstrap --add-to-path >/dev/null

  [[ -r "$HOME/.zshrc" ]] || fail "Zsh PATH installation did not create .zshrc"
  [[ ! -e "$HOME/.bashrc" ]] || fail "Zsh PATH installation changed .bashrc"
  grep -Fq "$TEST_ROOT/prefix/bin" "$HOME/.zshrc" || fail "Zsh PATH entry is missing"
}

test_setup_is_explicit_and_can_run_offline() {
  export SELFISHELL_OFFLINE=1
  run_bootstrap --setup --yes >/dev/null

  assert_symlink_to "$XDG_CONFIG_HOME/selfishell/zsh/zshrc" "$HOME/.zshrc"
}

test_missing_bin_path_prints_actionable_message() {
  local output
  output="$(PATH=/usr/bin:/bin run_bootstrap)"

  [[ "$output" == *"export PATH=\"$TEST_ROOT/prefix/bin:\$PATH\""* ]] ||
    fail "Missing PATH guidance did not include a current-shell command"
  [[ "$output" == *'reinstall with --add-to-path'* ]] ||
    fail "Missing PATH guidance did not explain persistent setup"
  [[ "$output" == *"$TEST_ROOT/prefix/bin/selfishell install"* ]] ||
    fail "Missing PATH guidance did not include the absolute CLI command"
}

test_purge_dry_run_preserves_installation() {
  run_bootstrap --setup --skip-packages --yes >/dev/null

  "$TEST_ROOT/prefix/bin/selfishell" uninstall --restore --purge --dry-run >/dev/null

  [[ -x "$TEST_ROOT/prefix/bin/selfishell" ]] || fail "Purge dry-run removed the CLI"
  [[ -L "$HOME/.zshrc" ]] || fail "Purge dry-run removed managed configuration"
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
  [[ ! -e "$HOME/.zshrc" ]] || fail "Purge retained managed configuration"
}

test_purge_refuses_non_managed_cli_path_before_uninstall() {
  local status
  run_bootstrap --setup --skip-packages --yes >/dev/null
  rm "$TEST_ROOT/prefix/bin/sfs"
  printf 'user command\n' >"$TEST_ROOT/prefix/bin/sfs"

  set +e
  "$TEST_ROOT/prefix/bin/selfishell" uninstall --restore --purge --yes >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -eq 1 ]] || fail "Purge should reject a non-managed CLI path"
  [[ -x "$TEST_ROOT/prefix/bin/selfishell" ]] || fail "Rejected purge removed the CLI"
  [[ -L "$HOME/.zshrc" ]] || fail "Rejected purge removed managed configuration"
  assert_file_content 'user command' "$TEST_ROOT/prefix/bin/sfs"
}

test_refuses_to_replace_non_link_cli_path() {
  local status

  mkdir -p "$TEST_ROOT/prefix/bin"
  printf 'user file' >"$TEST_ROOT/prefix/bin/selfishell"
  set +e
  run_bootstrap >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -eq 1 ]] || fail "Non-link CLI path should block installation"
  assert_file_content 'user file' "$TEST_ROOT/prefix/bin/selfishell"
  [[ ! -e "$TEST_ROOT/prefix/share/selfishell/current" ]] ||
    fail "Link preflight failure changed the active release"
}

run_test() {
  local test_name="$1"

  setup_release_home
  trap 'teardown_release_home' RETURN
  "$test_name"
  trap - RETURN
  teardown_release_home
  printf 'PASS: %s\n' "$test_name"
}

main() {
  local test_name

  while IFS= read -r test_name; do
    run_test "$test_name"
  done < <(declare -F | awk '{print $3}' | grep '^test_' | sort)
}

main "$@"
