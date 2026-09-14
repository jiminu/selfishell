#!/usr/bin/env bash

# These globals are outputs consumed by separately sourced command modules.
# shellcheck disable=SC2034

tool_status_reset_cache() {
  TOOL_STATUS_BREW_FORMULAE=""
  TOOL_STATUS_BREW_CASKS=""
  TOOL_STATUS_BREW_FORMULAE_READY=0
  TOOL_STATUS_BREW_CASKS_READY=0
  TOOL_STATUS_APT_PACKAGES=""
  TOOL_STATUS_APT_PACKAGES_READY=0
  TOOL_STATUS_MISE_VERSIONS=""
  TOOL_STATUS_MISE_APPROVED_VERSIONS=""
  TOOL_STATUS_MISE_READY=0
}

tool_status_apt_version() {
  local package="$1"
  local name version

  TOOL_STATUS_APT_VERSION=""
  if [[ "$TOOL_STATUS_APT_PACKAGES_READY" == 0 ]]; then
    TOOL_STATUS_APT_PACKAGES="$(dpkg-query -W -f='${binary:Package}\t${Version}\n' 2>/dev/null)" ||
      TOOL_STATUS_APT_PACKAGES=""
    TOOL_STATUS_APT_PACKAGES_READY=1
  fi

  while IFS=$'\t' read -r name version; do
    name="${name%%:*}"
    if [[ "$name" == "$package" && -n "$version" ]]; then
      TOOL_STATUS_APT_VERSION="$version"
      return
    fi
  done <<<"$TOOL_STATUS_APT_PACKAGES"
  return 1
}

tool_status_brew_version() {
  local manager="$1"
  local package="$2"
  local cache name output versions

  TOOL_STATUS_BREW_VERSION=""

  case "$manager" in
    formula)
      if [[ "$TOOL_STATUS_BREW_FORMULAE_READY" == 0 ]]; then
        TOOL_STATUS_BREW_FORMULAE="$(brew list --formula --versions 2>/dev/null)" ||
          TOOL_STATUS_BREW_FORMULAE=""
        TOOL_STATUS_BREW_FORMULAE_READY=1
      fi
      cache="$TOOL_STATUS_BREW_FORMULAE"
      ;;
    cask)
      if [[ "$TOOL_STATUS_BREW_CASKS_READY" == 0 ]]; then
        TOOL_STATUS_BREW_CASKS="$(brew list --cask --versions 2>/dev/null)" ||
          TOOL_STATUS_BREW_CASKS=""
        TOOL_STATUS_BREW_CASKS_READY=1
      fi
      cache="$TOOL_STATUS_BREW_CASKS"
      ;;
  esac

  while IFS=' ' read -r name versions; do
    if [[ "$name" == "$package" && -n "$versions" ]]; then
      TOOL_STATUS_BREW_VERSION="$versions"
      return
    fi
  done <<<"$cache"

  if [[ "$manager" == cask ]]; then
    return 1
  fi
  output="$(brew list --versions "$package" 2>/dev/null)" || return 1
  [[ -n "$output" ]] || return 1
  TOOL_STATUS_BREW_VERSION="${output#* }"
}

tool_status_reset_cache

tool_status_executable() {
  case "$1" in
    ripgrep) printf 'rg\n' ;;
    neovim) printf 'nvim\n' ;;
    *@*) printf '%s\n' "${1%%@*}" ;;
    *) printf '%s\n' "$1" ;;
  esac
}

