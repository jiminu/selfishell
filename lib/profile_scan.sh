#!/usr/bin/env bash

selfishell_profile_platforms() {
  case "$1" in
    ubuntu | ubuntu-wsl)
      printf 'linux\nubuntu\n'
      ;;
    *)
      printf '%s\n%s\n' "$1" "$1"
      ;;
  esac
}

selfishell_scan_profile_packages() {
  local profile="$1"
  local dependency_platform="$2"
  local architecture="$3"
  local callback="$4"
  local profile_platform="$5"
  local index package manager requirement key=""

  profile_load "$profile" "${SELFISHELL_LOCAL_PROFILE:-}"

  for ((index = 0; index < ${#PROFILE_PACKAGES[@]}; index++)); do
    [[ "${PROFILE_PLATFORMS[$index]}" == all || "${PROFILE_PLATFORMS[$index]}" == "$profile_platform" ]] || continue
    package="${PROFILE_PACKAGES[$index]}"
    [[ "$key" != *"|$package|"* ]] || continue
    key="${key}|${package}|"
    manager="${PROFILE_MANAGERS[$index]}"
    requirement="${PROFILE_REQUIREMENTS[$index]}"
    "$callback" "$package" "$manager" "$requirement" "$dependency_platform" "$architecture"
  done
}
