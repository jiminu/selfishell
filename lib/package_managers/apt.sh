#!/usr/bin/env bash

apt_install_managed_packages() {
  local requirement="$1"
  local dry_run="$2"
  shift 2
  local package
  local available_packages=()
  local unavailable_packages=()

  (("$#" > 0)) || return 0

  if [[ "$dry_run" == "1" ]]; then
    printf 'Would install %s apt packages: %s\n' "$requirement" "$*"
    return
  fi

  if ! have_command apt-get; then
    cli_error "apt-get is required to install packages."
    if [[ "$requirement" == "optional" ]]; then
      SELFISHELL_SKIPPED_OPTIONAL_PACKAGES+=("$@")
      return 0
    fi
    return 1
  fi

  if ((SELFISHELL_APT_UPDATED == 0)); then
    sudo apt-get update
    SELFISHELL_APT_UPDATED=1
  fi

  for package in "$@"; do
    if dpkg -s "$package" >/dev/null 2>&1; then
      printf 'Already installed: %s\n' "$package"
    elif apt-cache show "$package" >/dev/null 2>&1; then
      available_packages+=("$package")
    else
      unavailable_packages+=("$package")
    fi
  done

  if ((${#unavailable_packages[@]} > 0)); then
    cli_error "Unavailable $requirement apt packages: ${unavailable_packages[*]}"
    if [[ "$requirement" == "required" ]]; then
      return 1
    fi
    SELFISHELL_SKIPPED_OPTIONAL_PACKAGES+=("${unavailable_packages[@]}")
  fi

  ((${#available_packages[@]} > 0)) || return 0

  if ! sudo apt-get install -y "${available_packages[@]}"; then
    cli_error "Could not install $requirement apt packages: ${available_packages[*]}"
    if [[ "$requirement" == "optional" ]]; then
      SELFISHELL_SKIPPED_OPTIONAL_PACKAGES+=("${available_packages[@]}")
      return 0
    fi
    return 1
  fi
}
