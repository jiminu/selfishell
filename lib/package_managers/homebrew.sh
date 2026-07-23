#!/usr/bin/env bash

homebrew_activate() {
  if [[ -x /opt/homebrew/bin/brew ]]; then
    eval "$(/opt/homebrew/bin/brew shellenv)"
  elif [[ -x /usr/local/bin/brew ]]; then
    eval "$(/usr/local/bin/brew shellenv)"
  fi
}

homebrew_ensure_installed() {
  if have_command brew; then
    return
  fi

  printf 'Installing Homebrew\n'
  /bin/bash -c "$(selfishell_curl transfer https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
  homebrew_activate
  have_command brew
}

homebrew_package_installed() {
  local manager="$1"
  local package="$2"
  local inventory

  if [[ "$manager" == cask ]]; then
    if [[ "${SELFISHELL_BREW_CASKS_READY:-0}" == 0 ]]; then
      SELFISHELL_BREW_CASKS="$(brew list --cask 2>/dev/null)" || SELFISHELL_BREW_CASKS=""
      SELFISHELL_BREW_CASKS_READY=1
    fi
    inventory="$SELFISHELL_BREW_CASKS"
  else
    if [[ "${SELFISHELL_BREW_FORMULAE_READY:-0}" == 0 ]]; then
      SELFISHELL_BREW_FORMULAE="$(brew list --formula 2>/dev/null)" || SELFISHELL_BREW_FORMULAE=""
      SELFISHELL_BREW_FORMULAE_READY=1
    fi
    inventory="$SELFISHELL_BREW_FORMULAE"
  fi

  while IFS= read -r installed; do
    [[ "$installed" == "$package" ]] && return 0
  done <<<"$inventory"
  return 1
}

homebrew_install_packages() {
  local requirement="$1"
  local manager="$2"
  local dry_run="$3"
  local package
  local installed_packages=()
  local missing_packages=()
  shift 3

  (("$#" > 0)) || return 0

  if [[ "$dry_run" == "1" ]]; then
    printf '%sWould install %s Homebrew %s:%s %s\n' "$SELFISHELL_COLOR_CYAN" "$requirement" "$manager" "$SELFISHELL_COLOR_RESET" "$*"
    return
  fi

  if [[ "$requirement" == "required" ]] && ! homebrew_ensure_installed; then
    cli_error "Homebrew installation failed."
    return 1
  fi

  if ! have_command brew; then
    cli_error "Homebrew is required to install packages."
    if [[ "$requirement" == "optional" ]]; then
      SELFISHELL_SKIPPED_OPTIONAL_PACKAGES+=("$@")
      return 0
    fi
    return 1
  fi

  for package in "$@"; do
    if homebrew_package_installed "$manager" "$package"; then
      installed_packages+=("$package")
    else
      missing_packages+=("$package")
    fi
  done

  if ((${#installed_packages[@]} > 0)); then
    printf '%sAlready installed Homebrew %s (%d):%s %s\n' \
      "$SELFISHELL_COLOR_GREEN" "$manager" "${#installed_packages[@]}" "$SELFISHELL_COLOR_RESET" "${installed_packages[*]}"
  fi

  ((${#missing_packages[@]} > 0)) || return 0

  if [[ "$manager" == "cask" ]]; then
    if ! HOMEBREW_NO_ASK=1 brew install --cask "${missing_packages[@]}"; then
      cli_error "Could not install $requirement Homebrew casks: ${missing_packages[*]}"
      if [[ "$requirement" == "optional" ]]; then
        SELFISHELL_SKIPPED_OPTIONAL_PACKAGES+=("${missing_packages[@]}")
        return 0
      fi
      return 1
    fi
  else
    if ! HOMEBREW_NO_ASK=1 brew install "${missing_packages[@]}"; then
      cli_error "Could not install $requirement Homebrew formulae: ${missing_packages[*]}"
      if [[ "$requirement" == "optional" ]]; then
        SELFISHELL_SKIPPED_OPTIONAL_PACKAGES+=("${missing_packages[@]}")
        return 0
      fi
      return 1
    fi
  fi
}
