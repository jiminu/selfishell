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
  payload="$TEST_ROOT/mise"
  printf 'mise fixture' >"$payload"
  checksum="$(fixture_sha256 "$payload")"
  export SELFISHELL_DEPENDENCIES_FILE="$TEST_ROOT/dependencies.conf"
  printf 'download mise 1.0 linux amd64 file://%s %s .local/bin/mise raw\n' "$payload" "$checksum" >"$SELFISHELL_DEPENDENCIES_FILE"

  cat >"$fake_bin/curl" <<'EOF'
#!/bin/sh
printf '%s' "$HTTPS_PROXY" >"$HOME/proxy-observed"
printf '%s\n' "$@" >"$HOME/curl-arguments"
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
  install_direct_package required mise 0

  assert_file_content "$HTTPS_PROXY" "$HOME/proxy-observed"
  grep -Fxq -- '--connect-timeout' "$HOME/curl-arguments" ||
    fail "Direct download did not set a connection timeout"
  grep -Fxq -- '--speed-limit' "$HOME/curl-arguments" ||
    fail "Direct download did not set a low-speed limit"
  grep -Fxq -- '--speed-time' "$HOME/curl-arguments" ||
    fail "Direct download did not set a low-speed duration"
  ! grep -Fxq -- '--max-time' "$HOME/curl-arguments" ||
    fail "Direct download should not use a short total timeout"
}

test_invalid_curl_policy_is_rejected_before_network_access() {
  local output status
  rm -f "$HOME/proxy-observed"

  set +e
  output="$(SELFISHELL_CURL_CONNECT_TIMEOUT=0 \
    selfishell_curl transfer file://"$TEST_ROOT/mise" 2>&1)"
  status=$?
  set -e

  [[ "$status" -eq "$SELFISHELL_EXIT_USAGE" ]] ||
    fail "Invalid curl policy should return a usage error"
  [[ "$output" == *'must be positive integers'* ]] ||
    fail "Invalid curl policy did not explain the accepted values"
  [[ ! -e "$HOME/proxy-observed" ]] || fail "Invalid curl policy still invoked curl"
}

main() {
  setup_test_home
  trap teardown_test_home EXIT
  test_direct_installer_preserves_proxy_environment
  printf 'PASS: test_direct_installer_preserves_proxy_environment\n'
  test_invalid_curl_policy_is_rejected_before_network_access
  printf 'PASS: test_invalid_curl_policy_is_rejected_before_network_access\n'
}

main "$@"
