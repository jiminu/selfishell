#!/usr/bin/env bash

apt_run_privileged() {
  if [[ "$(id -u)" == "0" ]]; then
    apt-get "$@"
    return
  fi

  if ! have_command sudo; then
    cli_error "sudo is required to install apt packages as a non-root user."
    return 1
  fi

  sudo apt-get "$@"
}

apt_install_managed_packages() {
  local requirement="$1"
  local dry_run="$2"
  shift 2
  local package
  local installed_packages=()
  local missing_packages=()
  local available_packages=()
  local unavailable_packages=()

  (("$#" > 0)) || return 0

  if [[ "$dry_run" == "1" ]]; then
    printf '%sWould install %s apt packages:%s %s\n' "$SELFISHELL_COLOR_CYAN" "$requirement" "$SELFISHELL_COLOR_RESET" "$*"
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

  for package in "$@"; do
    if dpkg -s "$package" >/dev/null 2>&1; then
      installed_packages+=("$package")
    else
      missing_packages+=("$package")
    fi
  done

  if ((${#installed_packages[@]} > 0)); then
    printf '%sAlready installed apt packages (%d):%s %s\n' "$SELFISHELL_COLOR_GREEN" "${#installed_packages[@]}" "$SELFISHELL_COLOR_RESET" "${installed_packages[*]}"
  fi

  ((${#missing_packages[@]} > 0)) || return 0

  if ((SELFISHELL_APT_UPDATED == 0)); then
    apt_run_privileged update || return 1
    SELFISHELL_APT_UPDATED=1
  fi

  for package in "${missing_packages[@]}"; do
    if apt-cache show "$package" >/dev/null 2>&1; then
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

  if ! apt_run_privileged install -y "${available_packages[@]}"; then
    cli_error "Could not install $requirement apt packages: ${available_packages[*]}"
    if [[ "$requirement" == "optional" ]]; then
      SELFISHELL_SKIPPED_OPTIONAL_PACKAGES+=("${available_packages[@]}")
      return 0
    fi
    return 1
  fi
}
