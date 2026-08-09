#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/tests/test_helper.bash"

OLD_COMPLETIONS=1111111111111111111111111111111111111111
NEW_COMPLETIONS=2222222222222222222222222222222222222222
OLD_FZF_TAB=3333333333333333333333333333333333333333
NEW_FZF_TAB=4444444444444444444444444444444444444444
OLD_AUTOSUGGESTIONS=5555555555555555555555555555555555555555

write_zsh_root_fixtures() {
  local zsh_root="$1"

  mkdir -p "$zsh_root/common"
  cat >"$zsh_root/common/completion.zsh" <<EOF
if [[ -s "\$ZINIT_HOME/zinit.zsh" ]]; then
  source "\$ZINIT_HOME/zinit.zsh"
  zinit ice blockf atpull'zinit creinstall -q .' ver'$OLD_COMPLETIONS'
  zinit light zsh-users/zsh-completions
fi
EOF
  cat >"$zsh_root/common/interactive.zsh" <<EOF
if ((\$+functions[zinit])); then
  if command -v fzf >/dev/null 2>&1; then
    zinit ice ver'$OLD_FZF_TAB'
    zinit light Aloxaf/fzf-tab
  fi
  zinit ice wait'0' lucid ver'$OLD_AUTOSUGGESTIONS'
  zinit light zsh-users/zsh-autosuggestions
fi
EOF
  printf 'unrelated fixture content\n' >"$zsh_root/common/other.zsh"
}

write_mise_toml_fixtures() {
  local zsh_root="$1"

  mkdir -p "$zsh_root/common"
  cat >"$zsh_root/common/mise.toml" <<'EOF'
[tools]
node = "24.18.0"
python = "3.13.14"
neovim = "0.12.4"
tree-sitter = "0.26.11"
uv = "0.5.21"

[settings]
not_found_auto_install = false
EOF
}

run_dependency_update() {
  local manifest="$1"
  local metadata="$2"
  local zsh_root="${3:-}"
  local arguments=(--manifest "$manifest" --metadata "$metadata")

  if [[ -n "$zsh_root" ]]; then
    arguments+=(--zsh-root "$zsh_root")
  fi
  bash "$ROOT_DIR/scripts/update-dependencies.sh" "${arguments[@]}"
}

assert_manifest_record() {
  local manifest="$1"
  local message="$2"
  shift 2

  grep -Fqx "$*" "$manifest" || fail "$message"
}

test_updates_only_matching_manifest_fields() {
  local manifest metadata zsh_root

  manifest="$TEST_ROOT/dependencies.conf"
  metadata="$TEST_ROOT/metadata"
  zsh_root="$TEST_ROOT/zsh-root"
  write_zsh_root_fixtures "$zsh_root"
  cat >"$manifest" <<EOF
# type name version platform architecture source checksum target marker
download starship 1.0.0 linux amd64 https://old/starship.tar.gz oldsum .local/bin/starship starship
download mise 1.0.0 linux arm64 https://old/mise oldmise .local/bin/mise raw
git zinit v0.1.0 all all https://github.com/zdharma-continuum/zinit.git - .local/share/zinit/zinit.git zinit.zsh
nvim-plugin folke/lazy.nvim 1111111111111111111111111111111111111111 all all https://github.com/folke/lazy.nvim.git - - -
zsh-plugin zsh-users/zsh-completions $OLD_COMPLETIONS all all https://github.com/zsh-users/zsh-completions.git - - -
EOF
  cat >"$metadata" <<EOF
download starship 2.0.0 linux amd64 https://new/starship.tar.gz newsum
download mise 2.0.0 linux arm64 https://new/mise newmise
git zinit v0.2.0
nvim-plugin folke/lazy.nvim 2222222222222222222222222222222222222222
zsh-plugin zsh-users/zsh-completions $NEW_COMPLETIONS
EOF

  run_dependency_update "$manifest" "$metadata" "$zsh_root"

  assert_manifest_record "$manifest" "Starship metadata was not applied" \
    download starship 2.0.0 linux amd64 \
    https://new/starship.tar.gz newsum .local/bin/starship starship
  assert_manifest_record "$manifest" "mise metadata was not applied" \
    download mise 2.0.0 linux arm64 https://new/mise newmise .local/bin/mise raw
  assert_manifest_record "$manifest" "Git dependency metadata was not applied" \
    git zinit v0.2.0 all all \
    https://github.com/zdharma-continuum/zinit.git - .local/share/zinit/zinit.git zinit.zsh
  assert_manifest_record "$manifest" "Neovim plugin metadata was not applied" \
    nvim-plugin folke/lazy.nvim 2222222222222222222222222222222222222222 \
    all all https://github.com/folke/lazy.nvim.git - - -
  assert_manifest_record "$manifest" "Zsh plugin metadata was not applied" \
    zsh-plugin zsh-users/zsh-completions "$NEW_COMPLETIONS" all all \
    https://github.com/zsh-users/zsh-completions.git - - -
  grep -Fq "ver'$NEW_COMPLETIONS'" "$zsh_root/common/completion.zsh" ||
    fail "Zinit pin in completion.zsh was not updated to the new commit"
}

