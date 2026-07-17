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
  TOOL_STATUS_BREW_OUTDATED_FORMULAE=""
  TOOL_STATUS_BREW_OUTDATED_CASKS=""
  TOOL_STATUS_BREW_OUTDATED_FORMULAE_READY=0
  TOOL_STATUS_BREW_OUTDATED_CASKS_READY=0
  TOOL_STATUS_APT_UPGRADABLE=""
  TOOL_STATUS_APT_UPGRADABLE_READY=0
}

tool_status_package_update() {
  local manager="$1"
  local package="$2"
  local inventory=""
  local entry

  TOOL_STATUS_UPDATE="unknown"
  case "$manager" in
    formula)
      have_command brew || return
      if [[ "$TOOL_STATUS_BREW_OUTDATED_FORMULAE_READY" == 0 ]]; then
        TOOL_STATUS_BREW_OUTDATED_FORMULAE="$(brew outdated --formula 2>/dev/null)" ||
          TOOL_STATUS_BREW_OUTDATED_FORMULAE=""
        TOOL_STATUS_BREW_OUTDATED_FORMULAE_READY=1
      fi
      inventory="$TOOL_STATUS_BREW_OUTDATED_FORMULAE"
      ;;
    cask)
      have_command brew || return
      if [[ "$TOOL_STATUS_BREW_OUTDATED_CASKS_READY" == 0 ]]; then
        TOOL_STATUS_BREW_OUTDATED_CASKS="$(brew outdated --cask 2>/dev/null)" ||
          TOOL_STATUS_BREW_OUTDATED_CASKS=""
        TOOL_STATUS_BREW_OUTDATED_CASKS_READY=1
      fi
      inventory="$TOOL_STATUS_BREW_OUTDATED_CASKS"
      ;;
    apt)
      have_command apt || return
      if [[ "$TOOL_STATUS_APT_UPGRADABLE_READY" == 0 ]]; then
        TOOL_STATUS_APT_UPGRADABLE="$(apt list --upgradable 2>/dev/null)" ||
          TOOL_STATUS_APT_UPGRADABLE=""
        TOOL_STATUS_APT_UPGRADABLE_READY=1
      fi
      inventory="$TOOL_STATUS_APT_UPGRADABLE"
      ;;
    *)
      TOOL_STATUS_UPDATE="selfishell-managed"
      return
      ;;
  esac

  TOOL_STATUS_UPDATE="current"
  while IFS= read -r entry; do
    entry="${entry%% *}"
    entry="${entry%%/*}"
    [[ "$entry" == "$package" ]] || continue
    TOOL_STATUS_UPDATE="available"
    return
  done <<<"$inventory"
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
    *@*) printf '%s\n' "${1%%@*}" ;;
    *) printf '%s\n' "$1" ;;
  esac
}

tool_status_detect() {
  local manager="$1"
  local package="$2"
  local dependency_platform="$3"
  local architecture="$4"
  local output state marker_path

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
      if [[ "$DEPENDENCY_TYPE" == git ]]; then
        marker_path="$DEPENDENCY_TARGET/$DEPENDENCY_MARKER"
      else
        marker_path="$DEPENDENCY_TARGET"
      fi
      if [[ -n "$state" ]]; then
        TOOL_STATUS_SOURCE="selfishell"
        [[ -e "$marker_path" ]] && TOOL_STATUS_INSTALLED="$state"
        return 0
      fi
      if [[ -e "$marker_path" ]]; then
        TOOL_STATUS_INSTALLED="detected"
        TOOL_STATUS_SOURCE="external"
        return
      fi
      ;;
    mise)
      local mise_tool="${package%%@*}"
      local mise_version="${package#*@}"
      local mise_command=""
      TOOL_STATUS_APPROVED="$mise_version"
      if have_command mise; then
        mise_command="$(command -v mise)"
      elif [[ -x "$HOME/.local/bin/mise" ]]; then
        mise_command="$HOME/.local/bin/mise"
      fi
      if [[ -n "$mise_command" ]]; then
        output="$(MISE_GLOBAL_CONFIG_FILE="${SELFISHELL_CONFIG_DIR:-${XDG_CONFIG_HOME:-$HOME/.config}/selfishell}/mise/config.toml" "$mise_command" current "$mise_tool" 2>/dev/null)" || output=""
        if [[ -n "$output" ]]; then
          TOOL_STATUS_INSTALLED="$output"
          TOOL_STATUS_SOURCE="mise"
          return
        fi
      fi
      ;;
  esac

  if have_command "$(tool_status_executable "$package")"; then
    TOOL_STATUS_INSTALLED="detected"
    TOOL_STATUS_SOURCE="external"
  fi
}
