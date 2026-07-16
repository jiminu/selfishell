#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/tests/test_helper.bash"

test_updates_only_matching_manifest_fields() {
  local manifest metadata

  setup_test_home
  trap teardown_test_home EXIT
  manifest="$TEST_ROOT/dependencies.conf"
  metadata="$TEST_ROOT/metadata"
  cat >"$manifest" <<'EOF'
# type name version platform architecture source checksum target marker
download starship 1.0.0 linux amd64 https://old/starship.tar.gz oldsum .local/bin/starship starship
download kubectl 1.30.0 linux arm64 https://old/kubectl oldkubectl .local/bin/kubectl raw
git nvm v0.1.0 all all https://github.com/nvm-sh/nvm.git - .nvm nvm.sh
EOF
  cat >"$metadata" <<'EOF'
download starship 2.0.0 linux amd64 https://new/starship.tar.gz newsum
download kubectl 1.31.0 linux arm64 https://new/kubectl newkubectl
git nvm v0.2.0
EOF

  bash "$ROOT_DIR/scripts/update-dependencies.sh" --manifest "$manifest" --metadata "$metadata"

  grep -Fqx 'download starship 2.0.0 linux amd64 https://new/starship.tar.gz newsum .local/bin/starship starship' "$manifest" ||
    fail "Starship metadata was not applied"
  grep -Fqx 'download kubectl 1.31.0 linux arm64 https://new/kubectl newkubectl .local/bin/kubectl raw' "$manifest" ||
    fail "kubectl metadata was not applied"
  grep -Fqx 'git nvm v0.2.0 all all https://github.com/nvm-sh/nvm.git - .nvm nvm.sh' "$manifest" ||
    fail "Git dependency metadata was not applied"
}

test_rejects_metadata_without_manifest_entry() {
  local manifest metadata status

  setup_test_home
  trap teardown_test_home EXIT
  manifest="$TEST_ROOT/dependencies.conf"
  metadata="$TEST_ROOT/metadata"
  printf 'git nvm v0.1.0 all all https://example.invalid/nvm.git - .nvm nvm.sh\n' >"$manifest"
  printf 'git missing v1.0.0\n' >"$metadata"

  set +e
  bash "$ROOT_DIR/scripts/update-dependencies.sh" --manifest "$manifest" --metadata "$metadata" >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -ne 0 ]] || fail "Unmatched metadata should fail"
  grep -Fqx 'git nvm v0.1.0 all all https://example.invalid/nvm.git - .nvm nvm.sh' "$manifest" ||
    fail "Rejected metadata changed the manifest"
}

test_updates_only_matching_manifest_fields
printf 'PASS: test_updates_only_matching_manifest_fields\n'
test_rejects_metadata_without_manifest_entry
printf 'PASS: test_rejects_metadata_without_manifest_entry\n'
