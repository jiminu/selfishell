#!/usr/bin/env bash

packages_install() {
  local platform="$1"
  local dry_run="$2"
  local index
  local package_platform
  local architecture=""
  local required_apt=()
  local optional_apt=()
  local required_formula=()
  local optional_formula=()
  local required_cask=()
  local optional_cask=()
  local required_direct=()
  local optional_direct=()
  local required_mise=()
  local optional_mise=()

  # Consumed by the apt adapter after this module is sourced.
  # shellcheck disable=SC2034
  SELFISHELL_APT_UPDATED=0
  # Consumed by the Homebrew adapter after this module is sourced.
  # shellcheck disable=SC2034
  SELFISHELL_BREW_FORMULAE=""
  # shellcheck disable=SC2034
  SELFISHELL_BREW_CASKS=""
  # shellcheck disable=SC2034
  SELFISHELL_BREW_FORMULAE_READY=0
  # shellcheck disable=SC2034
  SELFISHELL_BREW_CASKS_READY=0
  SELFISHELL_SKIPPED_OPTIONAL_PACKAGES=()
  package_platform="$(platform_package_platform "$platform")"

  for ((index = 0; index < ${#PACKAGE_NAMES[@]}; index++)); do
    if [[ "${PACKAGE_PLATFORMS[$index]}" != "all" && "${PACKAGE_PLATFORMS[$index]}" != "$package_platform" ]]; then
      continue
    fi

    case "${PACKAGE_REQUIREMENTS[$index]}:${PACKAGE_MANAGERS[$index]}" in
      required:apt) required_apt+=("${PACKAGE_NAMES[$index]}") ;;
      optional:apt) optional_apt+=("${PACKAGE_NAMES[$index]}") ;;
      required:formula) required_formula+=("${PACKAGE_NAMES[$index]}") ;;
      optional:formula) optional_formula+=("${PACKAGE_NAMES[$index]}") ;;
      required:cask) required_cask+=("${PACKAGE_NAMES[$index]}") ;;
      optional:cask) optional_cask+=("${PACKAGE_NAMES[$index]}") ;;
      required:direct) required_direct+=("${PACKAGE_NAMES[$index]}") ;;
      optional:direct) optional_direct+=("${PACKAGE_NAMES[$index]}") ;;
      required:mise) required_mise+=("${PACKAGE_NAMES[$index]}") ;;
      optional:mise) optional_mise+=("${PACKAGE_NAMES[$index]}") ;;
    esac
  done

  ((${#required_apt[@]} == 0)) || apt_install_managed_packages required "$dry_run" "${required_apt[@]}"
  ((${#optional_apt[@]} == 0)) || apt_install_managed_packages optional "$dry_run" "${optional_apt[@]}"
  ((${#required_formula[@]} == 0)) || homebrew_install_packages required formula "$dry_run" "${required_formula[@]}"
  ((${#optional_formula[@]} == 0)) || homebrew_install_packages optional formula "$dry_run" "${optional_formula[@]}"
  ((${#required_cask[@]} == 0)) || homebrew_install_packages required cask "$dry_run" "${required_cask[@]}"
  ((${#optional_cask[@]} == 0)) || homebrew_install_packages optional cask "$dry_run" "${optional_cask[@]}"

  if [[ "$dry_run" == "0" ]] && ((${#required_direct[@]} > 0 || ${#optional_direct[@]} > 0)); then
    architecture="$(detect_architecture)"
  fi

  if ((${#required_direct[@]} > 0)); then
    for index in "${required_direct[@]}"; do
      install_direct_package required "$index" "$dry_run" "$platform" "$architecture"
    done
  fi
  if ((${#optional_direct[@]} > 0)); then
    for index in "${optional_direct[@]}"; do
      install_direct_package optional "$index" "$dry_run" "$platform" "$architecture"
    done
  fi

  ((${#required_mise[@]} == 0)) || install_mise_tools required "$dry_run" "${required_mise[@]}"
  ((${#optional_mise[@]} == 0)) || install_mise_tools optional "$dry_run" "${optional_mise[@]}"

  if ((${#SELFISHELL_SKIPPED_OPTIONAL_PACKAGES[@]} > 0)); then
    cli_warn "Skipped optional packages: ${SELFISHELL_SKIPPED_OPTIONAL_PACKAGES[*]}"
  fi
}
