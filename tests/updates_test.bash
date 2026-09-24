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

test_tools_update_synchronizes_packages() {
  local output
  export XDG_CONFIG_HOME="$HOME/.config"
  mkdir -p "$XDG_STATE_HOME/selfishell"
  printf '1\n' >"$XDG_STATE_HOME/selfishell/configured"

  output="$(bash "$ROOT_DIR/bin/selfishell" update --tools-only --dry-run)"
  [[ "$output" == *'Would install required apt packages:'* ]] ||
    fail "Tools update did not synchronize package-manager packages"
  [[ "$output" == *'git'* ]] || fail "Tools update did not include the environment packages"
  [[ "$output" == *'Neovim plugins'* ]] || fail "Tools update omitted Neovim plugin setup"
  [[ "$output" == *'Would prune unused mise versions'* ]] || fail "Tools update omitted default mise cleanup"
}

test_tools_update_skip_packages_avoids_package_operations() {
  local output
  export XDG_CONFIG_HOME="$HOME/.config"
  mkdir -p "$XDG_STATE_HOME/selfishell"
  printf '1\n' >"$XDG_STATE_HOME/selfishell/configured"

  output="$(bash "$ROOT_DIR/bin/selfishell" update --tools-only --skip-packages --dry-run)"
  [[ "$output" == *'Skipping package and tool installation.'* ]] ||
    fail "--skip-packages did not report skipping package and tool installation: $output"
  [[ "$output" != *'apt packages'* ]] ||
    fail "--skip-packages still touched apt packages: $output"
  [[ "$output" != *'prune'* ]] || fail "--skip-packages still planned cleanup: $output"
}

# Keep lifecycle orchestration real; replace only the installing operations.
run_update_cleanup_fixture() {
  bash -euo pipefail -c '
    export SELFISHELL_ROOT="$1"
    for module in common paths platform package_manifest installers packages releases commands/update; do
      source "$1/lib/$module.sh"
    done
    confirm_action() { return 0; }
    managed_preflight_zsh_loader() { return 0; }
    managed_preflight_block_target() { return 0; }
    packages_install() {
      printf "packages\n"
      SELFISHELL_SKIPPED_OPTIONAL_PACKAGES=()
      [[ "$2" != 1 ]] || return 0
      [[ "$FAIL_PHASE" != optional ]] || SELFISHELL_SKIPPED_OPTIONAL_PACKAGES=(bat)
      [[ "$FAIL_PHASE" != packages ]]
    }
    install_managed_configuration() { printf "configuration\n"; [[ "$FAIL_PHASE" != configuration ]]; }
    install_neovim_plugins() { printf "plugins\n"; [[ "$FAIL_PHASE" != plugins ]]; }
    selfishell_mise_command() { printf "mise\n"; }
    mise() {
      case "$*" in
        *"settings get ignored_config_paths") printf "[]\n" ;;
        *"config ls --tracked-configs") [[ "$FAIL_PHASE" != tracking ]] || return 0; printf "%s/config/shared/mise.toml\n" "$SELFISHELL_ROOT" ;;
        *"config ls") [[ "$FAIL_PHASE" != retention ]] ;;
        *"prune --tools --yes"*) printf "pruned\n"; [[ "$FAIL_PHASE" != prune ]] ;;
        *) return 91 ;;
      esac
    }
    update_tools_and_configuration 1 0 1 "$2" 0
  ' _ "$ROOT_DIR" "${1:-0}"
}

test_update_prunes_only_after_successful_setup() {
  local output phase status
  mkdir -p "$XDG_STATE_HOME/selfishell"
  printf '1\n' >"$XDG_STATE_HOME/selfishell/configured"
  output="$(FAIL_PHASE=none run_update_cleanup_fixture)"
  [[ "$output" == *$'packages\nconfiguration\nplugins\npruned\n'* ]] || fail "Cleanup did not follow successful setup: $output"
  for phase in packages configuration plugins; do
    status=0
    output="$(FAIL_PHASE="$phase" run_update_cleanup_fixture 2>&1)" || status=$?
    [[ "$status" != 0 && "$output" != *pruned* ]] || fail "Failed $phase phase still pruned or succeeded: $output"
  done
  output="$(FAIL_PHASE=optional run_update_cleanup_fixture 2>&1)"
  [[ "$output" != *pruned* && "$output" == *'Skipping mise cleanup'* ]] || fail "Partial tool update still pruned: $output"
  output="$(FAIL_PHASE=none run_update_cleanup_fixture 1)"
  [[ "$output" != *pruned* ]] || fail "Configuration-only update pruned tools"
  output="$(FAIL_PHASE=prune run_update_cleanup_fixture 2>&1)"
  [[ "$output" == *'Could not prune'* && "$output" == *'synchronized.'* ]] || fail "Cleanup failure hid successful update: $output"
  for phase in tracking retention; do
    output="$(FAIL_PHASE="$phase" run_update_cleanup_fixture 2>&1)"
    [[ "$output" != *pruned* && "$output" == *'Could not prune'* ]] || fail "Failed retention preflight still pruned: $output"
  done
}

