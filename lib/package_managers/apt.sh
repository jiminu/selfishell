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
    if [[ "$requirement" == "optional" ]]; then
      cli_warn "apt-get is required to install packages."
      SELFISHELL_SKIPPED_OPTIONAL_PACKAGES+=("$@")
      return 0
    fi
    cli_error "apt-get is required to install packages."
    return 1
  fi

  for package in "$@"; do
    # dpkg -s exits 0 for a package dpkg still has any record of, including
    # one that was removed but not purged ("rc" status: config files remain,
    # binaries gone) -- which would silently skip reinstalling it. Checking
    # the actual Status field distinguishes that from a real "ii" install.
    if dpkg-query -W -f='${Status}\n' "$package" 2>/dev/null | grep -q '^install ok installed$'; then
      installed_packages+=("$package")
    else
      missing_packages+=("$package")
    fi
  done

  if ((${#installed_packages[@]} > 0)); then
    printf '%sAlready installed apt packages (%d):%s %s\n' "$SELFISHELL_COLOR_CYAN" "${#installed_packages[@]}" "$SELFISHELL_COLOR_RESET" "${installed_packages[*]}"
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
    if [[ "$requirement" == "required" ]]; then
      cli_error "Unavailable $requirement apt packages: ${unavailable_packages[*]}"
      return 1
    fi
    cli_warn "Unavailable $requirement apt packages: ${unavailable_packages[*]}"
    SELFISHELL_SKIPPED_OPTIONAL_PACKAGES+=("${unavailable_packages[@]}")
  fi

  ((${#available_packages[@]} > 0)) || return 0

  if ! apt_run_privileged install -y "${available_packages[@]}"; then
    if [[ "$requirement" == "optional" ]]; then
      cli_warn "Could not install $requirement apt packages: ${available_packages[*]}"
      SELFISHELL_SKIPPED_OPTIONAL_PACKAGES+=("${available_packages[@]}")
      return 0
    fi
    cli_error "Could not install $requirement apt packages: ${available_packages[*]}"
    return 1
  fi
}