test_rejects_metadata_without_manifest_entry() {
  local manifest metadata status

  manifest="$TEST_ROOT/dependencies.conf"
  metadata="$TEST_ROOT/metadata"
  printf 'git zinit v0.1.0 all all https://example.invalid/zinit.git - .zinit zinit.zsh\n' >"$manifest"
  printf 'git missing v1.0.0\n' >"$metadata"

  set +e
  run_dependency_update "$manifest" "$metadata" >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -ne 0 ]] || fail "Unmatched metadata should fail"
  grep -Fqx 'git zinit v0.1.0 all all https://example.invalid/zinit.git - .zinit zinit.zsh' "$manifest" ||
    fail "Rejected metadata changed the manifest"
}

# End-to-end: a Zsh plugin bump must land in dependencies.conf *and* rewrite
# the matching Zinit `ver'<sha>'` pin, while leaving an unrelated plugin's
# pin (in the same target file) and an unrelated file byte-exact untouched.
test_zsh_plugin_update_rewrites_manifest_and_pin_file() {
  local manifest metadata zsh_root other_before other_after

  manifest="$TEST_ROOT/dependencies.conf"
  metadata="$TEST_ROOT/metadata"
  zsh_root="$TEST_ROOT/zsh-root"
  write_zsh_root_fixtures "$zsh_root"
  other_before="$(<"$zsh_root/common/other.zsh")"
  cat >"$manifest" <<EOF
zsh-plugin zsh-users/zsh-completions $OLD_COMPLETIONS all all https://github.com/zsh-users/zsh-completions.git - - -
zsh-plugin Aloxaf/fzf-tab $OLD_FZF_TAB all all https://github.com/Aloxaf/fzf-tab.git - - -
zsh-plugin zsh-users/zsh-autosuggestions $OLD_AUTOSUGGESTIONS all all https://github.com/zsh-users/zsh-autosuggestions.git - - -
EOF
  cat >"$metadata" <<EOF
zsh-plugin zsh-users/zsh-completions $NEW_COMPLETIONS
zsh-plugin Aloxaf/fzf-tab $NEW_FZF_TAB
EOF

  run_dependency_update "$manifest" "$metadata" "$zsh_root"

  grep -Fq "ver'$NEW_COMPLETIONS'" "$zsh_root/common/completion.zsh" ||
    fail "completion.zsh pin was not bumped to the new zsh-completions commit"
  grep -Fq "ver'$NEW_FZF_TAB'" "$zsh_root/common/interactive.zsh" ||
    fail "interactive.zsh pin was not bumped to the new fzf-tab commit"
  grep -Fq "ver'$OLD_AUTOSUGGESTIONS'" "$zsh_root/common/interactive.zsh" ||
    fail "Unrelated zsh-autosuggestions pin was not preserved"
  assert_manifest_record "$manifest" "Manifest zsh-completions entry was not bumped" \
    zsh-plugin zsh-users/zsh-completions "$NEW_COMPLETIONS" all all \
    https://github.com/zsh-users/zsh-completions.git - - -
  assert_manifest_record "$manifest" "Manifest fzf-tab entry was not bumped" \
    zsh-plugin Aloxaf/fzf-tab "$NEW_FZF_TAB" all all \
    https://github.com/Aloxaf/fzf-tab.git - - -
  assert_manifest_record "$manifest" "Unrelated manifest entry was not preserved" \
    zsh-plugin zsh-users/zsh-autosuggestions "$OLD_AUTOSUGGESTIONS" all all \
    https://github.com/zsh-users/zsh-autosuggestions.git - - -

  other_after="$(<"$zsh_root/common/other.zsh")"
  [[ "$other_before" == "$other_after" ]] || fail "An unrelated Zsh file was modified"
}