test_mise_cleanup_preserves_current_and_project_versions_not_rollback_only() {
  local mise_binary version dir before after module
  mise_binary="$(command -v mise)" || skip "mise is unavailable for cleanup integration"
  [[ -x "$mise_binary" && -f "$mise_binary" ]] || skip "mise executable is unavailable"
  export XDG_CONFIG_HOME="$HOME/.config" XDG_DATA_HOME="$HOME/.local/share" XDG_CACHE_HOME="$HOME/.cache"
  export MISE_DATA_DIR="$XDG_DATA_HOME/mise" MISE_CACHE_DIR="$XDG_CACHE_HOME/mise" MISE_STATE_DIR="$XDG_STATE_HOME/mise"
  export MISE_OFFLINE=1 MISE_TRUSTED_CONFIG_PATHS="$HOME/project with spaces"
  unset MISE_GLOBAL_CONFIG_FILE MISE_IGNORED_CONFIG_PATHS __MISE_DIFF __MISE_SESSION
  for module in package_manifest platform packages installers releases; do
    # shellcheck disable=SC1090 # Load the same modules as the CLI.
    source "$ROOT_DIR/lib/$module.sh"
  done
  export SELFISHELL_ROOT="$HOME/selfishell/releases/2.0.0"
  mkdir -p "$SELFISHELL_ROOT/config/shared" "$HOME/selfishell/releases/1.0.0/config/shared" "$HOME/project with spaces"
  # Exercise symlinked parents and repeated separators on every platform.
  ln -s "$HOME/selfishell" "$HOME/release alias"
  SELFISHELL_ROOT="$HOME/release alias//releases/2.0.0"
  ln -s releases/2.0.0 "$HOME/selfishell/current"
  ln -s releases/1.0.0 "$HOME/selfishell/previous"
  for version in 1.0.0 2.0.0; do
    mkdir -p "$HOME/selfishell/releases/$version/bin"
    touch "$HOME/selfishell/releases/$version/bin/selfishell"
    chmod +x "$HOME/selfishell/releases/$version/bin/selfishell"
    printf '%s\n' "$version" >"$HOME/selfishell/releases/$version/VERSION"
  done
  printf '[tools]\nnode = "24.18.0"\n' >"$SELFISHELL_ROOT/config/shared/mise.toml"
  printf '[tools]\nnode = "24.13.0"\n' >"$HOME/selfishell/releases/1.0.0/config/shared/mise.toml"
  printf '[tools]\nnode = "22.0.0"\n' >"$HOME/project with spaces/mise.toml"
  printf 'package all required mise node\npackage macos optional mise go\n' >"$SELFISHELL_ROOT/packages.conf"
  package_manifest_load
  # shellcheck disable=SC2034 # Consumed by packages_prune_mise.
  SELFISHELL_SKIPPED_OPTIONAL_PACKAGES=()
  # Real mise inventory/prune with inert installed-version fixtures; no downloads.
  for version in 20.0.0 22.0.0 24.13.0 24.18.0; do
    dir="$MISE_DATA_DIR/installs/node/$version/bin"
    mkdir -p "$dir"
    printf '#!/bin/sh\nexit 0\n' >"$dir/node"
    chmod +x "$dir/node"
  done
  mkdir -p "$MISE_DATA_DIR/installs/go/1.20.0/bin"
  printf '#!/bin/sh\nexit 0\n' >"$MISE_DATA_DIR/installs/go/1.20.0/bin/go"
  chmod +x "$MISE_DATA_DIR/installs/go/1.20.0/bin/go"
  "$mise_binary" -C "$HOME/project with spaces" config ls >/dev/null
  cd "$HOME/project with spaces"
  before="$(find "$HOME" -print | sort)"
  packages_prune_mise ubuntu-wsl 1
  after="$(find "$HOME" -print | sort)"
  [[ "$before" == "$after" ]] || fail "Cleanup dry-run changed the filesystem"
  printf 'invalid\n' >"$HOME/selfishell/releases/1.0.0/VERSION"
  if packages_prune_mise ubuntu-wsl 0; then
    fail "Cleanup accepted a corrupt rollback release"
  fi
  [[ -d "$MISE_DATA_DIR/installs/node/20.0.0" ]] || fail "Retention failure deleted tools"
  printf '1.0.0\n' >"$HOME/selfishell/releases/1.0.0/VERSION"
  MISE_TRUSTED_CONFIG_PATHS="$TEST_ROOT" "$mise_binary" -C "$HOME/selfishell/releases/1.0.0/config/shared" config ls >/dev/null
  if MISE_IGNORED_CONFIG_PATHS="$HOME/selfishell/releases/1.0.0" packages_prune_mise ubuntu-wsl 0; then
    fail "Cleanup accepted ignored rollback pins despite existing tracking metadata"
  fi
  [[ -x "$MISE_DATA_DIR/installs/node/24.13.0/bin/node" ]] || fail "Ignored rollback pin was removed"
  packages_prune_mise ubuntu-wsl 0
  [[ ! -e "$MISE_DATA_DIR/installs/node/20.0.0" ]] || fail "Unused managed version survived cleanup"
  [[ ! -e "$MISE_DATA_DIR/installs/node/24.13.0" ]] || fail "Rollback-only version survived cleanup"
  for version in 22.0.0 24.18.0; do
    [[ -x "$MISE_DATA_DIR/installs/node/$version/bin/node" ]] || fail "Needed node@$version was removed"
  done
  [[ -d "$MISE_DATA_DIR/installs/go/1.20.0" ]] || fail "Another platform's tool was removed"
  assert_file_content '[tools]
node = "24.13.0"' "$HOME/selfishell/releases/1.0.0/config/shared/mise.toml"
  [[ "$(readlink "$HOME/selfishell/previous")" == releases/1.0.0 ]] || fail "Cleanup changed CLI rollback"
  [[ -z "${MISE_IGNORED_CONFIG_PATHS:-}" ]] || fail "Cleanup leaked ignored paths into the shell"
  # The same rollback version must survive when a project still needs it.
  dir="$MISE_DATA_DIR/installs/node/24.13.0/bin"
  mkdir -p "$dir"
  printf '#!/bin/sh\nexit 0\n' >"$dir/node"
  chmod +x "$dir/node"
  printf '[tools]\nnode = ["22.0.0", "24.13.0"]\n' >"$HOME/project with spaces/mise.toml"
  packages_prune_mise ubuntu-wsl 0
  [[ -x "$dir/node" ]] || fail "Rollback version needed by a project was removed"
  mv "$HOME/selfishell/releases/1.0.0/config/shared/mise.toml" "$TEST_ROOT/previous.toml"
  ln -s "$HOME/project with spaces/mise.toml" "$HOME/selfishell/releases/1.0.0/config/shared/mise.toml"
  if packages_prune_mise ubuntu-wsl 0; then
    fail "Cleanup accepted a rollback config linked to a project"
  fi
  [[ -x "$MISE_DATA_DIR/installs/node/22.0.0/bin/node" ]] || fail "Linked project version was removed"
  rm "$HOME/selfishell/releases/1.0.0/config/shared/mise.toml"
  mv "$TEST_ROOT/previous.toml" "$HOME/selfishell/releases/1.0.0/config/shared/mise.toml"
  # A previous link pointing at current must not exclude the active config.
  release_atomic_link releases/2.0.0 "$HOME/selfishell/previous"
  [[ "$(readlink "$HOME/selfishell/previous")" == releases/2.0.0 ]] || fail "Fixture did not point previous at current"
  packages_prune_mise ubuntu-wsl 0
  [[ -x "$MISE_DATA_DIR/installs/node/24.18.0/bin/node" ]] || fail "Cleanup excluded the current release"
  # A literal path must not turn into multiple ignored paths or a glob.
  release_atomic_link releases/1.0.0 "$HOME/selfishell/previous"
  mv "$HOME/selfishell" "$HOME/selfishell:unsafe"
  SELFISHELL_ROOT="$HOME/selfishell:unsafe/releases/2.0.0"
  if packages_prune_mise ubuntu-wsl 0; then
    fail "Cleanup accepted a rollback path containing mise's list separator"
  fi
  # Empty scope must never turn into mise's global prune.
  printf 'package all required apt git\n' >"$SELFISHELL_ROOT/packages.conf"
  package_manifest_load
  packages_prune_mise ubuntu 0
  [[ -d "$MISE_DATA_DIR/installs/go/1.20.0" ]] || fail "Empty scope pruned unrelated tools"
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

test_data_directory_dependency_targets_follow_xdg_data_home() {
  local payload checksum

  payload="$TEST_ROOT/tool"
  printf '#!/bin/sh\n' >"$payload"
  checksum="$(fixture_sha256 "$payload")"
  export SELFISHELL_DEPENDENCIES_FILE="$TEST_ROOT/dependencies.conf"
  printf 'download tool 1.0 linux amd64 file://%s %s .local/share/tool/tool raw\n' "$payload" "$checksum" \
    >"$SELFISHELL_DEPENDENCIES_FILE"

  XDG_DATA_HOME="$TEST_ROOT/data" run_dependency_install tool >/dev/null
  [[ -x "$TEST_ROOT/data/tool/tool" ]] || fail "Data dependency ignored XDG_DATA_HOME"
  [[ ! -e "$HOME/.local/share/tool" ]] || fail "Data dependency was installed under the default data home"
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

  # A directory at the same recorded version must be replaced, not nested into by mv.
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

  # A ".previous.$$" directory already on the exact path this process would
  # use, as a killed run with a reused PID leaves behind, must be skipped for
  # an unused path rather than corrupting or nesting the target.
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

# A tag can be moved after approval; the commit in the checksum column is what
# install verifies, and a checkout that drifts from it is reprovisioned.
test_git_dependency_is_pinned_to_its_commit() {
  local repo="$TEST_ROOT/repo" approved output status

  awk '$1 == "git" && $2 == "zinit" { exit ($7 ~ /^[0-9a-f]{40}$/ ? 0 : 1) }' "$ROOT_DIR/dependencies.conf" ||
    fail "The shipped zinit record does not pin a commit"

  mkdir -p "$repo"
  git -C "$repo" init --quiet
  git -C "$repo" config user.email test@example.com
  git -C "$repo" config user.name test
  printf 'marker\n' >"$repo/marker"
  git -C "$repo" add marker
  git -C "$repo" commit --quiet -m initial
  git -C "$repo" tag v1.0
  approved="$(git -C "$repo" rev-parse HEAD)"
  export SELFISHELL_DEPENDENCIES_FILE="$TEST_ROOT/dependencies.conf"
  printf 'git testgit v1.0 linux amd64 %s %s .local/share/testgit marker\n' "$repo" "$approved" >"$SELFISHELL_DEPENDENCIES_FILE"

  run_dependency_install testgit >/dev/null
  [[ "$(git -C "$HOME/.local/share/testgit" rev-parse HEAD)" == "$approved" ]] ||
    fail "The pinned dependency was not checked out at its approved commit"

  git -C "$HOME/.local/share/testgit" -c user.email=test@example.com -c user.name=test \
    commit --quiet --allow-empty -m drift
  output="$(run_dependency_install testgit)"
  [[ "$output" == *'Installed approved dependency: testgit v1.0'* &&
    "$(git -C "$HOME/.local/share/testgit" rev-parse HEAD)" == "$approved" ]] ||
    fail "A checkout that drifted from its approved commit was not reprovisioned: $output"

  git -C "$repo" commit --quiet --allow-empty -m moved
  git -C "$repo" tag -f v1.0 >/dev/null
  rm -rf "$HOME/.local/share/testgit" "$XDG_STATE_HOME/selfishell/dependencies/testgit"
  set +e
  output="$(run_dependency_install testgit 2>&1)"
  status=$?
  set -e
  [[ "$status" -ne 0 ]] || fail "A moved tag was installed"
  [[ "$output" == *"testgit v1.0 no longer points to its approved commit $approved"* ]] ||
    fail "A moved tag was not explained: $output"
  [[ ! -e "$HOME/.local/share/testgit" ]] || fail "A moved tag left a checkout behind"
}

run_discovered_tests setup_update_home teardown_update_home
