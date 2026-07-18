#!/usr/bin/env bash

install_direct_package() {
  local requirement="$1"
  local package="$2"
  local dry_run="$3"

  if [[ "$dry_run" == "1" ]]; then
    printf 'Would install %s direct package: %s\n' "$requirement" "$package"
    return
  fi

  case "$package" in
    starship | mise | zinit | neovim)
      local dependency_platform
      case "$(detect_platform)" in ubuntu | ubuntu-wsl) dependency_platform=linux ;; *) dependency_platform="$(detect_platform)" ;; esac
      dependency_install "$package" "$dependency_platform" "$(detect_architecture)"
      ;;
    *)
      cli_error "Unknown direct package: $package"
      if [[ "$requirement" == "optional" ]]; then
        SELFISHELL_SKIPPED_OPTIONAL_PACKAGES+=("$package")
        return 0
      fi
      return 1
      ;;
  esac
}

install_mise_tools() {
  local requirement="$1"
  local dry_run="$2"
  local mise_command
  shift 2

  if [[ "$dry_run" == "1" ]]; then
    printf 'Would install %s mise tools: %s\n' "$requirement" "$*"
    return
  fi
  if have_command mise; then
    mise_command="$(command -v mise)"
  elif [[ -x "$HOME/.local/bin/mise" ]]; then
    mise_command="$HOME/.local/bin/mise"
  else
    cli_error "mise is required to install mise-managed tools."
    if [[ "$requirement" == "optional" ]]; then
      SELFISHELL_SKIPPED_OPTIONAL_PACKAGES+=("$@")
      return 0
    fi
    return 1
  fi

  if ! MISE_GLOBAL_CONFIG_FILE="$SELFISHELL_ROOT/common/mise.toml" "$mise_command" install "$@"; then
    cli_error "Could not install $requirement mise tools: $*"
    if [[ "$requirement" == "optional" ]]; then
      SELFISHELL_SKIPPED_OPTIONAL_PACKAGES+=("$@")
      return 0
    fi
    return 1
  fi
}

install_vim_plugins() {
  local dry_run="$1"
  local nvim_command

  [[ "${SELFISHELL_OFFLINE:-0}" != "1" ]] || return 0
  if [[ "$dry_run" == "1" ]]; then
    printf 'Would install declared Neovim plugins.\n'
    return
  fi
  if have_command nvim; then
    nvim_command="nvim"
  elif [[ -x "$HOME/.local/bin/nvim" ]]; then
    nvim_command="$HOME/.local/bin/nvim"
  else
    cli_error "Neovim is required to install declared plugins."
    return 1
  fi

  if ! APPIMAGE_EXTRACT_AND_RUN=1 "$nvim_command" --headless "+Lazy! sync" +qa >/dev/null 2>&1; then
    cli_error "Could not install Neovim plugins."
    return 1
  fi
}