test_zsh_plugin_update_fails_when_target_pin_missing() {
  local manifest metadata zsh_root manifest_before completion_before status

  manifest="$TEST_ROOT/dependencies.conf"
  metadata="$TEST_ROOT/metadata"
  zsh_root="$TEST_ROOT/zsh-root"
  write_zsh_root_fixtures "$zsh_root"
  # dependencies.conf disagrees with the fixture's actual pin.
  cat >"$manifest" <<EOF
zsh-plugin zsh-users/zsh-completions 9999999999999999999999999999999999999999 all all https://github.com/zsh-users/zsh-completions.git - - -
EOF
  printf 'zsh-plugin zsh-users/zsh-completions %s\n' "$NEW_COMPLETIONS" >"$metadata"
  manifest_before="$(<"$manifest")"
  completion_before="$(<"$zsh_root/common/completion.zsh")"

  set +e
  run_dependency_update "$manifest" "$metadata" "$zsh_root" >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -ne 0 ]] || fail "A missing target pin should fail the update"
  [[ "$(<"$manifest")" == "$manifest_before" ]] || fail "Manifest changed despite a missing target pin"
  [[ "$(<"$zsh_root/common/completion.zsh")" == "$completion_before" ]] || fail "Pin file changed despite a missing target pin"
}

test_zsh_plugin_update_fails_when_manifest_entry_missing() {
  local manifest metadata zsh_root completion_before status

  manifest="$TEST_ROOT/dependencies.conf"
  metadata="$TEST_ROOT/metadata"
  zsh_root="$TEST_ROOT/zsh-root"
  write_zsh_root_fixtures "$zsh_root"
  : >"$manifest"
  printf 'zsh-plugin zsh-users/zsh-completions %s\n' "$NEW_COMPLETIONS" >"$metadata"
  completion_before="$(<"$zsh_root/common/completion.zsh")"

  set +e
  run_dependency_update "$manifest" "$metadata" "$zsh_root" >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -ne 0 ]] || fail "A zsh-plugin bump without a manifest entry should fail"
  [[ ! -s "$manifest" ]] || fail "Manifest should remain untouched when its entry is missing"
  [[ "$(<"$zsh_root/common/completion.zsh")" == "$completion_before" ]] || fail "Pin file changed despite a missing manifest entry"
}

test_zsh_plugin_update_rejects_invalid_commit_format() {
  local manifest metadata zsh_root manifest_before status

  manifest="$TEST_ROOT/dependencies.conf"
  metadata="$TEST_ROOT/metadata"
  zsh_root="$TEST_ROOT/zsh-root"
  write_zsh_root_fixtures "$zsh_root"
  cat >"$manifest" <<EOF
zsh-plugin zsh-users/zsh-completions $OLD_COMPLETIONS all all https://github.com/zsh-users/zsh-completions.git - - -
EOF
  printf 'zsh-plugin zsh-users/zsh-completions NOT-A-VALID-SHA\n' >"$metadata"
  manifest_before="$(<"$manifest")"

  set +e
  run_dependency_update "$manifest" "$metadata" "$zsh_root" >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -ne 0 ]] || fail "A malformed new commit should fail the update"
  [[ "$(<"$manifest")" == "$manifest_before" ]] || fail "Manifest changed despite a malformed new commit"
}

