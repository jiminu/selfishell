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
    printf '[TOOL] %s | Installed: %s | Source: %s | Approved: %s\n' \
      "$package" "$TOOL_STATUS_INSTALLED" "$TOOL_STATUS_SOURCE" "$TOOL_STATUS_APPROVED"
  fi
}

status_rollback_version() {
  local releases_dir share_dir previous_link previous_target version release_dir

  releases_dir="${SELFISHELL_ROOT%/*}"
  if [[ "${releases_dir##*/}" != releases ]]; then
    printf 'none\n'
    return
  fi
  share_dir="${releases_dir%/*}"
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
  local verbose=0
  local current_version="unknown"
  local rollback_version="none"
  local platform profile_platform dependency_platform architecture
  local profile=""
  local resource

  while (("$#" > 0)); do
    case "$1" in
      --verbose) verbose=1 ;;
      help | --help | -h)
        printf 'Usage: selfishell status [--verbose]\n'
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
  printf '[CLI] Current: %s | Rollback: %s\n' \
    "$current_version" "$rollback_version"

  SELFISHELL_STATUS_RESOURCE_COUNT=0
  SELFISHELL_STATUS_PACKAGES_TOTAL=0
  SELFISHELL_STATUS_PACKAGES_PRESENT=0
  SELFISHELL_STATUS_PACKAGES_MISSING=0
  SELFISHELL_STATUS_RESULT="$SELFISHELL_EXIT_OK"
  SELFISHELL_STATUS_VERBOSE="$verbose"
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
    printf '%s[SUMMARY]%s Managed paths: %s | Tools: %s present, %s missing\n' \
      "$SELFISHELL_COLOR_CYAN" "$SELFISHELL_COLOR_RESET" \
      "$SELFISHELL_STATUS_RESOURCE_COUNT" \
      "$SELFISHELL_STATUS_PACKAGES_PRESENT" \
      "$SELFISHELL_STATUS_PACKAGES_MISSING"
  fi

  return "$SELFISHELL_STATUS_RESULT"
}
