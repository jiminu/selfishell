#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/tests/test_helper.bash"

test_published_release_verification() {
  local version=9.8.7
  local artifacts release_root raw_root fake_bin output status=0

  setup_test_home
  trap teardown_test_home EXIT
  artifacts="$TEST_ROOT/artifacts"
  release_root="$TEST_ROOT/releases"
  raw_root="$TEST_ROOT/raw"
  fake_bin="$TEST_ROOT/bin"
  mkdir -p "$artifacts" "$release_root/download/v$version" \
    "$release_root/latest/download" "$raw_root/v$version" "$fake_bin"

  bash "$ROOT_DIR/scripts/build-release.sh" --version "$version" \
    --output "$artifacts" --no-update-source >/dev/null
  cp "$artifacts"/* "$release_root/download/v$version/"
  cp "$artifacts/VERSION" "$release_root/latest/download/VERSION"
  cp "$ROOT_DIR/install.sh" "$raw_root/v$version/install.sh"

  cat >"$fake_bin/gh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == release && "$2" == view && "$*" == *'--json assets'* ]]; then
  find "$SELFISHELL_TEST_RELEASE_ASSETS" -maxdepth 1 -type f -exec basename {} \; | LC_ALL=C sort
  exit 0
elif [[ "$1" == release && "$2" == view ]]; then
  printf 'v%s\tfalse\thttps://example.invalid/releases/tag/v%s\n' \
    "$SELFISHELL_TEST_RELEASE_VERSION" "$SELFISHELL_TEST_RELEASE_VERSION"
  exit 0
elif [[ "$1" == release && "$2" == download ]]; then
  while (($# > 0)); do
    if [[ "$1" == --dir ]]; then
      shift
      cp "$SELFISHELL_TEST_RELEASE_ASSETS"/* "$1/"
      exit 0
    fi
    shift
  done
  exit 2
elif [[ "$1" == attestation && "$2" == verify ]]; then
  exit 0
fi
exit 2
EOF
  chmod +x "$fake_bin/gh"

  output="$(
    PATH="$fake_bin:$PATH" \
      SELFISHELL_TEST_RELEASE_ASSETS="$artifacts" \
      SELFISHELL_TEST_RELEASE_VERSION="$version" \
      SELFISHELL_VERIFY_RAW_ROOT="file://$raw_root" \
      SELFISHELL_VERIFY_RELEASE_ROOT="file://$release_root" \
      bash "$ROOT_DIR/scripts/verify-published-release.sh" "$version" 2>"$TEST_ROOT/verification-stderr"
  )" || status=$?
  if ((status != 0)); then
    fail "Published release verification failed: $(<"$TEST_ROOT/verification-stderr")"
  fi
  [[ "$output" == *"Published release $version verified"* ]] ||
    fail "Published release verification did not report success"
}

test_invalid_version_is_rejected() {
  local status=0

  bash "$ROOT_DIR/scripts/verify-published-release.sh" invalid >/dev/null 2>&1 || status=$?
  [[ "$status" -eq 2 ]] || fail "Invalid published release version should return a usage error"
}

test_published_release_verification
printf 'PASS: test_published_release_verification\n'
test_invalid_version_is_rejected
printf 'PASS: test_invalid_version_is_rejected\n'
