#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/tests/test_helper.bash"

setup_release_verification_fixture() {
  local version="$1"
  local artifacts release_root raw_root fake_bin

  artifacts="$TEST_ROOT/artifacts"
  release_root="$TEST_ROOT/releases"
  raw_root="$TEST_ROOT/raw"
  fake_bin="$TEST_ROOT/bin"
  mkdir -p "$artifacts" "$release_root/download/v$version" \
    "$release_root/latest/download" "$raw_root/v$version" "$fake_bin"

  bash "$ROOT_DIR/scripts/build-release.sh" --version "$version" \
    --output "$artifacts" >/dev/null
  cp "$artifacts"/* "$release_root/download/v$version/"
  cp "$artifacts/VERSION" "$release_root/latest/download/VERSION"
  cp "$ROOT_DIR/install.sh" "$raw_root/v$version/install.sh"

  # SELFISHELL_TEST_GH_NO_ATTESTATION=1 simulates a gh CLI that predates the
  # `gh attestation` subcommand (an unrecognized-command exit), so callers can
  # exercise the "attestation verification unavailable" path.
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
  [[ "${SELFISHELL_TEST_GH_NO_ATTESTATION:-0}" != "1" ]] || exit 2
  exit 0
fi
exit 2
EOF
  chmod +x "$fake_bin/gh"
}

test_published_release_verification() {
  local version=9.8.7
  local output status=0

  setup_test_home
  trap teardown_test_home EXIT
  setup_release_verification_fixture "$version"

  output="$(
    PATH="$TEST_ROOT/bin:$PATH" \
      SELFISHELL_TEST_RELEASE_ASSETS="$TEST_ROOT/artifacts" \
      SELFISHELL_TEST_RELEASE_VERSION="$version" \
      SELFISHELL_VERIFY_RAW_ROOT="file://$TEST_ROOT/raw" \
      SELFISHELL_VERIFY_RELEASE_ROOT="file://$TEST_ROOT/releases" \
      bash "$ROOT_DIR/scripts/verify-published-release.sh" "$version" 2>"$TEST_ROOT/verification-stderr"
  )" || status=$?
  if ((status != 0)); then
    fail "Published release verification failed: $(<"$TEST_ROOT/verification-stderr")"
  fi
  [[ "$output" == *"Published release $version verified"* ]] ||
    fail "Published release verification did not report success"
  [[ "$output" == *'Artifact attestations verified.'* ]] ||
    fail "Published release verification did not report attestation verification"
}

# A gh CLI without attestation support must not let this script silently
# downgrade to checksum self-consistency only while still reporting the
# release as fully verified -- docs/RELEASING.md documents attestation as a
# guaranteed property of every release.
test_published_release_verification_fails_without_attestation_by_default() {
  local version=9.8.8
  local status=0

  setup_test_home
  trap teardown_test_home EXIT
  setup_release_verification_fixture "$version"

  SELFISHELL_TEST_GH_NO_ATTESTATION=1 \
    PATH="$TEST_ROOT/bin:$PATH" \
    SELFISHELL_TEST_RELEASE_ASSETS="$TEST_ROOT/artifacts" \
    SELFISHELL_TEST_RELEASE_VERSION="$version" \
    SELFISHELL_VERIFY_RAW_ROOT="file://$TEST_ROOT/raw" \
    SELFISHELL_VERIFY_RELEASE_ROOT="file://$TEST_ROOT/releases" \
    bash "$ROOT_DIR/scripts/verify-published-release.sh" "$version" >/dev/null 2>"$TEST_ROOT/verification-stderr" ||
    status=$?

  ((status != 0)) || fail "Verification succeeded despite unavailable attestation support"
  grep -Fq 'SELFISHELL_VERIFY_SKIP_ATTESTATION' "$TEST_ROOT/verification-stderr" ||
    fail "Failure did not explain the opt-out: $(<"$TEST_ROOT/verification-stderr")"
}

test_published_release_verification_skip_attestation_opt_out() {
  local version=9.8.9
  local output status=0

  setup_test_home
  trap teardown_test_home EXIT
  setup_release_verification_fixture "$version"

  output="$(
    SELFISHELL_TEST_GH_NO_ATTESTATION=1 \
      SELFISHELL_VERIFY_SKIP_ATTESTATION=1 \
      PATH="$TEST_ROOT/bin:$PATH" \
      SELFISHELL_TEST_RELEASE_ASSETS="$TEST_ROOT/artifacts" \
      SELFISHELL_TEST_RELEASE_VERSION="$version" \
      SELFISHELL_VERIFY_RAW_ROOT="file://$TEST_ROOT/raw" \
      SELFISHELL_VERIFY_RELEASE_ROOT="file://$TEST_ROOT/releases" \
      bash "$ROOT_DIR/scripts/verify-published-release.sh" "$version" 2>"$TEST_ROOT/verification-stderr"
  )" || status=$?
  if ((status != 0)); then
    fail "Explicit attestation opt-out still failed: $(<"$TEST_ROOT/verification-stderr")"
  fi
  [[ "$output" == *"Published release $version verified"* ]] ||
    fail "Explicit attestation opt-out did not report success"
  [[ "$output" == *'skipped (SELFISHELL_VERIFY_SKIP_ATTESTATION=1)'* ]] ||
    fail "Explicit attestation opt-out did not report that it was skipped"
}

test_invalid_version_is_rejected() {
  local status=0

  bash "$ROOT_DIR/scripts/verify-published-release.sh" invalid >/dev/null 2>&1 || status=$?
  [[ "$status" -eq 2 ]] || fail "Invalid published release version should return a usage error"
}

run_discovered_tests '' teardown_test_home
