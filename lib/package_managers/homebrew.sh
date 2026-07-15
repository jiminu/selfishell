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
  /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
  homebrew_activate
  have_command brew
}

homebrew_install_packages() {
  local requirement="$1"
  local manager="$2"
  local dry_run="$3"
  shift 3

  (("$#" > 0)) || return 0

  if [[ "$dry_run" == "1" ]]; then
    printf 'Would install %s Homebrew %s: %s\n' "$requirement" "$manager" "$*"
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

  if [[ "$manager" == "cask" ]]; then
    if ! brew install --cask "$@"; then
      cli_error "Could not install $requirement Homebrew casks: $*"
      if [[ "$requirement" == "optional" ]]; then
        SELFISHELL_SKIPPED_OPTIONAL_PACKAGES+=("$@")
        return 0
      fi
      return 1
    fi
  else
    if ! brew install "$@"; then
      cli_error "Could not install $requirement Homebrew formulae: $*"
      if [[ "$requirement" == "optional" ]]; then
        SELFISHELL_SKIPPED_OPTIONAL_PACKAGES+=("$@")
        return 0
      fi
      return 1
    fi
  fi
}
