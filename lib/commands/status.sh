#!/usr/bin/env bash

status_resource() {
  local resource="$1"
  local current_checksum

  if ! managed_read_state "$resource"; then
    return 0
  fi

  SELFISHELL_STATUS_RESOURCE_COUNT=$((SELFISHELL_STATUS_RESOURCE_COUNT + 1))

  if [[ "$MANAGED_STATE_STATUS" != "active" ]]; then
    printf '[PENDING] %s\n' "$MANAGED_STATE_TARGET"
    SELFISHELL_STATUS_RESULT="$SELFISHELL_EXIT_ERROR"
    return
  fi

  case "$MANAGED_STATE_TYPE" in
    link)
      if [[ -L "$MANAGED_STATE_TARGET" && "$(readlink "$MANAGED_STATE_TARGET")" == "$MANAGED_STATE_REFERENCE" ]]; then
        printf '[OK] %s -> %s\n' "$MANAGED_STATE_TARGET" "$MANAGED_STATE_REFERENCE"
      else
        printf '[CHANGED] %s\n' "$MANAGED_STATE_TARGET"
        SELFISHELL_STATUS_RESULT="$SELFISHELL_EXIT_ERROR"
      fi
      ;;
    file)
      if [[ -f "$MANAGED_STATE_TARGET" ]]; then
        current_checksum="$(managed_checksum "$MANAGED_STATE_TARGET")"
      fi
      if [[ -n "$current_checksum" && "$current_checksum" == "$MANAGED_STATE_CHECKSUM" ]]; then
        printf '[OK] %s\n' "$MANAGED_STATE_TARGET"
      else
        printf '[CHANGED] %s\n' "$MANAGED_STATE_TARGET"
        SELFISHELL_STATUS_RESULT="$SELFISHELL_EXIT_ERROR"
      fi
      ;;
  esac
}

command_status() {
  local check_updates=0
  local check_package_updates=0
  local current_version="unknown"
  local available_version="not checked"
  local platform profile_platform dependency_platform architecture
  local profile index package manager requirement key=""
  local resource

  while (("$#" > 0)); do
    case "$1" in
      --check-updates) check_updates=1 ;;
      --check-package-updates) check_package_updates=1 ;;
      help | --help | -h)
        printf 'Usage: selfishell status [--check-updates] [--check-package-updates]\n'
        return
        ;;
      *)
        cli_error "Unknown status option: $1"
        return "$SELFISHELL_EXIT_USAGE"
        ;;
    esac
    shift
  done
  selfishell_initialize_paths

  [[ -r "$SELFISHELL_ROOT/VERSION" ]] && current_version="$(<"$SELFISHELL_ROOT/VERSION")"
  if [[ "$check_updates" == 1 ]]; then
    available_version="$(release_latest_version)" || {
      cli_error "Unable to check the available CLI version."
      available_version="unavailable"
    }
  fi
  printf '[CLI] Current: %s | Available: %s\n' "$current_version" "$available_version"

  SELFISHELL_STATUS_RESOURCE_COUNT=0
  SELFISHELL_STATUS_RESULT="$SELFISHELL_EXIT_OK"
  tool_status_reset_cache

  platform="$(detect_platform)"
  case "$platform" in
    ubuntu | ubuntu-wsl)
      dependency_platform=linux
      profile_platform=ubuntu
      ;;
    *)
      dependency_platform="$platform"
      profile_platform="$platform"
      ;;
  esac
  architecture="$(detect_architecture)"

  if [[ -r "$SELFISHELL_STATE_DIR/profile" ]]; then
    profile="$(<"$SELFISHELL_STATE_DIR/profile")"
    printf '[INFO] Profile: %s\n' "$profile"
    profile_load "$profile" "${SELFISHELL_LOCAL_PROFILE:-}"

    for ((index = 0; index < ${#PROFILE_PACKAGES[@]}; index++)); do
      [[ "${PROFILE_PLATFORMS[$index]}" == all || "${PROFILE_PLATFORMS[$index]}" == "$profile_platform" ]] || continue
      package="${PROFILE_PACKAGES[$index]}"
      [[ "$key" != *"|$package|"* ]] || continue
      key="${key}|${package}|"
      manager="${PROFILE_MANAGERS[$index]}"
      requirement="${PROFILE_REQUIREMENTS[$index]}"
      tool_status_detect "$manager" "$package" "$dependency_platform" "$architecture"
      if [[ "$check_package_updates" == 1 ]]; then
        tool_status_package_update "$manager" "$package"
        printf '[TOOL] %s | Installed: %s | Source: %s | Approved: %s | Update: %s\n' \
          "$package" "$TOOL_STATUS_INSTALLED" "$TOOL_STATUS_SOURCE" "$TOOL_STATUS_APPROVED" "$TOOL_STATUS_UPDATE"
      else
        printf '[TOOL] %s | Installed: %s | Source: %s | Approved: %s\n' \
          "$package" "$TOOL_STATUS_INSTALLED" "$TOOL_STATUS_SOURCE" "$TOOL_STATUS_APPROVED"
      fi
      if [[ "$requirement" == required && "$TOOL_STATUS_INSTALLED" == missing ]]; then
        SELFISHELL_STATUS_RESULT="$SELFISHELL_EXIT_ERROR"
      fi
    done
  fi

  while IFS= read -r resource; do
    status_resource "$resource"
  done < <(selfishell_managed_resource_names)

  if ((SELFISHELL_STATUS_RESOURCE_COUNT == 0)); then
    printf 'Selfishell configuration is not installed.\n'
    return "$SELFISHELL_EXIT_ERROR"
  fi

  return "$SELFISHELL_STATUS_RESULT"
}
