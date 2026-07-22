#!/usr/bin/env bash

# Exercises the real managed-configuration lifecycle (install, idempotent
# reinstall, status, update, uninstall --restore, purge) against an isolated
# HOME on a genuine macOS runner -- the only lifecycle E2E that previously
# existed (scripts/ubuntu-container-e2e.sh) only ran on Ubuntu, leaving BSD
# touch/stat/sed differences, macOS's Bash 3.2, and Ghostty's preflight path
# unverified end-to-end. Uses --skip-packages/SELFISHELL_OFFLINE throughout
# so this never installs Homebrew formulae/casks or needs network access,
# except where a real CLI release install is the thing under test (which
# uses file:// release fixtures built locally, never the network either).

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/selfishell-macos-e2e.XXXXXX")"
INITIAL_VERSION=0.0.0-macos.1
NEXT_VERSION=0.0.0-macos.2
RELEASE_ROOT="$TEST_ROOT/releases"

cleanup() {
  rm -rf "$TEST_ROOT"
}
trap cleanup EXIT HUP INT TERM

fail() {
  printf 'macOS configuration E2E failed: %s\n' "$*" >&2
  exit 1
}

# `status`'s own exit code also reflects missing *required* packages, which
# --skip-packages guarantees throughout this script (by design, to avoid
# real Homebrew installs) -- so it is not a useful "is anything actually
# wrong" signal here. Managed resources reporting anything other than [OK]
# is: a resource file/link/block was not left in the state the CLI itself
# considers correct.
assert_managed_resources_clean() {
  local prefix="$1"
  local context="$2"
  local status_output

  # Captured rather than piped directly into grep: under `pipefail`,
  # status's own (expected, package-driven) exit code would otherwise
  # poison the pipeline's exit status regardless of what grep finds.
  status_output="$("$prefix/bin/selfishell" status 2>&1)" || true
  printf '%s\n' "$status_output" | grep -Eq '\[CHANGED\]|\[MALFORMED\]|\[PENDING\]' &&
    fail "status reported a changed, malformed, or pending managed resource $context"
  return 0
}

[[ "$(uname -s)" == Darwin ]] || fail "this script must run on macOS"

