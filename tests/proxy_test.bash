#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export SELFISHELL_ROOT="$ROOT_DIR"

source "$ROOT_DIR/tests/test_helper.bash"
source "$ROOT_DIR/lib/common.sh"
source "$ROOT_DIR/lib/paths.sh"
source "$ROOT_DIR/lib/platform.sh"
source "$ROOT_DIR/lib/dependencies.sh"
source "$ROOT_DIR/lib/installers.sh"

test_direct_installer_preserves_proxy_environment() {
  local fake_bin="$TEST_ROOT/bin"

  mkdir -p "$fake_bin"
  local payload checksum
  payload="$TEST_ROOT/nvm.tar"
  printf 'nvm fixture' >"$payload"
  checksum="$(fixture_sha256 "$payload")"
  export SELFISHELL_DEPENDENCIES_FILE="$TEST_ROOT/dependencies.conf"
  printf 'download nvm 1.0 linux amd64 file://%s %s .nvm/nvm.sh raw\n' "$payload" "$checksum" >"$SELFISHELL_DEPENDENCIES_FILE"

  cat >"$fake_bin/curl" <<'EOF'
#!/bin/sh
printf '%s' "$HTTPS_PROXY" >"$HOME/proxy-observed"
exec /usr/bin/curl "$@"
EOF
  chmod +x "$fake_bin/curl"

  export HTTPS_PROXY='http://proxy.example:8443'
  export PATH="$fake_bin:/usr/bin:/bin"
  export SELFISHELL_TEST_SYSTEM_NAME=Linux
  export SELFISHELL_TEST_MACHINE_ARCH=x86_64
  export SELFISHELL_TEST_OS_RELEASE_FILE="$TEST_ROOT/os-release"
  export SELFISHELL_TEST_PROC_VERSION_FILE="$TEST_ROOT/proc-version"
  printf 'ID=ubuntu\n' >"$SELFISHELL_TEST_OS_RELEASE_FILE"
  printf 'Linux version\n' >"$SELFISHELL_TEST_PROC_VERSION_FILE"
  SELFISHELL_SKIPPED_OPTIONAL_PACKAGES=()
  install_direct_package required nvm 0

  assert_file_content "$HTTPS_PROXY" "$HOME/proxy-observed"
}

main() {
  setup_test_home
  trap teardown_test_home EXIT
  test_direct_installer_preserves_proxy_environment
  printf 'PASS: test_direct_installer_preserves_proxy_environment\n'
}

main "$@"
