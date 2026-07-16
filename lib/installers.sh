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
    starship | nvm | pyenv | pyenv-virtualenv | kubectl | zinit | vundle)
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

install_vim_plugins() {
  local dry_run="$1"

  [[ "${SELFISHELL_OFFLINE:-0}" != "1" ]] || return 0
  if [[ "$dry_run" == "1" ]]; then
    printf 'Would install declared Vim plugins.\n'
    return
  fi
  if ! have_command vim; then
    cli_error "Vim is required to install declared plugins."
    return 1
  fi

  vim +PluginInstall +qall
}