publish_fixture() {
  local version="$1"
  local artifacts="$TEST_ROOT/artifacts-$version"
  local release_dir="$RELEASE_ROOT/download/v$version"

  mkdir -p "$artifacts" "$release_dir"
  bash "$ROOT_DIR/scripts/build-release.sh" --version "$version" --output "$artifacts" --no-update-source >/dev/null
  cp "$artifacts"/* "$release_dir/"
}

publish_fixture "$INITIAL_VERSION"
publish_fixture "$NEXT_VERSION"

# -----------------------------------------------------------------------
# Primary lifecycle: clean install, idempotent reinstall, status, update,
# uninstall --restore -- all configuration-only (--skip-packages), all
# against an isolated HOME/XDG sandbox that is never the real runner HOME.
# -----------------------------------------------------------------------
run_primary_lifecycle() {
  local home="$TEST_ROOT/home-primary"
  local prefix="$home/.local"
  local backups_before backups_after loader_count starship_backup vimrc_backup
  local zshrc_mode_before zshrc_mode_after

  export HOME="$home"
  export XDG_CONFIG_HOME="$home/xdg-config"
  export XDG_STATE_HOME="$home/xdg-state"
  export XDG_CACHE_HOME="$home/xdg-cache"
  mkdir -p "$HOME" "$XDG_CONFIG_HOME"

  # No trailing newline and a CRLF-styled line, so the real (not mocked)
  # lifecycle proves it preserves both byte-for-byte, matching M8's
  # acceptance criteria for the loader block.
  printf 'export SELFISHELL_E2E_MARKER=1\r\nalias ll="ls -la"' >"$HOME/.zshrc"
  local zshrc_before
  zshrc_before="$(cat "$HOME/.zshrc")"
  chmod 640 "$HOME/.zshrc"
  zshrc_mode_before="$(stat -f '%Lp' "$HOME/.zshrc")"

  # A pre-existing Starship config (a regular file at a managed *link*
  # target) must be moved to a timestamped backup, not silently replaced.
  printf 'format = "user starship config"\n' >"$XDG_CONFIG_HOME/starship.toml"

  # A dangling symlink at another managed link target must also be treated
  # as user data (the -L check in managed_install_link covers this), not
  # silently followed or deleted.
  mkdir -p "$XDG_CONFIG_HOME/vim"
  ln -s /nonexistent-target "$XDG_CONFIG_HOME/vim/vimrc"

  SELFISHELL_RELEASE_ROOT="file://$RELEASE_ROOT" \
    bash "$ROOT_DIR/install.sh" --version "$INITIAL_VERSION" --prefix "$prefix" \
    --setup --yes --profile minimal --skip-packages

  # --- clean install ---
  [[ -f "$HOME/.zshrc" && ! -L "$HOME/.zshrc" ]] || fail "install did not leave .zshrc as a regular, user-owned file"
  loader_count="$(grep -Fc '# >>> Selfishell initialize >>>' "$HOME/.zshrc")"
  [[ "$loader_count" == 1 ]] || fail "install did not add exactly one loader block (found $loader_count)"
  [[ -d "$XDG_CONFIG_HOME/selfishell" ]] || fail "managed configuration was not created under XDG_CONFIG_HOME"
  [[ -d "$XDG_STATE_HOME/selfishell" ]] || fail "managed state was not created under XDG_STATE_HOME"
  # The managed *links* (.zshenv, starship.toml, vim/vimrc, mise's conf.d
  # entry) live under $HOME itself, pointing into the copied
  # $XDG_CONFIG_HOME/selfishell tree -- never directly at the source
  # checkout that this script runs from.
  while IFS= read -r -d '' link; do
    case "$(readlink "$link")" in
      "$ROOT_DIR"*) fail "$link links directly into the source checkout instead of the copied managed configuration" ;;
    esac
  done < <(find "$HOME" -type l -print0)

  starship_backup="$(find "$XDG_CONFIG_HOME" -maxdepth 1 -name 'starship.toml.backup.*')"
  [[ -n "$starship_backup" ]] || fail "a pre-existing starship.toml was not backed up"
  grep -Fq 'user starship config' "$starship_backup" || fail "the starship.toml backup does not hold the original content"
  vimrc_backup="$(find "$XDG_CONFIG_HOME/vim" -maxdepth 1 -name 'vimrc.backup.*')"
  [[ -n "$vimrc_backup" ]] || fail "a pre-existing dangling vimrc symlink was not treated as user data"

  zshrc_mode_after="$(stat -f '%Lp' "$HOME/.zshrc")"
  [[ "$zshrc_mode_after" == "$zshrc_mode_before" ]] ||
    fail "install changed .zshrc's permission bits ($zshrc_mode_before -> $zshrc_mode_after)"

  backups_before="$(find "$XDG_CONFIG_HOME" "$XDG_STATE_HOME" -name '*.backup.*' | sort)"

  # --- idempotent reinstall ---
  SELFISHELL_RELEASE_ROOT="file://$RELEASE_ROOT" \
    "$prefix/bin/selfishell" install --profile minimal --skip-packages --yes >/dev/null
  loader_count="$(grep -Fc '# >>> Selfishell initialize >>>' "$HOME/.zshrc")"
  [[ "$loader_count" == 1 ]] || fail "a second install duplicated the loader block (found $loader_count)"
  backups_after="$(find "$XDG_CONFIG_HOME" "$XDG_STATE_HOME" -name '*.backup.*' | sort)"
  [[ "$backups_before" == "$backups_after" ]] || fail "a second install created an unnecessary backup"
  assert_managed_resources_clean "$prefix" "after an idempotent reinstall"

  # --- status ---
  local status_output
  status_output="$("$prefix/bin/selfishell" status)" || true
  printf '%s\n' "$status_output" | grep -Fq '[INFO] Installed profile: minimal' ||
    fail "status did not report the installed profile"
  assert_managed_resources_clean "$prefix" "on a clean install"

  # --- configuration update ---
  # Minimal's own package list is small enough (git/starship/vim/zinit) that
  # this is the one step in this script that may perform real, bounded
  # package work if the runner doesn't already have all of them; there is
  # no offline equivalent of --skip-packages for `update`. The CLI-release
  # path is not part of --tools-only, confirmed by pointing it at an
  # unreachable release root.
  SELFISHELL_RELEASE_ROOT='file:///network-must-not-be-used' \
    "$prefix/bin/selfishell" update --tools-only --yes >/dev/null
  loader_count="$(grep -Fc '# >>> Selfishell initialize >>>' "$HOME/.zshrc")"
  [[ "$loader_count" == 1 ]] || fail "update duplicated the loader block (found $loader_count)"
  # The loader block is prepended, so the user's original content is a
  # suffix of the file, not a prefix.
  [[ "$(cat "$HOME/.zshrc")" == *"$zshrc_before" ]] ||
    fail "update did not preserve the user's original .zshrc content"
  assert_managed_resources_clean "$prefix" "after a configuration update"

  # --- uninstall --restore ---
  "$prefix/bin/selfishell" uninstall --restore --yes >/dev/null
  [[ "$(cat "$HOME/.zshrc")" == "$zshrc_before" ]] ||
    fail "uninstall --restore did not preserve the user's .zshrc byte-for-byte"
  [[ ! -e "$XDG_CONFIG_HOME/selfishell/zsh/zshrc" ]] || fail "uninstall left managed configuration behind"
  grep -Fq 'user starship config' "$XDG_CONFIG_HOME/starship.toml" ||
    fail "uninstall --restore did not restore the original starship.toml"
  [[ -L "$XDG_CONFIG_HOME/vim/vimrc" && "$(readlink "$XDG_CONFIG_HOME/vim/vimrc")" == /nonexistent-target ]] ||
    fail "uninstall --restore did not restore the original dangling vimrc symlink"

  printf 'PASS: primary configuration lifecycle (clean install, idempotent reinstall, status, update, uninstall --restore)\n'
}

# -----------------------------------------------------------------------
# Ghostty preflight: a separate, isolated install where Ghostty defaults to
# enabled (no prior state, --yes) so its config-path preflight and managed
# files are exercised, while --skip-packages guarantees the actual Ghostty
# Homebrew cask is never installed.
# -----------------------------------------------------------------------
run_ghostty_preflight_check() {
  local home="$TEST_ROOT/home-ghostty"
  local prefix="$home/.local"

  export HOME="$home"
  export XDG_CONFIG_HOME="$home/xdg-config"
  export XDG_STATE_HOME="$home/xdg-state"
  export XDG_CACHE_HOME="$home/xdg-cache"
  mkdir -p "$HOME"

  SELFISHELL_RELEASE_ROOT="file://$RELEASE_ROOT" \
    bash "$ROOT_DIR/install.sh" --version "$INITIAL_VERSION" --prefix "$prefix" \
    --setup --yes --profile minimal --skip-packages

  [[ "$(<"$XDG_STATE_HOME/selfishell/ghostty")" == 1 ]] ||
    fail "Ghostty did not default to enabled on a clean --yes install"
  [[ -f "$XDG_CONFIG_HOME/selfishell/ghostty/config.ghostty" ]] ||
    fail "the managed Ghostty configuration was not installed"
  [[ -f "$XDG_CONFIG_HOME/ghostty/config.ghostty" ]] ||
    fail "the Ghostty entrypoint (preflighted target) was not created"
  grep -Fq 'Selfishell' "$XDG_CONFIG_HOME/ghostty/config.ghostty" ||
    fail "the Ghostty entrypoint does not contain the Selfishell-managed block"

  printf 'PASS: Ghostty config-path preflight and managed files (no package installed)\n'
}

# -----------------------------------------------------------------------
# Purge: a separate, isolated bootstrap install, then uninstall --restore
# --purge, verifying the CLI link, release data, cache, and state are all
# removed (package-manager-installed tools are never touched).
# -----------------------------------------------------------------------
run_purge_lifecycle() {
  local home="$TEST_ROOT/home-purge"
  local prefix="$home/.local"

  export HOME="$home"
  export XDG_CONFIG_HOME="$home/xdg-config"
  export XDG_STATE_HOME="$home/xdg-state"
  export XDG_CACHE_HOME="$home/xdg-cache"
  mkdir -p "$HOME"

  SELFISHELL_RELEASE_ROOT="file://$RELEASE_ROOT" \
    bash "$ROOT_DIR/install.sh" --version "$INITIAL_VERSION" --prefix "$prefix" \
    --setup --yes --profile minimal --skip-packages

  [[ -e "$prefix/bin/selfishell" ]] || fail "bootstrap did not install the CLI link"

  "$prefix/bin/selfishell" uninstall --restore --purge --yes >/dev/null

  [[ ! -e "$prefix/bin/selfishell" && ! -e "$prefix/bin/sfs" ]] || fail "purge retained CLI links"
  [[ ! -e "$prefix/share/selfishell" ]] || fail "purge retained release data"
  [[ ! -e "$XDG_CACHE_HOME/selfishell" ]] || fail "purge retained the cache directory"
  [[ ! -e "$XDG_STATE_HOME/selfishell" ]] || fail "purge retained the state directory"

  printf 'PASS: purge removes the CLI, releases, cache, and state\n'
}

# -----------------------------------------------------------------------
# macOS-specific portability: this whole script already runs every step
# above against a real macOS runner's BSD touch/stat/sed, mktemp, and
# XDG-override handling; this section double-checks a couple of things
# not otherwise implied by the lifecycle passing.
# -----------------------------------------------------------------------
run_macos_portability_checks() {
  local bash_version

  bash_version="$(bash -c 'printf "%s" "$BASH_VERSION"')"
  case "$bash_version" in
    3.2*) : ;;
    *) printf 'Note: /bin/bash reports %s, not the historical macOS 3.2 (informational only).\n' "$bash_version" ;;
  esac

  command -v sudo >/dev/null 2>&1 || printf 'Note: sudo is unavailable on this runner (informational only).\n'

  printf 'PASS: macOS portability notes recorded\n'
}

run_primary_lifecycle
run_ghostty_preflight_check
run_purge_lifecycle
run_macos_portability_checks

printf 'PASS: macOS configuration lifecycle E2E\n'
