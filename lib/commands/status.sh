#!/usr/bin/env bash

status_resource() {
  local resource="$1"
  local current_checksum

  if ! managed_read_state "$resource"; then
    if managed_state_exists "$resource"; then
      printf '%s[MALFORMED]%s %s\n' "$SELFISHELL_COLOR_RED" "$SELFISHELL_COLOR_RESET" "$(managed_state_path "$resource")"
      SELFISHELL_STATUS_RESOURCE_COUNT=$((SELFISHELL_STATUS_RESOURCE_COUNT + 1))
      SELFISHELL_STATUS_RESULT="$SELFISHELL_EXIT_ERROR"
    fi
    return 0
  fi

  SELFISHELL_STATUS_RESOURCE_COUNT=$((SELFISHELL_STATUS_RESOURCE_COUNT + 1))

  if [[ "$MANAGED_STATE_STATUS" != "active" ]]; then
    printf '%s[PENDING]%s %s\n' "$SELFISHELL_COLOR_YELLOW" "$SELFISHELL_COLOR_RESET" "$MANAGED_STATE_TARGET"
    SELFISHELL_STATUS_RESULT="$SELFISHELL_EXIT_ERROR"
    return
  fi

  case "$MANAGED_STATE_TYPE" in
    block)
      if [[ -f "$MANAGED_STATE_TARGET" && ! -L "$MANAGED_STATE_TARGET" ]]; then
        managed_inspect_block "$resource" "$MANAGED_STATE_TARGET" || return
      else
        MANAGED_BLOCK_STATUS=absent
      fi
      managed_block_definition "$resource" || return
      if [[ "$MANAGED_BLOCK_STATUS" == intact && "$MANAGED_BLOCK_CHECKSUM" == "$MANAGED_STATE_CHECKSUM" ]]; then
        printf '%s[OK]%s %s (%s)\n' "$SELFISHELL_COLOR_GREEN" "$SELFISHELL_COLOR_RESET" "$MANAGED_STATE_TARGET" "$MANAGED_BLOCK_LABEL"
      else
        printf '%s[CHANGED]%s %s (%s)\n' "$SELFISHELL_COLOR_YELLOW" "$SELFISHELL_COLOR_RESET" "$MANAGED_STATE_TARGET" "$MANAGED_BLOCK_LABEL"
        SELFISHELL_STATUS_RESULT="$SELFISHELL_EXIT_ERROR"
      fi
      ;;
    link)
      if [[ -L "$MANAGED_STATE_TARGET" && "$(readlink "$MANAGED_STATE_TARGET")" == "$MANAGED_STATE_REFERENCE" ]]; then
        printf '%s[OK]%s %s -> %s\n' "$SELFISHELL_COLOR_GREEN" "$SELFISHELL_COLOR_RESET" "$MANAGED_STATE_TARGET" "$MANAGED_STATE_REFERENCE"
      else
        printf '%s[CHANGED]%s %s\n' "$SELFISHELL_COLOR_YELLOW" "$SELFISHELL_COLOR_RESET" "$MANAGED_STATE_TARGET"
        SELFISHELL_STATUS_RESULT="$SELFISHELL_EXIT_ERROR"
      fi
      ;;
    file)
      if [[ -f "$MANAGED_STATE_TARGET" ]]; then
        current_checksum="$(managed_checksum "$MANAGED_STATE_TARGET")"
      fi
      if [[ -n "$current_checksum" && "$current_checksum" == "$MANAGED_STATE_CHECKSUM" ]]; then
        printf '%s[OK]%s %s\n' "$SELFISHELL_COLOR_GREEN" "$SELFISHELL_COLOR_RESET" "$MANAGED_STATE_TARGET"
      else
        printf '%s[CHANGED]%s %s\n' "$SELFISHELL_COLOR_YELLOW" "$SELFISHELL_COLOR_RESET" "$MANAGED_STATE_TARGET"
        SELFISHELL_STATUS_RESULT="$SELFISHELL_EXIT_ERROR"
      fi
      ;;
  esac
}