tool_status_mise_version() {
  local tool="$1"
  local mise_command="" name versions

  TOOL_STATUS_MISE_VERSION=""
  TOOL_STATUS_APPROVED=""
  if [[ "$TOOL_STATUS_MISE_READY" == 0 ]]; then
    # mise.toml owns approved versions; profile records contain only tool names.
    TOOL_STATUS_MISE_APPROVED_VERSIONS="$(awk '
      /^\[/ { in_tools = ($0 == "[tools]"); next }
      in_tools && $2 == "=" {
        gsub(/[[:space:]"]/, "", $3)
        print $1, $3
      }
    ' "$SELFISHELL_ROOT/config/shared/mise.toml" 2>/dev/null)" || TOOL_STATUS_MISE_APPROVED_VERSIONS=""
    if have_command mise; then
      mise_command="$(command -v mise)"
    elif [[ -x "$HOME/.local/bin/mise" ]]; then
      mise_command="$HOME/.local/bin/mise"
    fi
    if [[ -n "$mise_command" ]]; then
      TOOL_STATUS_MISE_VERSIONS="$(MISE_GLOBAL_CONFIG_FILE="${SELFISHELL_CONFIG_DIR:-${XDG_CONFIG_HOME:-$HOME/.config}/selfishell}/mise/selfishell.toml" "$mise_command" current 2>/dev/null)" ||
        TOOL_STATUS_MISE_VERSIONS=""
    fi
    TOOL_STATUS_MISE_READY=1
  fi

  while read -r name versions; do
    if [[ "$name" == "$tool" ]]; then
      TOOL_STATUS_APPROVED="$versions"
      break
    fi
  done <<<"$TOOL_STATUS_MISE_APPROVED_VERSIONS"
  while read -r name versions; do
    if [[ "$name" == "$tool" && -n "$versions" ]]; then
      TOOL_STATUS_MISE_VERSION="$versions"
      return
    fi
  done <<<"$TOOL_STATUS_MISE_VERSIONS"
  return 1
}

tool_status_detect() {
  local manager="$1"
  local package="$2"
  local dependency_platform="$3"
  local architecture="$4"
  local output state

  TOOL_STATUS_INSTALLED="missing"
  TOOL_STATUS_SOURCE="none"
  TOOL_STATUS_APPROVED="package-manager"

  case "$manager" in
    formula)
      if have_command brew; then
        tool_status_brew_version formula "$package" || true
        output="$TOOL_STATUS_BREW_VERSION"
        if [[ -n "$output" ]]; then
          TOOL_STATUS_INSTALLED="$output"
          TOOL_STATUS_SOURCE="homebrew"
          return
        fi
      fi
      ;;
    cask)
      if have_command brew; then
        tool_status_brew_version cask "$package" || true
        output="$TOOL_STATUS_BREW_VERSION"
        if [[ -n "$output" ]]; then
          TOOL_STATUS_INSTALLED="$output"
          TOOL_STATUS_SOURCE="homebrew-cask"
          return
        fi
      fi
      ;;
    apt)
      if have_command dpkg-query; then
        tool_status_apt_version "$package" || true
        output="$TOOL_STATUS_APT_VERSION"
        if [[ -n "$output" ]]; then
          TOOL_STATUS_INSTALLED="$output"
          TOOL_STATUS_SOURCE="apt"
          return
        fi
      fi
      ;;
    direct)
      dependency_load "$package" "$dependency_platform" "$architecture" || return
      TOOL_STATUS_APPROVED="$DEPENDENCY_VERSION"
      state="$(dependency_installed_version "$package")"
      if [[ -n "$state" ]]; then
        TOOL_STATUS_SOURCE="selfishell"
        if dependency_managed_target_is_valid; then
          TOOL_STATUS_INSTALLED="$state"
        fi
        return 0
      fi
      if dependency_external_target_is_usable; then
        TOOL_STATUS_INSTALLED="detected"
        TOOL_STATUS_SOURCE="external"
        return
      fi
      ;;
    mise)
      tool_status_mise_version "$package" || true
      if [[ -n "$TOOL_STATUS_MISE_VERSION" ]]; then
        TOOL_STATUS_INSTALLED="$TOOL_STATUS_MISE_VERSION"
        TOOL_STATUS_SOURCE="mise"
        return
      fi
      ;;
  esac

  if have_command "$(tool_status_executable "$package")"; then
    TOOL_STATUS_INSTALLED="detected"
    TOOL_STATUS_SOURCE="external"
  fi
}
