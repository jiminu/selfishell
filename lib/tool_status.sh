#!/usr/bin/env bash

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
        output="$(brew list --versions "$package" 2>/dev/null)" || output=""
        if [[ -n "$output" ]]; then
          TOOL_STATUS_INSTALLED="${output#* }"
          TOOL_STATUS_SOURCE="homebrew"
          return
        fi
      fi
      ;;
    cask)
      if have_command brew; then
        output="$(brew list --cask --versions "$package" 2>/dev/null)" || output=""
        if [[ -n "$output" ]]; then
          TOOL_STATUS_INSTALLED="${output#* }"
          TOOL_STATUS_SOURCE="homebrew-cask"
          return
        fi
      fi
      ;;
    apt)
      if have_command dpkg-query; then
        output="$(dpkg-query -W -f='${Version}' "$package" 2>/dev/null)" || output=""
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
  esac

  if have_command "$package"; then
    TOOL_STATUS_INSTALLED="detected"
    TOOL_STATUS_SOURCE="external"
  fi
}
