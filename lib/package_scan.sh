#!/usr/bin/env bash

selfishell_scan_packages() {
  local dependency_platform="$1"
  local architecture="$2"
  local callback="$3"
  local package_platform="$4"
  local index package manager requirement key=""

  package_manifest_load

  for ((index = 0; index < ${#PACKAGE_NAMES[@]}; index++)); do
    [[ "${PACKAGE_PLATFORMS[$index]}" == all || "${PACKAGE_PLATFORMS[$index]}" == "$package_platform" ]] || continue
    package="${PACKAGE_NAMES[$index]}"
    [[ "$key" != *"|$package|"* ]] || continue
    key="${key}|${package}|"
    manager="${PACKAGE_MANAGERS[$index]}"
    requirement="${PACKAGE_REQUIREMENTS[$index]}"
    "$callback" "$package" "$manager" "$requirement" "$dependency_platform" "$architecture"
  done
}