test_zsh_plugin_update_rejects_duplicate_pin_matches() {
  local manifest metadata zsh_root status

  manifest="$TEST_ROOT/dependencies.conf"
  metadata="$TEST_ROOT/metadata"
  zsh_root="$TEST_ROOT/zsh-root"
  mkdir -p "$zsh_root/common"
  cat >"$zsh_root/common/completion.zsh" <<EOF
zinit ice blockf ver'$OLD_COMPLETIONS'
zinit light zsh-users/zsh-completions
# Accidentally duplicated pin comment: ver'$OLD_COMPLETIONS'
EOF
  cat >"$manifest" <<EOF
zsh-plugin zsh-users/zsh-completions $OLD_COMPLETIONS all all https://github.com/zsh-users/zsh-completions.git - - -
EOF
  printf 'zsh-plugin zsh-users/zsh-completions %s\n' "$NEW_COMPLETIONS" >"$metadata"

  set +e
  run_dependency_update "$manifest" "$metadata" "$zsh_root" >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -ne 0 ]] || fail "Multiple matching pins in the target file should fail the update"
}

# Two plugins bump in the same run; the second one is invalid. Neither the
# manifest nor the first (individually valid) plugin's pin file may change.
test_zsh_plugin_update_is_all_or_nothing() {
  local manifest metadata zsh_root manifest_before completion_before status

  manifest="$TEST_ROOT/dependencies.conf"
  metadata="$TEST_ROOT/metadata"
  zsh_root="$TEST_ROOT/zsh-root"
  write_zsh_root_fixtures "$zsh_root"
  cat >"$manifest" <<EOF
zsh-plugin zsh-users/zsh-completions $OLD_COMPLETIONS all all https://github.com/zsh-users/zsh-completions.git - - -
zsh-plugin Aloxaf/fzf-tab 9999999999999999999999999999999999999999 all all https://github.com/Aloxaf/fzf-tab.git - - -
EOF
  cat >"$metadata" <<EOF
zsh-plugin zsh-users/zsh-completions $NEW_COMPLETIONS
zsh-plugin Aloxaf/fzf-tab $NEW_FZF_TAB
EOF
  manifest_before="$(<"$manifest")"
  completion_before="$(<"$zsh_root/common/completion.zsh")"

  set +e
  run_dependency_update "$manifest" "$metadata" "$zsh_root" >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -ne 0 ]] || fail "A partially-invalid batch of pin bumps should fail entirely"
  [[ "$(<"$manifest")" == "$manifest_before" ]] || fail "Manifest was updated despite a failure elsewhere in the batch"
  [[ "$(<"$zsh_root/common/completion.zsh")" == "$completion_before" ]] ||
    fail "An individually-valid pin was committed even though the batch failed"
}

# A newer stable candidate must land in common/mise.toml -- the sole source
# of truth for these versions -- while every other mise-managed pin
# (including node/python, which this updater never queries) stays
# byte-for-byte unchanged.
test_mise_tool_update_bumps_pin_in_mise_toml_only() {
  local manifest metadata zsh_root

  manifest="$TEST_ROOT/dependencies.conf"
  metadata="$TEST_ROOT/metadata"
  zsh_root="$TEST_ROOT/zsh-root"
  write_mise_toml_fixtures "$zsh_root"
  : >"$manifest"
  printf 'mise-tool neovim 0.12.5\n' >"$metadata"

  run_dependency_update "$manifest" "$metadata" "$zsh_root"

  grep -Fqx 'neovim = "0.12.5"' "$zsh_root/common/mise.toml" ||
    fail "common/mise.toml neovim pin was not bumped"
  grep -Fqx 'tree-sitter = "0.26.11"' "$zsh_root/common/mise.toml" ||
    fail "An untouched mise tool pin was modified"
  grep -Fqx 'node = "24.18.0"' "$zsh_root/common/mise.toml" ||
    fail "node was modified; this updater must never touch it"
  grep -Fqx 'python = "3.13.14"' "$zsh_root/common/mise.toml" ||
    fail "python was modified; this updater must never touch it"
}

