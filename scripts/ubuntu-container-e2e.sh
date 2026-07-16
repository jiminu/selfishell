#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/selfishell-container-e2e.XXXXXX")"
INITIAL_VERSION=0.0.0-container.1
NEXT_VERSION=0.0.0-container.2
PREFIX="$HOME/.local"
RELEASE_ROOT="$TEST_ROOT/releases"

cleanup() {
  rm -rf "$TEST_ROOT"
}

fail() {
  printf 'Container E2E failed: %s\n' "$*" >&2
  exit 1
}

publish_fixture() {
  local version="$1"
  local artifacts="$TEST_ROOT/artifacts-$version"
  local release_dir="$RELEASE_ROOT/download/v$version"

  mkdir -p "$artifacts" "$release_dir"
  bash "$ROOT_DIR/scripts/build-release.sh" --version "$version" --output "$artifacts" >/dev/null
  cp "$artifacts"/* "$release_dir/"
}

trap cleanup EXIT HUP INT TERM

[[ "$(id -u)" == 0 ]] || fail "the Ubuntu container test must run as root"
command -v sudo >/dev/null 2>&1 && fail "the test image unexpectedly contains sudo"

publish_fixture "$INITIAL_VERSION"
publish_fixture "$NEXT_VERSION"

SELFISHELL_RELEASE_ROOT="file://$RELEASE_ROOT" \
  bash "$ROOT_DIR/install.sh" --version "$INITIAL_VERSION" --prefix "$PREFIX" --add-to-path

bash --noprofile --rcfile "$HOME/.bashrc" -ic \
  'command -v selfishell >/dev/null && [[ "$(selfishell version)" == "selfishell 0.0.0-container.1" ]]' ||
  fail "a new Bash shell did not load the installer-managed PATH"

SELFISHELL_RELEASE_ROOT="file://$RELEASE_ROOT" \
  "$PREFIX/bin/selfishell" install --profile minimal --yes
"$PREFIX/bin/selfishell" status >/dev/null
"$PREFIX/bin/selfishell" doctor >/dev/null

SELFISHELL_RELEASE_ROOT="file://$RELEASE_ROOT" \
  "$PREFIX/bin/selfishell" update --cli-only --version "$NEXT_VERSION" --yes
[[ "$("$PREFIX/bin/selfishell" version)" == "selfishell $NEXT_VERSION" ]] || fail "CLI update failed"

SELFISHELL_RELEASE_ROOT='file:///network-must-not-be-used' \
  "$PREFIX/bin/selfishell" rollback --yes
[[ "$("$PREFIX/bin/selfishell" version)" == "selfishell $INITIAL_VERSION" ]] || fail "offline rollback failed"

"$PREFIX/bin/selfishell" uninstall --restore --purge --yes
[[ ! -e "$PREFIX/bin/selfishell" && ! -e "$PREFIX/bin/sfs" ]] || fail "purge retained CLI links"
[[ ! -e "$PREFIX/share/selfishell" ]] || fail "purge retained release data"
! grep -Fq '# Added by Selfishell installer' "$HOME/.bashrc" || fail "purge retained the PATH entry"

printf 'PASS: Ubuntu 24.04 container installation lifecycle\n'
