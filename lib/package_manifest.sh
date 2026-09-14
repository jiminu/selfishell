#!/usr/bin/env bash

package_manifest_reset() {
  PACKAGE_PLATFORMS=()
  PACKAGE_REQUIREMENTS=()
  PACKAGE_MANAGERS=()
  PACKAGE_NAMES=()
}

package_manifest_add() {
  local platform="$1"
  local requirement="$2"
  local manager="$3"
  local package="$4"
  local index

  for ((index = 0; index < ${#PACKAGE_NAMES[@]}; index++)); do
    if [[ "${PACKAGE_PLATFORMS[$index]}" == "$platform" &&
      "${PACKAGE_REQUIREMENTS[$index]}" == "$requirement" &&
      "${PACKAGE_MANAGERS[$index]}" == "$manager" &&
      "${PACKAGE_NAMES[$index]}" == "$package" ]]; then
      return
    fi
  done

  PACKAGE_PLATFORMS+=("$platform")
  PACKAGE_REQUIREMENTS+=("$requirement")
  PACKAGE_MANAGERS+=("$manager")
  PACKAGE_NAMES+=("$package")
}

package_manifest_read() {
  local manifest_file="$1"
  local record
  local first
  local second
  local third
  local fourth
  local extra

  [[ -r "$manifest_file" ]] || {
    cli_error "Package manifest not found: $manifest_file"
    return "$SELFISHELL_EXIT_ERROR"
  }

  while read -r record first second third fourth extra; do
    [[ -z "$record" || "$record" == \#* ]] && continue

    case "$record" in
      package)
        if [[ -z "$first" || -z "$second" || -z "$third" || -z "$fourth" || -n "$extra" ]]; then
          cli_error "Invalid package record in manifest: $manifest_file"
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
        case "$third" in apt | formula | cask | direct | mise) ;; *)
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
        package_manifest_add "$first" "$second" "$third" "$fourth"
        ;;
      *)
        cli_error "Unknown package manifest record: $record"
        return "$SELFISHELL_EXIT_ERROR"
        ;;
    esac
  done <"$manifest_file"
}

package_manifest_load() {
  package_manifest_reset
  package_manifest_read "$SELFISHELL_ROOT/packages.conf"
}
