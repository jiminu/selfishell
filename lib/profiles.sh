#!/usr/bin/env bash

profile_reset() {
  PROFILE_PLATFORMS=()
  PROFILE_REQUIREMENTS=()
  PROFILE_MANAGERS=()
  PROFILE_PACKAGES=()
}

profile_is_supported() {
  case "$1" in
    minimal | developer) return 0 ;;
    *) return 1 ;;
  esac
}

profile_add_package() {
  local platform="$1"
  local requirement="$2"
  local manager="$3"
  local package="$4"
  local index

  for ((index = 0; index < ${#PROFILE_PACKAGES[@]}; index++)); do
    if [[ "${PROFILE_PLATFORMS[$index]}" == "$platform" &&
      "${PROFILE_REQUIREMENTS[$index]}" == "$requirement" &&
      "${PROFILE_MANAGERS[$index]}" == "$manager" &&
      "${PROFILE_PACKAGES[$index]}" == "$package" ]]; then
      return
    fi
  done

  PROFILE_PLATFORMS+=("$platform")
  PROFILE_REQUIREMENTS+=("$requirement")
  PROFILE_MANAGERS+=("$manager")
  PROFILE_PACKAGES+=("$package")
}

profile_read_file() {
  local profile_file="$1"
  local allow_include="$2"
  local record
  local first
  local second
  local third
  local fourth
  local extra

  [[ -r "$profile_file" ]] || {
    cli_error "Profile file not found: $profile_file"
    return "$SELFISHELL_EXIT_ERROR"
  }

  while read -r record first second third fourth extra; do
    [[ -z "$record" || "$record" == \#* ]] && continue

    case "$record" in
      include)
        if [[ "$allow_include" != "1" || -z "$first" || -n "$second" ]]; then
          cli_error "Invalid include in profile: $profile_file"
          return "$SELFISHELL_EXIT_ERROR"
        fi
        profile_load_builtin "$first"
        ;;
      package)
        if [[ -z "$first" || -z "$second" || -z "$third" || -z "$fourth" || -n "$extra" ]]; then
          cli_error "Invalid package record in profile: $profile_file"
          return "$SELFISHELL_EXIT_ERROR"
        fi
        case "$first" in macos | ubuntu | all) ;; *)
          cli_error "Invalid package platform: $first"
          return 1
          ;;
        esac
        case "$second" in required | optional) ;; *)
          cli_error "Invalid package requirement: $second"
          return 1
          ;;
        esac
        case "$third" in apt | formula | cask | direct) ;; *)
          cli_error "Invalid package manager: $third"
          return 1
          ;;
        esac
        case "$fourth" in
          -* | *[!A-Za-z0-9@+._/-]*)
            cli_error "Invalid package name: $fourth"
            return "$SELFISHELL_EXIT_USAGE"
            ;;
        esac
        profile_add_package "$first" "$second" "$third" "$fourth"
        ;;
      *)
        cli_error "Unknown profile record: $record"
        return "$SELFISHELL_EXIT_ERROR"
        ;;
    esac
  done <"$profile_file"
}

profile_load_builtin() {
  local profile="$1"

  if ! profile_is_supported "$profile"; then
    cli_error "Unknown profile: $profile (expected: minimal or developer)"
    return "$SELFISHELL_EXIT_USAGE"
  fi

  profile_read_file "$SELFISHELL_ROOT/profiles/$profile.conf" 1
}

profile_load() {
  local profile="$1"
  local local_config="$2"

  profile_reset
  profile_load_builtin "$profile"

  if [[ -n "$local_config" ]]; then
    profile_read_file "$local_config" 0
  fi
}