# uv's real upstream history jumps from a 0.5.x pin to 0.12.x: a naive
# lexicographic comparison would treat "0.12.3" as smaller than "0.5.21"
# and wrongly skip a legitimate upgrade.
test_mise_tool_update_compares_versions_numerically() {
  local manifest metadata zsh_root

  manifest="$TEST_ROOT/dependencies.conf"
  metadata="$TEST_ROOT/metadata"
  zsh_root="$TEST_ROOT/zsh-root"
  write_mise_toml_fixtures "$zsh_root"
  : >"$manifest"
  printf 'mise-tool uv 0.12.3\n' >"$metadata"

  run_dependency_update "$manifest" "$metadata" "$zsh_root"

  grep -Fqx 'uv = "0.12.3"' "$zsh_root/common/mise.toml" ||
    fail "A numerically newer candidate with a smaller leading digit was not applied"
}

test_mise_tool_update_skips_same_or_older_candidate() {
  local manifest metadata zsh_root mise_toml_before

  manifest="$TEST_ROOT/dependencies.conf"
  metadata="$TEST_ROOT/metadata"
  zsh_root="$TEST_ROOT/zsh-root"
  write_mise_toml_fixtures "$zsh_root"
  : >"$manifest"
  printf 'mise-tool neovim 0.12.4\nmise-tool uv 0.5.20\n' >"$metadata"
  mise_toml_before="$(<"$zsh_root/common/mise.toml")"

  run_dependency_update "$manifest" "$metadata" "$zsh_root"

  [[ "$(<"$zsh_root/common/mise.toml")" == "$mise_toml_before" ]] ||
    fail "mise.toml changed for a same-or-older candidate"
}

test_mise_tool_update_rejects_non_stable_candidate_format() {
  local manifest metadata zsh_root mise_toml_before status

  manifest="$TEST_ROOT/dependencies.conf"
  metadata="$TEST_ROOT/metadata"
  zsh_root="$TEST_ROOT/zsh-root"
  write_mise_toml_fixtures "$zsh_root"
  : >"$manifest"
  printf 'mise-tool neovim nightly\n' >"$metadata"
  mise_toml_before="$(<"$zsh_root/common/mise.toml")"

  set +e
  run_dependency_update "$manifest" "$metadata" "$zsh_root" >/dev/null 2>&1
  status=$?
  set -e

  [[ "$status" -ne 0 ]] || fail "A non-semver mise-tool candidate should fail the update"
  [[ "$(<"$zsh_root/common/mise.toml")" == "$mise_toml_before" ]] ||
    fail "mise.toml changed despite a malformed candidate version"
}

test_mise_tool_update_is_idempotent_on_rerun() {
  local manifest metadata zsh_root mise_toml_after_first

  manifest="$TEST_ROOT/dependencies.conf"
  metadata="$TEST_ROOT/metadata"
  zsh_root="$TEST_ROOT/zsh-root"
  write_mise_toml_fixtures "$zsh_root"
  : >"$manifest"
  printf 'mise-tool neovim 0.12.5\n' >"$metadata"

  run_dependency_update "$manifest" "$metadata" "$zsh_root"
  mise_toml_after_first="$(<"$zsh_root/common/mise.toml")"

  run_dependency_update "$manifest" "$metadata" "$zsh_root"

  [[ "$(<"$zsh_root/common/mise.toml")" == "$mise_toml_after_first" ]] ||
    fail "Re-running the updater with the same candidate produced additional changes"
}

run_discovered_tests setup_test_home teardown_test_home