status_report_package() {
  local package="$1"
  local manager="$2"
  local requirement="$3"
  local dependency_platform="$4"
  local architecture="$5"

  tool_status_detect "$manager" "$package" "$dependency_platform" "$architecture"
  if [[ "$requirement" == required && "$TOOL_STATUS_INSTALLED" == missing ]]; then
    SELFISHELL_STATUS_RESULT="$SELFISHELL_EXIT_ERROR"
  fi

  SELFISHELL_STATUS_PACKAGES_TOTAL=$((SELFISHELL_STATUS_PACKAGES_TOTAL + 1))
  if [[ "$TOOL_STATUS_INSTALLED" == missing ]]; then
    SELFISHELL_STATUS_PACKAGES_MISSING=$((SELFISHELL_STATUS_PACKAGES_MISSING + 1))
  else
    SELFISHELL_STATUS_PACKAGES_PRESENT=$((SELFISHELL_STATUS_PACKAGES_PRESENT + 1))
  fi

  if [[ "$SELFISHELL_STATUS_VERBOSE" == 1 ]]; then
    if [[ "$SELFISHELL_STATUS_CHECK_PACKAGE_UPDATES" == 1 ]]; then
      tool_status_package_update "$manager" "$package"
      printf '[TOOL] %s | Installed: %s | Source: %s | Approved: %s | Update: %s\n' \
        "$package" "$TOOL_STATUS_INSTALLED" "$TOOL_STATUS_SOURCE" "$TOOL_STATUS_APPROVED" "$TOOL_STATUS_UPDATE"
    else
      printf '[TOOL] %s | Installed: %s | Source: %s | Approved: %s\n' \
        "$package" "$TOOL_STATUS_INSTALLED" "$TOOL_STATUS_SOURCE" "$TOOL_STATUS_APPROVED"
    fi
  fi
}

status_rollback_version() {
  local releases_dir share_dir previous_link previous_target version release_dir

  releases_dir="$(dirname "$SELFISHELL_ROOT")"
  if [[ "$(basename "$releases_dir")" != releases ]]; then
    printf 'none\n'
    return
  fi
  share_dir="$(dirname "$releases_dir")"
  previous_link="$share_dir/previous"
  if [[ ! -L "$previous_link" ]]; then
    printf 'none\n'
    return
  fi

  previous_target="$(readlink "$previous_link")"
  version="${previous_target##*/}"
  release_dir="$releases_dir/$version"
  if [[ -n "$version" && -d "$release_dir" && ! -L "$release_dir" &&
    -r "$release_dir/VERSION" && "$(<"$release_dir/VERSION")" == "$version" &&
    -x "$release_dir/bin/selfishell" ]]; then
    printf '%s\n' "$version"
  else
    printf 'invalid\n'
  fi
}

command_status() {
  local check_updates=0
  local check_package_updates=0
  local verbose=0
  local current_version="unknown"
  local rollback_version="none"
  local available_version="not checked"
  local platform profile_platform dependency_platform architecture
  local profile=""
  local resource

  while (("$#" > 0)); do
    case "$1" in
      --check-updates) check_updates=1 ;;
      --check-package-updates) check_package_updates=1 ;;
      --verbose) verbose=1 ;;
      help | --help | -h)
        printf 'Usage: selfishell status [--check-updates] [--check-package-updates] [--verbose]\n'
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
  rollback_version="$(status_rollback_version)"
  if [[ "$check_updates" == 1 ]]; then
    available_version="$(release_latest_version)" || {
      cli_error "Unable to check the available CLI version."
      available_version="unavailable"
    }
  fi
  printf '[CLI] Current: %s | Rollback: %s | Available: %s\n' \
    "$current_version" "$rollback_version" "$available_version"

  SELFISHELL_STATUS_RESOURCE_COUNT=0
  SELFISHELL_STATUS_PACKAGES_TOTAL=0
  SELFISHELL_STATUS_PACKAGES_PRESENT=0
  SELFISHELL_STATUS_PACKAGES_MISSING=0
  SELFISHELL_STATUS_RESULT="$SELFISHELL_EXIT_OK"
  SELFISHELL_STATUS_VERBOSE="$verbose"
  SELFISHELL_STATUS_CHECK_PACKAGE_UPDATES="$check_package_updates"
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
    printf '%s[INFO]%s Installed profile: %s\n' "$SELFISHELL_COLOR_CYAN" "$SELFISHELL_COLOR_RESET" "$profile"
    selfishell_scan_profile_packages "$profile" "$dependency_platform" "$architecture" status_report_package "$profile_platform"
  fi

  while IFS= read -r resource; do
    if [[ "$profile" != "developer" && ("$resource" == nvim-* || "$resource" == user-nvim) ]]; then
      continue
    fi
    if [[ "$platform" != "macos" && "$resource" == user-ghostty ]]; then
      continue
    fi
    status_resource "$resource"
  done < <(selfishell_managed_resource_names)

  if ((SELFISHELL_STATUS_RESOURCE_COUNT == 0)); then
    printf 'Selfishell configuration is not installed.\n'
    return "$SELFISHELL_EXIT_ERROR"
  fi

  if [[ "$verbose" == 0 ]]; then
    printf '[SUMMARY] Managed paths: %s | Tools: %s present, %s missing\n' \
      "$SELFISHELL_STATUS_RESOURCE_COUNT" \
      "$SELFISHELL_STATUS_PACKAGES_PRESENT" \
      "$SELFISHELL_STATUS_PACKAGES_MISSING"
  fi

  return "$SELFISHELL_STATUS_RESULT"
}
