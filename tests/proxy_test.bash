#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

source "$ROOT_DIR/tests/test_helper.bash"
source "$ROOT_DIR/lib/common.sh"
source "$ROOT_DIR/lib/installers.sh"

test_direct_installer_preserves_proxy_environment() {
  local fake_bin="$TEST_ROOT/bin"

  mkdir -p "$fake_bin"
  cat >"$fake_bin/curl" <<'EOF'
#!/bin/sh
printf '%s' "$HTTPS_PROXY" >"$HOME/proxy-observed"
printf 'exit 0\n'
EOF
  chmod +x "$fake_bin/curl"

  export HTTPS_PROXY='http://proxy.example:8443'
  export PATH="$fake_bin:/usr/bin:/bin"
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
