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
download mise 1.0.0 linux arm64 https://old/mise oldmise .local/bin/mise raw
git zinit v0.1.0 all all https://github.com/zdharma-continuum/zinit.git - .local/share/zinit/zinit.git zinit.zsh
nvim-plugin folke/lazy.nvim 1111111111111111111111111111111111111111 all all https://github.com/folke/lazy.nvim.git - - -
zsh-plugin zsh-users/zsh-completions 1111111111111111111111111111111111111111 all all https://github.com/zsh-users/zsh-completions.git - - -
EOF
  cat >"$metadata" <<'EOF'
download starship 2.0.0 linux amd64 https://new/starship.tar.gz newsum
download mise 2.0.0 linux arm64 https://new/mise newmise
git zinit v0.2.0
nvim-plugin folke/lazy.nvim 2222222222222222222222222222222222222222
zsh-plugin zsh-users/zsh-completions 3333333333333333333333333333333333333333
EOF

  bash "$ROOT_DIR/scripts/update-dependencies.sh" --manifest "$manifest" --metadata "$metadata"

  grep -Fqx 'download starship 2.0.0 linux amd64 https://new/starship.tar.gz newsum .local/bin/starship starship' "$manifest" ||
    fail "Starship metadata was not applied"
  grep -Fqx 'download mise 2.0.0 linux arm64 https://new/mise newmise .local/bin/mise raw' "$manifest" ||
    fail "mise metadata was not applied"
  grep -Fqx 'git zinit v0.2.0 all all https://github.com/zdharma-continuum/zinit.git - .local/share/zinit/zinit.git zinit.zsh' "$manifest" ||
    fail "Git dependency metadata was not applied"
  grep -Fqx 'nvim-plugin folke/lazy.nvim 2222222222222222222222222222222222222222 all all https://github.com/folke/lazy.nvim.git - - -' "$manifest" ||
    fail "Neovim plugin metadata was not applied"
  grep -Fqx 'zsh-plugin zsh-users/zsh-completions 3333333333333333333333333333333333333333 all all https://github.com/zsh-users/zsh-completions.git - - -' "$manifest" ||
    fail "Zsh plugin metadata was not applied"
}

test_rejects_metadata_without_manifest_entry() {
  local manifest metadata status

  setup_test_home
  trap teardown_test_home EXIT
  manifest="$TEST_ROOT/dependencies.conf"
  metadata="$TEST_ROOT/metadata"
  printf 'git zinit v0.1.0 all all https://example.invalid/zinit.git - .zinit zinit.zsh\n' >"$manifest"
  printf 'git missing v1.0.0\n' >"$metadata"

  set +e
  bash "$ROOT_DIR/scripts/update-dependencies.sh" --manifest "$manifest" --metadata "$metadata" >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -ne 0 ]] || fail "Unmatched metadata should fail"
  grep -Fqx 'git zinit v0.1.0 all all https://example.invalid/zinit.git - .zinit zinit.zsh' "$manifest" ||
    fail "Rejected metadata changed the manifest"
}

test_discovery_has_no_removed_vundle_dependency() {
  if grep -qi 'vundle' "$ROOT_DIR/scripts/update-dependencies.sh"; then
    fail "Dependency discovery still references removed Vundle metadata"
  fi
}

test_updates_only_matching_manifest_fields
printf 'PASS: test_updates_only_matching_manifest_fields\n'
test_rejects_metadata_without_manifest_entry
printf 'PASS: test_rejects_metadata_without_manifest_entry\n'
test_discovery_has_no_removed_vundle_dependency
printf 'PASS: test_discovery_has_no_removed_vundle_dependency\n'
