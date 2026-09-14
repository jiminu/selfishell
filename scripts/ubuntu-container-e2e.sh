#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/selfishell-container-e2e.XXXXXX")"
INITIAL_VERSION=0.0.0-container.1
NEXT_VERSION=0.0.0-container.2
export HOME="$TEST_ROOT/home"
export XDG_CONFIG_HOME="$HOME/.config"
export XDG_DATA_HOME="$HOME/.local/share"
export XDG_STATE_HOME="$HOME/.local/state"
export XDG_CACHE_HOME="$HOME/.cache"
PREFIX="$HOME/.local"
export PATH="$PREFIX/bin:$PATH"
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
mkdir -p "$HOME"

publish_fixture "$INITIAL_VERSION"
publish_fixture "$NEXT_VERSION"

SELFISHELL_RELEASE_ROOT="file://$RELEASE_ROOT" \
  bash "$ROOT_DIR/install.sh" --version "$INITIAL_VERSION" --prefix "$PREFIX"

# Installer-owned operations must ignore conflicting caller project versions.
mkdir -p "$TEST_ROOT/project"
cat >"$TEST_ROOT/project/mise.toml" <<'EOF'
[tools]
node = "0.0.0"
neovim = "0.0.0"
starship = "0.0.0"
EOF
export MISE_TRUSTED_CONFIG_PATHS="$TEST_ROOT"
cd "$TEST_ROOT/project"

SELFISHELL_RELEASE_ROOT="file://$RELEASE_ROOT" \
  "$PREFIX/bin/selfishell" install --yes
"$PREFIX/bin/selfishell" status >/dev/null
"$PREFIX/bin/selfishell" doctor >/dev/null

# Exercise the installed shell entrypoint, not just package inventory.
(
  cd "$HOME"
  MISE_OFFLINE=1 zsh -d -i -c '
    for tool in starship fzf zoxide; do
      [[ "${commands[$tool]}" == "$XDG_DATA_HOME/mise/installs/"* ]] || exit 1
    done
    (( $+functions[prompt_starship_precmd] && $+functions[fzf-file-widget] && $+functions[__zoxide_z] ))
  '
) || fail "Installed Zsh did not initialize mise-managed shell tools"

# Config and shims survive a removed tool; doctor must still report it missing.
starship_install="$("$PREFIX/bin/mise" -C "$HOME" where starship)"
mv "$starship_install" "$TEST_ROOT/starship-not-installed"
doctor_status=0
doctor_output="$(PATH="$XDG_DATA_HOME/mise/shims:$PATH" "$PREFIX/bin/selfishell" doctor 2>&1)" || doctor_status=$?
mv "$TEST_ROOT/starship-not-installed" "$starship_install"
[[ "$doctor_status" != 0 && "$doctor_output" == *'Tool: starship is missing (mise)'* ]] ||
  fail "Doctor accepted configured-but-uninstalled Starship"

vim --not-a-term -c 'if !&number || !&relativenumber | cquit 1 | endif' -c 'q' </dev/null ||
  fail "Vim did not load Selfishell vimrc on default startup"

SELFISHELL_RELEASE_ROOT="file://$RELEASE_ROOT" \
  "$PREFIX/bin/selfishell" update --cli-only --version "$NEXT_VERSION" --yes
[[ "$("$PREFIX/bin/selfishell" version)" == "selfishell $NEXT_VERSION" ]] || fail "CLI update failed"

SELFISHELL_RELEASE_ROOT='file:///network-must-not-be-used' \
  "$PREFIX/bin/selfishell" rollback --yes
[[ "$("$PREFIX/bin/selfishell" version)" == "selfishell $INITIAL_VERSION" ]] || fail "offline rollback failed"

"$PREFIX/bin/selfishell" uninstall --restore --purge --yes
[[ ! -e "$PREFIX/bin/selfishell" && ! -e "$PREFIX/bin/sfs" ]] || fail "purge retained CLI links"
[[ ! -e "$PREFIX/share/selfishell" ]] || fail "purge retained release data"

printf 'PASS: Ubuntu 24.04 container installation lifecycle\n'
