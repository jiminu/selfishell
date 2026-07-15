#!/usr/bin/env bash

packages_install_profile() {
  local platform="$1"
  local dry_run="$2"
  local index
  local profile_platform
  local required_apt=()
  local optional_apt=()
  local required_formula=()
  local optional_formula=()
  local required_cask=()
  local optional_cask=()
  local required_direct=()
  local optional_direct=()

  # Consumed by the apt adapter after this module is sourced.
  # shellcheck disable=SC2034
  SELFISHELL_APT_UPDATED=0
  SELFISHELL_SKIPPED_OPTIONAL_PACKAGES=()
  [[ "$platform" == "ubuntu-wsl" ]] && profile_platform=ubuntu || profile_platform="$platform"

  for ((index = 0; index < ${#PROFILE_PACKAGES[@]}; index++)); do
    if [[ "${PROFILE_PLATFORMS[$index]}" != "all" && "${PROFILE_PLATFORMS[$index]}" != "$profile_platform" ]]; then
      continue
    fi

    case "${PROFILE_REQUIREMENTS[$index]}:${PROFILE_MANAGERS[$index]}" in
      required:apt) required_apt+=("${PROFILE_PACKAGES[$index]}") ;;
      optional:apt) optional_apt+=("${PROFILE_PACKAGES[$index]}") ;;
      required:formula) required_formula+=("${PROFILE_PACKAGES[$index]}") ;;
      optional:formula) optional_formula+=("${PROFILE_PACKAGES[$index]}") ;;
      required:cask) required_cask+=("${PROFILE_PACKAGES[$index]}") ;;
      optional:cask) optional_cask+=("${PROFILE_PACKAGES[$index]}") ;;
      required:direct) required_direct+=("${PROFILE_PACKAGES[$index]}") ;;
      optional:direct) optional_direct+=("${PROFILE_PACKAGES[$index]}") ;;
    esac
  done

  ((${#required_apt[@]} == 0)) || apt_install_managed_packages required "$dry_run" "${required_apt[@]}"
  ((${#optional_apt[@]} == 0)) || apt_install_managed_packages optional "$dry_run" "${optional_apt[@]}"
  ((${#required_formula[@]} == 0)) || homebrew_install_packages required formula "$dry_run" "${required_formula[@]}"
  ((${#optional_formula[@]} == 0)) || homebrew_install_packages optional formula "$dry_run" "${optional_formula[@]}"
  ((${#required_cask[@]} == 0)) || homebrew_install_packages required cask "$dry_run" "${required_cask[@]}"
  ((${#optional_cask[@]} == 0)) || homebrew_install_packages optional cask "$dry_run" "${optional_cask[@]}"

  if ((${#required_direct[@]} > 0)); then
    for index in "${required_direct[@]}"; do
      install_direct_package required "$index" "$dry_run"
    done
  fi
  if ((${#optional_direct[@]} > 0)); then
    for index in "${optional_direct[@]}"; do
      install_direct_package optional "$index" "$dry_run"
    done
  fi

  if ((${#SELFISHELL_SKIPPED_OPTIONAL_PACKAGES[@]} > 0)); then
    cli_error "Skipped optional packages: ${SELFISHELL_SKIPPED_OPTIONAL_PACKAGES[*]}"
  fi
}
